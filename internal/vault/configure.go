package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Configure writes the settings c gives to vault.yaml, atomically, keeping
// its other keys and comments, emits vault.configured and returns the
// settings now in effect (ADR-0021). It is the only write notesd makes to
// SettingsDir.
func (v *Vault) Configure(ctx context.Context, c *api.VaultConfigureParams) (Settings, error) {
	edits, err := settingEdits(c)
	if err != nil {
		return Settings{}, err
	}
	r, err := v.open()
	if err != nil {
		return Settings{}, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return Settings{}, err
	}
	defer v.mu.Unlock()

	var doc yaml.Node
	old, err := r.ReadFile(SettingsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return Settings{}, fmt.Errorf("read %s: %w", SettingsFile, err)
	}
	if err := yaml.Unmarshal(old, &doc); err != nil {
		return Settings{}, fmt.Errorf("%s does not parse, so it is left as it is: %w", SettingsFile, err)
	}
	doc.Kind = yaml.DocumentNode // an empty file, or one of comments, has none
	switch {
	case len(doc.Content) == 0 || doc.Content[0].Tag == "!!null":
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	case doc.Content[0].Kind != yaml.MappingNode:
		return Settings{}, fmt.Errorf("%s does not hold a mapping, so it is left as it is", SettingsFile)
	}
	for _, e := range edits {
		set(doc.Content[0], e.keys, e.value)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return Settings{}, fmt.Errorf("encode %s: %w", SettingsFile, err)
	}
	if err := enc.Close(); err != nil {
		return Settings{}, fmt.Errorf("encode %s: %w", SettingsFile, err)
	}
	data := buf.Bytes()

	// What the file would give must be settings notesd can use.
	s, err := parseSettings(v.root, data)
	if err != nil {
		return Settings{}, api.InvalidParams("%v", err)
	}
	if _, err := DailyPath(s.Daily, v.cfg.Now()); err != nil {
		return Settings{}, err
	}

	if err := mkdirs(r, SettingsFile); err != nil {
		return Settings{}, err
	}
	var existing fs.FileInfo
	if old != nil {
		if existing, err = r.Lstat(SettingsFile); err != nil {
			return Settings{}, fmt.Errorf("write %s: %w", SettingsFile, err)
		}
	}
	st, err := stage(r, v.root, SettingsFile, data, existing)
	if err != nil {
		return Settings{}, err
	}
	if err := v.settled(st.commit(false)); err != nil {
		return Settings{}, err
	}
	// The file is written: announce it whatever the client does now, and
	// record its digest, so the scan its change starts does not again.
	ctx = context.WithoutCancel(ctx)
	v.settingsDigest, v.settingsSeen = s.digest, true
	if err := v.emitConfigured(ctx); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// emitConfigured emits vault.configured.
func (v *Vault) emitConfigured(ctx context.Context) error {
	return v.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.Emit(ctx, api.VaultConfigured{ID: v.id}) })
}

// settingEdit is one key of vault.yaml to set.
type settingEdit struct {
	keys  []string
	value *yaml.Node
}

// settingEdits checks the settings c gives and returns them as edits of
// vault.yaml, folders and paths cleaned.
func settingEdits(c *api.VaultConfigureParams) ([]settingEdit, error) {
	var edits []settingEdit
	str := func(v *string, keys ...string) {
		if v != nil {
			edits = append(edits, settingEdit{keys, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: *v}})
		}
	}
	boolean := func(v *bool, keys ...string) {
		if v != nil {
			edits = append(edits, settingEdit{keys, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(*v)}})
		}
	}
	folder := func(name string, v *string, keys ...string) error {
		if v == nil {
			return nil
		}
		f, err := settingFolder(name, *v)
		if err == nil {
			str(&f, keys...)
		}
		return err
	}
	if err := folder("dailyFolder", c.DailyFolder, "daily", "folder"); err != nil {
		return nil, err
	}
	str(c.DailyFormat, "daily", "format")
	if t := c.DailyTemplate; t != nil {
		p := strings.Trim(strings.TrimSpace(*t), "/")
		if p != "" {
			if err := checkNotePath(p); err != nil {
				return nil, api.InvalidParams("dailyTemplate %q: %v", *t, err)
			}
		}
		str(&p, "daily", "template")
	}
	if err := folder("templateFolder", c.TemplateFolder, "templates", "folder"); err != nil {
		return nil, err
	}
	str(c.TemplateDateFormat, "templates", "dateFormat")
	str(c.TemplateTimeFormat, "templates", "timeFormat")
	if l := c.NewNoteLocation; l != nil {
		if !l.Valid() {
			return nil, api.InvalidParams("newNoteLocation %q: root, current or folder", *l)
		}
		str(new(string(*l)), "newNotes", "location")
	}
	if err := folder("newNoteFolder", c.NewNoteFolder, "newNotes", "folder"); err != nil {
		return nil, err
	}
	if a := c.AttachmentFolder; a != nil {
		// As Obsidian writes it: "/" the root, "./" beside the note, "./NAME"
		// a folder beside it, else a folder's path.
		rest, beside := strings.CutPrefix(strings.TrimSpace(*a), "./")
		f, err := settingFolder("attachmentFolder", rest)
		if err != nil {
			return nil, err
		}
		switch {
		case beside:
			f = "./" + f
		case f == "":
			f = "/"
		}
		str(&f, "attachments", "folder")
	}
	if l := c.LinkFormat; l != nil {
		if !l.Valid() {
			return nil, api.InvalidParams("linkFormat %q: shortest, relative or absolute", *l)
		}
		str(new(string(*l)), "links", "format")
	}
	boolean(c.Wikilinks, "links", "wikilinks")
	boolean(c.UpdateLinks, "links", "update")
	if c.Ignore != nil {
		if _, err := NewIgnore(c.Ignore); err != nil {
			return nil, api.InvalidParams("ignore: %v", err)
		}
		list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
		for _, p := range c.Ignore {
			list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p})
		}
		edits = append(edits, settingEdit{[]string{"ignore"}, list})
	}
	return edits, nil
}

// settingFolder checks a folder a setting names: "" is the vault's root,
// and the settings folder is no place for notes.
func settingFolder(name, folder string) (string, error) {
	folder, err := CheckFolder(strings.TrimSpace(folder))
	if err == nil && (folder == SettingsDir || IsSettings(folder)) {
		err = fmt.Errorf("%s holds no notes", SettingsDir)
	}
	if err != nil {
		return "", api.InvalidParams("%s %q: %v", name, folder, err)
	}
	return folder, nil
}

// set sets the value at keys under mapping m, adding the keys it lacks and
// keeping the comments of a value it replaces.
func set(m *yaml.Node, keys []string, value *yaml.Node) {
	for i := 0; i < len(m.Content)-1; i += 2 {
		if m.Content[i].Value != keys[0] {
			continue
		}
		old := m.Content[i+1]
		if len(keys) == 1 {
			value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
			m.Content[i+1] = value
			return
		}
		if old.Kind != yaml.MappingNode {
			m.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", LineComment: old.LineComment}
		}
		set(m.Content[i+1], keys[1:], value)
		return
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: keys[0]}
	if len(keys) == 1 {
		m.Content = append(m.Content, key, value)
		return
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, key, child)
	set(child, keys[1:], value)
}
