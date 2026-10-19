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
	var out *api.DailyNote
	err := v.withDaily(ctx, date, who, func(_ *os.Root, n *api.Note, created bool) error {
		out = &api.DailyNote{Note: *n, Created: created}
		return nil
	})
	return out, err
}

// AppendDaily appends text at the end of date's daily note, after a line
// break when the note does not end in one, and ends it with one; it
// creates the note first as Daily does. The append is a write whose base
// is the note as read under the vault's lock, so it changes no text that
// is there (ADR-0028).
func (v *Vault) AppendDaily(ctx context.Context, date time.Time, text string, who Author) (*api.DailyAppend, error) {
	var out *api.DailyAppend
	err := v.withDaily(ctx, date, who, func(r *os.Root, n *api.Note, created bool) error {
		res, err := v.write(ctx, r, n.Path, n.Hash, []byte(appendText(n.Content, text)), true, who)
		if err == nil {
			out = &api.DailyAppend{Write: *res, Created: created}
		}
		return err
	})
	return out, err
}

// appendText is content with text after it, each ending in a line break.
func appendText(content, text string) string {
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return content + text
}

// withDaily calls fn with date's daily note, created from the template
// when it is missing, under the vault's lock. ctx's write guard judges the
// note's path first, whether or not it exists.
func (v *Vault) withDaily(ctx context.Context, date time.Time, who Author, fn func(r *os.Root, n *api.Note, created bool) error) error {
	settings, err := LoadSettings(v.root)
	if err != nil {
		return err
	}
	p, err := DailyPath(settings.Daily, date)
	if err != nil {
		return err
	}
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return err
	}
	if err := checkGuard(ctx, p); err != nil {
		return err
	}
	n, err := v.read(ctx, r, p)
	if err == nil {
		return fn(r, n, false)
	}
	if !errors.Is(err, api.ErrNotFound) {
		return err
	}
	if _, err := v.create(ctx, r, p, v.dailyContent(r, settings, p, date), who, 0); err != nil {
		return err
	}
	if n, err = v.read(ctx, r, p); err != nil {
		return err
	}
	return fn(r, n, true)
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
