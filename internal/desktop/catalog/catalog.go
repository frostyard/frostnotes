// Package catalog is what the desktop app knows of a vault's files
// (docs/design/app.md#data): the files and folders from notesd, the notes'
// aliases and the tags, kept up to date from its events, with the link
// text and completions the editor asks for. It imports no window.
package catalog

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/markdown"
)

// The catalog's timing, as the web app's.
const (
	// AliasesDelay is how long after the files change the aliases are
	// read again.
	AliasesDelay = time.Second
	// TagsTTL is how long the tags are kept before they are read again.
	TagsTTL = 10 * time.Second
	// ReloadDelay is how long after a new file the files are read again.
	ReloadDelay = 250 * time.Millisecond
)

// API is what the catalog needs of notesd; api.NoteClient has it.
type API interface {
	Files(ctx context.Context, p *api.NoteFilesParams) (*api.VaultFiles, error)
	List(ctx context.Context, p *api.NoteListParams) ([]api.NoteEntry, error)
	Tags(ctx context.Context, p *api.NoteTagsParams) ([]api.TagCount, error)
	Resolve(ctx context.Context, p *api.NoteResolveParams) ([]api.Resolution, error)
	Read(ctx context.Context, p *api.NoteReadParams) (*api.Note, error)
	Upload(ctx context.Context, p *api.NoteUploadParams) (*api.WriteResult, error)
}

// Catalog is one vault's files. UI thread only.
type Catalog struct {
	env   notes.Env
	api   func() API
	vault int64

	files   []api.VaultFile
	named   map[string][]string // paths by last segment, lower-cased
	folders []string
	aliases map[string][]string
	tags    []api.TagCount
	tagsAt  time.Time
	stop    func()
	// LinkFormat is the vault's: shortest, relative or absolute.
	LinkFormat string
	// AttachmentFolder and Wikilinks are the vault's attachment settings.
	AttachmentFolder string
	Wikilinks        bool
	changed          []func()
	reloading        bool
	loaded           bool
}

// New makes the catalog of a vault; Reload reads it.
func New(env notes.Env, client func() API, vault int64) *Catalog {
	return &Catalog{env: env, api: client, vault: vault, aliases: map[string][]string{}, LinkFormat: "shortest", AttachmentFolder: "/", Wikilinks: true}
}

// OnChange calls fn on the UI thread when the files, aliases or tags
// change.
func (c *Catalog) OnChange(fn func()) { c.changed = append(c.changed, fn) }

func (c *Catalog) notify() {
	for _, fn := range c.changed {
		fn()
	}
}

// Loaded reports whether the files have been read once.
func (c *Catalog) Loaded() bool { return c.loaded }

// Files are the vault's files, by path.
func (c *Catalog) Files() []api.VaultFile { return c.files }

// File is the file at p.
func (c *Catalog) File(p string) (api.VaultFile, bool) {
	i, ok := slices.BinarySearchFunc(c.files, p, func(f api.VaultFile, p string) int { return strings.Compare(f.Path, p) })
	if !ok {
		return api.VaultFile{}, false
	}
	return c.files[i], true
}

// Folders are its folders, by path.
func (c *Catalog) Folders() []string { return c.folders }

// Aliases are a note's aliases.
func (c *Catalog) Aliases(p string) []string { return c.aliases[p] }

func (c *Catalog) do(f func(ctx context.Context, n API) error, done func(error)) {
	n := c.api()
	if n == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := f(ctx, n)
		c.env.Post(func() { done(err) })
	}()
}

// Reload reads the files, then the aliases and tags.
func (c *Catalog) Reload() {
	var got *api.VaultFiles
	c.do(func(ctx context.Context, n API) (err error) {
		got, err = n.Files(ctx, &api.NoteFilesParams{VaultID: c.vault})
		return err
	}, func(err error) {
		if err != nil {
			return
		}
		c.files, c.folders, c.loaded = got.Files, got.Folders, true
		slices.SortFunc(c.files, func(a, b api.VaultFile) int { return strings.Compare(a.Path, b.Path) })
		c.named = make(map[string][]string, len(c.files))
		for _, f := range c.files {
			name := strings.ToLower(path.Base(f.Path))
			c.named[name] = append(c.named[name], f.Path)
		}
		c.notify()
		c.readAliases()
		c.readTags()
	})
}

func (c *Catalog) readAliases() {
	var entries []api.NoteEntry
	c.do(func(ctx context.Context, n API) (err error) {
		entries, err = n.List(ctx, &api.NoteListParams{VaultID: c.vault})
		return err
	}, func(err error) {
		if err != nil {
			return
		}
		c.aliases = make(map[string][]string, len(entries))
		for _, e := range entries {
			if len(e.Aliases) > 0 {
				c.aliases[e.Path] = e.Aliases
			}
		}
		c.notify()
	})
}

func (c *Catalog) readTags() {
	c.tagsAt = c.env.Now()
	var got []api.TagCount
	c.do(func(ctx context.Context, n API) (err error) {
		got, err = n.Tags(ctx, &api.NoteTagsParams{VaultID: c.vault})
		return err
	}, func(err error) {
		if err == nil {
			c.tags = got
			c.notify()
		}
	})
}

// reloadSoon reads the files once after a burst of new ones: a scan
// reports thousands.
func (c *Catalog) reloadSoon() {
	if c.reloading {
		return
	}
	c.reloading = true
	c.env.After(ReloadDelay, func() {
		c.reloading = false
		c.Reload()
	})
}

func (c *Catalog) soon() {
	if c.stop != nil {
		c.stop()
	}
	c.stop = c.env.After(AliasesDelay, func() {
		c.stop = nil
		c.readAliases()
	})
}

// Event follows notesd's events for the catalog's vault: a known file's
// new hash is patched in; anything else reads the files again.
func (c *Catalog) Event(ev api.Event) {
	switch e := ev.(type) {
	case api.NoteChanged:
		if e.VaultID != c.vault {
			return
		}
		if i, ok := slices.BinarySearchFunc(c.files, e.Path, func(f api.VaultFile, p string) int { return strings.Compare(f.Path, p) }); ok {
			c.files[i].Hash = e.Hash
			c.soon()
			return
		}
		c.reloadSoon()
	case api.NoteMoved:
		if e.VaultID == c.vault {
			c.Reload()
		}
	case api.NoteRemoved:
		if e.VaultID == c.vault {
			c.Reload()
		}
	}
}

func bare(p string) string {
	if strings.HasSuffix(strings.ToLower(p), ".md") {
		return p[:len(p)-3]
	}
	return p
}

func nameOf(p string) string { return bare(path.Base(p)) }

// LinkText is what a link from the note at from writes to name p, by the
// vault's link format: absolute, the path; relative, the path from the
// note's folder; shortest, the file's name when no other file has it,
// else the path. A note's ".md" is left out.
func (c *Catalog) LinkText(p, from string) string {
	switch c.LinkFormat {
	case "absolute":
		return bare(p)
	case "relative":
		var dir []string
		if d := path.Dir(from); d != "." {
			dir = strings.Split(d, "/")
		}
		to := strings.Split(p, "/")
		i := 0
		for i < len(dir) && i < len(to)-1 && dir[i] == to[i] {
			i++
		}
		parts := make([]string, 0, len(dir)-i+len(to)-i)
		for range dir[i:] {
			parts = append(parts, "..")
		}
		return bare(strings.Join(append(parts, to[i:]...), "/"))
	}
	name := strings.ToLower(nameOf(p))
	same := 0
	for _, f := range c.files {
		if strings.ToLower(nameOf(f.Path)) == name {
			same++
		}
	}
	if same > 1 {
		return bare(p)
	}
	return nameOf(p)
}

// Match is a note a query found: by its path, or by one of its aliases.
type Match struct {
	Path  string
	Alias string // "" when the path matched
	Score int
}

// Search ranks the files and aliases for query, best first, at most limit:
// the quick switcher's and [['s results. Non-notes are left out unless
// files is true.
func (c *Catalog) Search(query string, limit int, files bool) []Match {
	var out []Match
	for _, f := range c.files {
		if !files && f.Kind != api.FileKindNote {
			continue
		}
		if s, ok := frostui.Match(query, bare(f.Path)); ok {
			out = append(out, Match{Path: f.Path, Score: s})
		}
		for _, a := range c.aliases[f.Path] {
			if s, ok := frostui.Match(query, a); ok {
				out = append(out, Match{Path: f.Path, Alias: a, Score: s})
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Match) int { return b.Score - a.Score })
	return out[:min(len(out), limit)]
}

// Sources are a note's completions and uploads, from the catalog: what
// the editor asks. text gives a note's text when a tab has it open.
func (c *Catalog) Sources(from func() string, text func(path string) (string, bool)) *NoteSources {
	return &NoteSources{c: c, from: from, text: text}
}

// Upload stores files beside the note (ADR-0023), by the vault's settings,
// and gives the embeds to insert, one per line.
func (s *NoteSources) Upload(files []editor.File, done func(embeds string, err error)) {
	c := s.c
	folder := editor.AttachmentFolder(c.AttachmentFolder, s.from())
	paths := make(map[string]bool, len(c.files))
	names := map[string]int{}
	for _, f := range c.files {
		paths[f.Path] = true
		names[path.Base(f.Path)]++
	}
	var embeds []string
	var err error
	c.do(func(ctx context.Context, n API) error {
		for _, f := range files {
			if len(f.Data) > editor.MaxAttach {
				return fmt.Errorf("%s is over 10 MB: copy it into the vault's folder instead", f.Name)
			}
			p := editor.FreePath(folder, f.Name, func(x string) bool { return paths[x] })
			if _, err := n.Upload(ctx, &api.NoteUploadParams{VaultID: c.vault, Path: p, Data: base64.StdEncoding.EncodeToString(f.Data)}); err != nil {
				return fmt.Errorf("upload %s: %w", f.Name, err)
			}
			base := path.Base(p)
			embeds = append(embeds, editor.EmbedFor(p, c.Wikilinks, names[base] == 0))
			paths[p] = true
			names[base]++
		}
		return nil
	}, func(e error) {
		err = e
		done(strings.Join(embeds, "\n"), err)
		c.Reload()
	})
}

// NoteSources are one note's completions and uploads.
type NoteSources struct {
	c    *Catalog
	from func() string
	text func(path string) (string, bool)
}

// LinkTargets lists the notes [[ completes to.
func (s *NoteSources) LinkTargets(query string, limit int) []editor.Target {
	var out []editor.Target
	from := s.from()
	if strings.TrimSpace(query) == "" {
		for _, f := range s.c.files[:min(len(s.c.files), limit)] {
			out = append(out, editor.Target{Insert: s.c.LinkText(f.Path, from), Label: bare(f.Path)})
		}
		return out
	}
	for _, m := range s.c.Search(query, limit, true) {
		if m.Alias != "" {
			out = append(out, editor.Target{Insert: s.c.LinkText(m.Path, from) + "|" + m.Alias, Label: m.Alias, Detail: bare(m.Path)})
		} else {
			out = append(out, editor.Target{Insert: s.c.LinkText(m.Path, from), Label: bare(m.Path)})
		}
	}
	return out
}

// Tags lists the vault's tags, by count.
// Resolve is the file a link from the note names, "" for none, by the
// rules notesd follows (docs/specs/markdown.md#link-resolution).
func (s *NoteSources) Resolve(target string) string {
	return markdown.Resolve(s.from(), target, func(name string) []string { return s.c.named[name] })
}

func (s *NoteSources) Tags() []string {
	if s.c.env.Now().Sub(s.c.tagsAt) > TagsTTL {
		s.c.readTags()
	}
	tags := slices.Clone(s.c.tags)
	slices.SortStableFunc(tags, func(a, b api.TagCount) int { return int(b.Count - a.Count) })
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Tag
	}
	return out
}

// Subpaths lists the headings or block IDs of the note target leads to
// ("" is the note being edited), from its text.
func (s *NoteSources) Subpaths(target string, blocks bool, done func([]editor.Target)) {
	from := s.from()
	finish := func(text string) { done(subpaths(text, blocks)) }
	if target == "" {
		if t, ok := s.text(from); ok {
			finish(t)
		}
		return
	}
	var resolved, content string
	s.c.do(func(ctx context.Context, n API) error {
		rs, err := n.Resolve(ctx, &api.NoteResolveParams{VaultID: s.c.vault, From: from, Targets: []string{target}})
		if err != nil || len(rs) == 0 || rs[0].Path == nil || !strings.HasSuffix(*rs[0].Path, ".md") {
			return err
		}
		resolved = *rs[0].Path
		note, err := n.Read(ctx, &api.NoteReadParams{VaultID: s.c.vault, Path: resolved})
		if err != nil {
			return err
		}
		content = note.Content
		return nil
	}, func(err error) {
		if err != nil || resolved == "" {
			return
		}
		if t, ok := s.text(resolved); ok {
			content = t
		}
		finish(content)
	})
}

func subpaths(text string, blocks bool) []editor.Target {
	doc := markdown.Parse([]byte(text))
	var out []editor.Target
	if blocks {
		lines := strings.Split(text, "\n")
		for _, b := range doc.Blocks {
			detail := ""
			if b.Line >= 1 && b.Line <= len(lines) {
				detail = strings.TrimSpace(lines[b.Line-1])
			}
			out = append(out, editor.Target{Insert: b.ID, Label: b.ID, Detail: detail})
		}
		return out
	}
	for _, h := range doc.Headings {
		out = append(out, editor.Target{Insert: h.Text, Label: h.Text})
	}
	return out
}
