package vault

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Settings are a vault's settings (docs/design/vault.md#on-disk). Each one
// comes from .frostnotes/vault.yaml, or while that lacks it from
// Obsidian's .obsidian/daily-notes.json or templates.json, or is the
// default. Reading them never writes to the vault.
type Settings struct {
	Daily     DailySettings
	Templates TemplateSettings
	// Ignore holds gitignore-style patterns of files the vault leaves out
	// (NewIgnore).
	Ignore []string
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

// SettingsFile is the settings file under SettingsDir.
const SettingsFile = SettingsDir + "/vault.yaml"

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

// LoadSettings reads the settings of the vault at root. A vault.yaml that
// cannot be read or decoded is an error; Obsidian's files are only
// defaults, so one that cannot be decoded is skipped.
func LoadSettings(root string) (Settings, error) {
	var own vaultYAML
	data, err := readSetting(root, SettingsFile)
	if err != nil {
		return Settings{}, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &own); err != nil {
			return Settings{}, fmt.Errorf("%s: %w", SettingsFile, err)
		}
	}
	var daily obsidianDaily
	if data, err := readSetting(root, ".obsidian/daily-notes.json"); err == nil && data != nil {
		_ = json.Unmarshal(data, &daily)
	}
	var tmpl obsidianTemplates
	if data, err := readSetting(root, ".obsidian/templates.json"); err == nil && data != nil {
		_ = json.Unmarshal(data, &tmpl)
	}

	pick := func(v *string, fallback string) string {
		if v != nil {
			return strings.TrimSpace(*v)
		}
		return strings.TrimSpace(fallback)
	}
	var s Settings
	if d := own.Daily; d != nil {
		s.Daily = DailySettings{pick(d.Folder, daily.Folder), pick(d.Format, daily.Format), pick(d.Template, daily.Template)}
	} else {
		s.Daily = DailySettings{pick(nil, daily.Folder), pick(nil, daily.Format), pick(nil, daily.Template)}
	}
	if t := own.Templates; t != nil {
		s.Templates = TemplateSettings{pick(t.Folder, tmpl.Folder), pick(t.DateFormat, tmpl.DateFormat), pick(t.TimeFormat, tmpl.TimeFormat)}
	} else {
		s.Templates = TemplateSettings{pick(nil, tmpl.Folder), pick(nil, tmpl.DateFormat), pick(nil, tmpl.TimeFormat)}
	}
	s.Daily.Folder = strings.Trim(s.Daily.Folder, "/")
	s.Daily.Template = strings.Trim(s.Daily.Template, "/")
	s.Templates.Folder = strings.Trim(s.Templates.Folder, "/")
	s.Ignore = own.Ignore
	if _, err := NewIgnore(s.Ignore); err != nil {
		return Settings{}, fmt.Errorf("%s: %w", SettingsFile, err)
	}
	return s, nil
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
