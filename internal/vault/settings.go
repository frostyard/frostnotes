package vault

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
)

// Settings are a vault's settings (docs/design/vault.md#on-disk). Each one
// comes from .frostnotes/vault.yaml, or while that lacks it from
// Obsidian's .obsidian/daily-notes.json, templates.json or app.json, or is
// the default. Reading them never writes to the vault.
type Settings struct {
	Daily     DailySettings
	Templates TemplateSettings
	Files     FileSettings
	// Ignore holds gitignore-style patterns of files the vault leaves out
	// (NewIgnore). Only vault.yaml sets them: Obsidian's Excluded files
	// hide files from its search and graph but keep them in the vault.
	Ignore []string
	// Sources says where each setting comes from, by its field's name in
	// api.VaultSettings.
	Sources map[string]api.SettingSource
	// digest is vault.yaml's content hash, "" when there is none: how a
	// scan tells that the file changed.
	digest string
}

// DailySettings say where daily notes go and what a new one holds.
type DailySettings struct {
	Folder   string // "" is the vault root
	Format   string // a moment.js format; "" is YYYY-MM-DD
	Template string // the template note's path, ".md" optional; "" for none
}

// TemplateSettings are the defaults of template variables.
type TemplateSettings struct {
	Folder     string // where templates live; "" when unset
	DateFormat string // {{date}}'s moment.js format; "" is YYYY-MM-DD
	TimeFormat string // {{time}}'s moment.js format; "" is HH:mm
}

// FileSettings say where new notes and files go and how new links are
// written: Obsidian's Files and links settings, with its defaults.
type FileSettings struct {
	NewNoteLocation  api.NewNoteLocation // root
	NewNoteFolder    string              // with NewNoteLocationFolder; "" is the vault root
	AttachmentFolder string              // as Obsidian writes it; "/" is the vault root
	LinkFormat       api.LinkFormat      // shortest
	Wikilinks        bool                // true
	UpdateLinks      bool                // false
}

// SettingsFile is the settings file under SettingsDir.
const SettingsFile = SettingsDir + "/vault.yaml"

// TypesFile is the property types file under SettingsDir.
const TypesFile = SettingsDir + "/types.yaml"

// vaultYAML is vault.yaml; a nil field is a setting it lacks.
type vaultYAML struct {
	Daily *struct {
		Folder   *string `yaml:"folder"`
		Format   *string `yaml:"format"`
		Template *string `yaml:"template"`
	} `yaml:"daily"`
	Templates *struct {
		Folder     *string `yaml:"folder"`
		DateFormat *string `yaml:"dateFormat"`
		TimeFormat *string `yaml:"timeFormat"`
	} `yaml:"templates"`
	NewNotes *struct {
		Location *string `yaml:"location"`
		Folder   *string `yaml:"folder"`
	} `yaml:"newNotes"`
	Attachments *struct {
		Folder *string `yaml:"folder"`
	} `yaml:"attachments"`
	Links *struct {
		Format    *string `yaml:"format"`
		Wikilinks *bool   `yaml:"wikilinks"`
		Update    *bool   `yaml:"update"`
	} `yaml:"links"`
	Ignore []string `yaml:"ignore"`
}

// obsidianDaily is .obsidian/daily-notes.json; obsidianTemplates is
// .obsidian/templates.json. Obsidian writes "" for a default.
type obsidianDaily struct {
	Folder   string `json:"folder"`
	Format   string `json:"format"`
	Template string `json:"template"`
}

type obsidianTemplates struct {
	Folder     string `json:"folder"`
	DateFormat string `json:"dateFormat"`
	TimeFormat string `json:"timeFormat"`
}

// obsidianApp is the part of .obsidian/app.json notesd reads; Obsidian
// leaves a setting at its default out.
type obsidianApp struct {
	NewFileLocation      *string `json:"newFileLocation"`
	NewFileFolderPath    *string `json:"newFileFolderPath"`
	AttachmentFolderPath *string `json:"attachmentFolderPath"`
	NewLinkFormat        *string `json:"newLinkFormat"`
	UseMarkdownLinks     *bool   `json:"useMarkdownLinks"`
	AlwaysUpdateLinks    *bool   `json:"alwaysUpdateLinks"`
}

// LoadSettings reads the settings of the vault at root. A vault.yaml that
// cannot be read or decoded, or holds a value no setting takes, is an
// error; Obsidian's files are only defaults, so one that cannot be
// decoded, or a value in it that is not one, is skipped.
func LoadSettings(root string) (Settings, error) {
	data, err := readSetting(root, SettingsFile)
	if err != nil {
		return Settings{}, err
	}
	return parseSettings(root, data)
}

// parseSettings is LoadSettings with data as vault.yaml's content, nil
// when there is none.
func parseSettings(root string, data []byte) (Settings, error) {
	var own vaultYAML
	if data != nil {
		if err := yaml.Unmarshal(data, &own); err != nil {
			return Settings{}, fmt.Errorf("%s: %w", SettingsFile, err)
		}
	}
	var daily obsidianDaily
	readObsidian(root, "daily-notes.json", &daily)
	var tmpl obsidianTemplates
	readObsidian(root, "templates.json", &tmpl)
	var app obsidianApp
	readObsidian(root, "app.json", &app)

	s := Settings{Sources: map[string]api.SettingSource{}}
	if data != nil {
		s.digest = blobs.ID(data)
	}
	str := func(name string, own *string, obsidian string) string {
		return strings.TrimSpace(*pick(s.Sources, name, own, given(obsidian)))
	}
	var d struct{ folder, format, template *string }
	if own.Daily != nil {
		d.folder, d.format, d.template = own.Daily.Folder, own.Daily.Format, own.Daily.Template
	}
	s.Daily = DailySettings{str("dailyFolder", d.folder, daily.Folder),
		str("dailyFormat", d.format, daily.Format), str("dailyTemplate", d.template, daily.Template)}
	var t struct{ folder, date, time *string }
	if own.Templates != nil {
		t.folder, t.date, t.time = own.Templates.Folder, own.Templates.DateFormat, own.Templates.TimeFormat
	}
	s.Templates = TemplateSettings{str("templateFolder", t.folder, tmpl.Folder),
		str("templateDateFormat", t.date, tmpl.DateFormat), str("templateTimeFormat", t.time, tmpl.TimeFormat)}
	s.Daily.Folder = strings.Trim(s.Daily.Folder, "/")
	s.Daily.Template = strings.Trim(s.Daily.Template, "/")
	s.Templates.Folder = strings.Trim(s.Templates.Folder, "/")
	if err := s.loadFiles(own, app); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", SettingsFile, err)
	}

	s.Ignore = own.Ignore
	s.Sources["ignore"] = api.SettingSourceDefault
	if own.Ignore != nil {
		s.Sources["ignore"] = api.SettingSourceFrostnotes
	}
	if _, err := NewIgnore(s.Ignore); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", SettingsFile, err)
	}
	return s, nil
}

// loadFiles picks the Files and links settings from vault.yaml's and
// app.json's.
func (s *Settings) loadFiles(own vaultYAML, app obsidianApp) error {
	var location, newFolder, attachments, format *string
	var wikilinks, update *bool
	if n := own.NewNotes; n != nil {
		location, newFolder = n.Location, n.Folder
	}
	if a := own.Attachments; a != nil {
		attachments = a.Folder
	}
	if l := own.Links; l != nil {
		format, wikilinks, update = l.Format, l.Wikilinks, l.Update
	}
	if location != nil && !api.NewNoteLocation(*location).Valid() {
		return fmt.Errorf("newNotes.location %q: root, current or folder", *location)
	}
	if format != nil && !api.LinkFormat(*format).Valid() {
		return fmt.Errorf("links.format %q: shortest, relative or absolute", *format)
	}
	if app.NewFileLocation != nil && !api.NewNoteLocation(*app.NewFileLocation).Valid() {
		app.NewFileLocation = nil
	}
	if app.NewLinkFormat != nil && !api.LinkFormat(*app.NewLinkFormat).Valid() {
		app.NewLinkFormat = nil
	}
	var markdownLinks *bool
	if app.UseMarkdownLinks != nil {
		markdownLinks = new(!*app.UseMarkdownLinks)
	}
	src := s.Sources
	s.Files = FileSettings{
		NewNoteLocation: api.NewNoteLocation(*pick(src, "newNoteLocation", location, app.NewFileLocation, "root")),
		NewNoteFolder:   strings.Trim(strings.TrimSpace(*pick(src, "newNoteFolder", newFolder, app.NewFileFolderPath, "")), "/"),
		AttachmentFolder: cmp.Or(strings.TrimSpace(
			*pick(src, "attachmentFolder", attachments, app.AttachmentFolderPath, "/")), "/"),
		LinkFormat:  api.LinkFormat(*pick(src, "linkFormat", format, app.NewLinkFormat, "shortest")),
		Wikilinks:   *pick(src, "wikilinks", wikilinks, markdownLinks, true),
		UpdateLinks: *pick(src, "updateLinks", update, app.AlwaysUpdateLinks, false),
	}
	return nil
}

// pick returns a setting's value, vault.yaml's (own) when it has one, else
// Obsidian's, else the default (the zero value when none is given), and
// records where it came from in sources.
func pick[T any](sources map[string]api.SettingSource, name string, own, obsidian *T, def ...T) *T {
	switch {
	case own != nil:
		sources[name] = api.SettingSourceFrostnotes
		return own
	case obsidian != nil:
		sources[name] = api.SettingSourceObsidian
		return obsidian
	}
	sources[name] = api.SettingSourceDefault
	v := new(T)
	if len(def) > 0 {
		*v = def[0]
	}
	return v
}

// given is an Obsidian string setting, nil when it is "": Obsidian writes
// "" for its default.
func given(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

// API is the settings as vault.settings returns them: each default
// spelled out.
func (s Settings) API() api.VaultSettings {
	return api.VaultSettings{
		DailyFolder:        s.Daily.Folder,
		DailyFormat:        cmp.Or(s.Daily.Format, "YYYY-MM-DD"),
		DailyTemplate:      s.Daily.Template,
		TemplateFolder:     s.Templates.Folder,
		TemplateDateFormat: cmp.Or(s.Templates.DateFormat, "YYYY-MM-DD"),
		TemplateTimeFormat: cmp.Or(s.Templates.TimeFormat, "HH:mm"),
		NewNoteLocation:    s.Files.NewNoteLocation,
		NewNoteFolder:      s.Files.NewNoteFolder,
		AttachmentFolder:   s.Files.AttachmentFolder,
		LinkFormat:         s.Files.LinkFormat,
		Wikilinks:          s.Files.Wikilinks,
		UpdateLinks:        s.Files.UpdateLinks,
		Ignore:             append([]string{}, s.Ignore...),
		Sources:            maps.Clone(s.Sources),
	}
}

// LoadTypes reads the property types of the vault at root, by key
// lower-cased: Obsidian's built-in ones (aliases, cssclasses, tags) under
// .frostnotes/types.yaml, a mapping of key to type, or while that does not
// exist Obsidian's .obsidian/types.json. A types.yaml that cannot be read
// or decoded is an error; types.json only gives defaults, so one that
// cannot be decoded is skipped.
func LoadTypes(root string) (map[string]string, error) {
	types := map[string]string{"aliases": "aliases", "cssclasses": "multitext", "tags": "tags"}
	var own map[string]string
	data, err := readSetting(root, TypesFile)
	if err != nil {
		return nil, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &own); err != nil {
			return nil, fmt.Errorf("%s: %w", TypesFile, err)
		}
	} else {
		var obsidian struct {
			Types map[string]string `json:"types"`
		}
		readObsidian(root, "types.json", &obsidian)
		own = obsidian.Types
	}
	for key, t := range own {
		if t = strings.TrimSpace(t); t != "" {
			types[strings.ToLower(key)] = t
		}
	}
	return types, nil
}

// readObsidian decodes one of Obsidian's settings files into v, leaving v
// as it was when the file is missing or does not decode.
func readObsidian[T any](root, name string, v *T) {
	data, err := readSetting(root, ".obsidian/"+name)
	if err != nil || data == nil {
		return
	}
	var decoded T
	if json.Unmarshal(data, &decoded) == nil {
		*v = decoded
	}
}

// readSetting reads a settings file, or returns nil when it is missing.
// Settings are not vault files, so a symlinked .obsidian is followed.
func readSetting(root, rel string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	return data, nil
}
