package vault

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/moment"
	"github.com/frostyard/frostnotes/internal/templates"
)

// DailyPath is the path of date's daily note under s: the folder, then the
// date in s's format, which may name subfolders, then ".md".
func DailyPath(s DailySettings, date time.Time) (string, error) {
	format := s.Format
	if format == "" {
		format = "YYYY-MM-DD"
	}
	p := strings.Trim(path.Join(s.Folder, moment.Format(format, date)+".md"), "/")
	if err := checkNotePath(p); err != nil {
		return "", api.InvalidParams("the daily-note settings give the path %q: %v", p, err)
	}
	return p, nil
}

// Daily returns date's daily note, creating it from the vault's daily-note
// template when it is missing (docs/design/vault.md#daily-notes).
func (v *Vault) Daily(ctx context.Context, date time.Time, who Author) (*api.DailyNote, error) {
	settings, err := LoadSettings(v.root)
	if err != nil {
		return nil, err
	}
	p, err := DailyPath(settings.Daily, date)
	if err != nil {
		return nil, err
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return nil, err
	}
	n, err := v.read(ctx, r, p)
	if err == nil {
		return &api.DailyNote{Note: *n}, nil
	}
	if !errors.Is(err, api.ErrNotFound) {
		return nil, err
	}
	if _, err := v.create(ctx, r, p, v.dailyContent(r, settings, p, date), who, 0); err != nil {
		return nil, err
	}
	if n, err = v.read(ctx, r, p); err != nil {
		return nil, err
	}
	return &api.DailyNote{Note: *n, Created: true}, nil
}

// dailyContent expands the daily-note template for the note at p, as
// Obsidian does: {{title}} is the note's name, {{date}} the date in the
// daily-note format, and the date carries the time of day it is now. A
// template that cannot be read gives an empty note, as in Obsidian.
func (v *Vault) dailyContent(r *os.Root, s Settings, p string, date time.Time) []byte {
	tmpl := s.Daily.Template
	if tmpl == "" {
		return nil
	}
	if !strings.HasSuffix(tmpl, ".md") {
		tmpl += ".md"
	}
	if err := CheckPath(tmpl); err != nil {
		v.log.Warn("daily-note template", "err", err)
		return nil
	}
	c, err := read(r, tmpl)
	if err != nil || c.data == nil {
		v.log.Warn("cannot read the daily-note template", "template", tmpl, "err", err)
		return nil
	}
	now := v.cfg.Now().In(date.Location())
	at := time.Date(date.Year(), date.Month(), date.Day(), now.Hour(), now.Minute(), now.Second(), 0, date.Location())
	format := s.Daily.Format
	if format == "" {
		format = "YYYY-MM-DD"
	}
	return templates.Expand(c.data, templates.Values{
		Title: strings.TrimSuffix(path.Base(p), ".md"), Now: at,
		DateFormat: format, TimeFormat: s.Templates.TimeFormat,
	})
}
