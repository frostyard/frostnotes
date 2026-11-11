// Package history is the desktop app's version history and merge view
// (docs/design/app.md#history-and-merges, specs/ui.md#history-batch-2 and
// #merge-view; plan 0009): a note's versions, and a synced vault's hub
// revisions, each shown as its source or as its changes against the
// note, restored with note.revert; and the merge view, MyGo's diff view
// of another text beside the note's own buffer, whose chunks the user
// takes back one at a time.
package history

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// API is what the history needs of notesd; api.NoteClient has it.
type API interface {
	Versions(ctx context.Context, p *api.NoteVersionsParams) ([]api.Version, error)
	ReadVersion(ctx context.Context, p *api.NoteReadVersionParams) (*api.VersionContent, error)
	Revert(ctx context.Context, p *api.NoteRevertParams) (*api.WriteResult, error)
}

// HubAPI is the hub's side of a synced vault; api.SyncClient has it.
type HubAPI interface {
	History(ctx context.Context, p *api.SyncHistoryParams) ([]api.Revision, error)
	ReadRevision(ctx context.Context, p *api.SyncReadRevisionParams) (*api.RevisionContent, error)
}

// Entry is one row of the history: a local version or a hub revision.
type Entry struct {
	Version  *api.Version
	Revision *api.Revision
}

// Title is the entry's time, as the list shows it.
func (e Entry) Title() string {
	switch {
	case e.Version != nil:
		return e.Version.At.Local().Format("Jan 2, 2006 15:04:05")
	case e.Revision != nil:
		return fmt.Sprintf("Revision %d", e.Revision.Seq)
	}
	return ""
}

// Detail says who made it, its size and what it is.
func (e Entry) Detail() string {
	var parts []string
	if v := e.Version; v != nil {
		who := string(v.Source)
		if v.Author != "" {
			who = v.Author
		}
		parts = append(parts, who, size(v.Size))
		if v.Conflict {
			parts = append(parts, "Conflict")
		}
		if v.Deleted {
			parts = append(parts, "Deleted")
		}
	}
	if r := e.Revision; r != nil {
		parts = append(parts, string(r.Op), size(r.Size))
	}
	return strings.Join(parts, " · ")
}

func size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// View is a tab's version history of a note. UI thread only.
type View struct {
	env  notes.Env
	api  func() API
	hub  func() HubAPI // nil, or returning nil, for a vault not synced
	doc  *notes.Document
	list ui.ListState

	entries  []Entry
	selected int
	content  string // the selected entry's content
	loaded   bool
	err      error

	// ShowChanges shows the selected entry against the note's text
	// rather than its source.
	ShowChanges bool
	// Compare opens the merge view of the note against text, which
	// title names.
	Compare func(text, title string)

	source *code.State
	diff   *code.DiffState
}

// New makes the history of doc; hub is nil for a vault that is not
// synced.
func New(env notes.Env, client func() API, hub func() HubAPI, doc *notes.Document) *View {
	v := &View{env: env, api: client, hub: hub, doc: doc, selected: -1}
	v.list.Selected = &v.selected
	v.Reload()
	return v
}

// Entries are the versions, newest first, then the hub's revisions.
func (v *View) Entries() []Entry { return v.entries }

// Err is why the history could not be read, nil when it could.
func (v *View) Err() error { return v.err }

// Content is the selected entry's content.
func (v *View) Content() string { return v.content }

func (v *View) do(f func(ctx context.Context) error, done func(error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := f(ctx)
		v.env.Post(func() { done(err) })
	}()
}

// Reload reads the versions again.
func (v *View) Reload() {
	c := v.api()
	if c == nil {
		return
	}
	var hub HubAPI
	if v.hub != nil {
		hub = v.hub()
	}
	var entries []Entry
	vault, path := v.doc.Vault(), v.doc.Path()
	v.do(func(ctx context.Context) error {
		versions, err := c.Versions(ctx, &api.NoteVersionsParams{VaultID: vault, Path: path})
		if err != nil {
			return err
		}
		for i := range versions {
			entries = append(entries, Entry{Version: &versions[i]})
		}
		if hub != nil {
			if revs, err := hub.History(ctx, &api.SyncHistoryParams{VaultID: vault, Path: path}); err == nil {
				for i := range revs {
					entries = append(entries, Entry{Revision: &revs[i]})
				}
			}
		}
		return nil
	}, func(err error) {
		v.err, v.loaded = err, true
		if err == nil {
			// The selection is an entry, not a place: a new version shifts them.
			if v.selected >= 0 && v.selected < len(v.entries) {
				v.selected = slices.IndexFunc(entries, v.entries[v.selected].same)
			}
			v.entries = entries
			if v.selected < 0 && len(entries) > 0 {
				v.Select(0)
			}
		}
	})
}

// same reports whether o is the same version or revision as e.
func (e Entry) same(o Entry) bool {
	switch {
	case e.Version != nil && o.Version != nil:
		return e.Version.ID == o.Version.ID
	case e.Revision != nil && o.Revision != nil:
		return e.Revision.Seq == o.Revision.Seq
	}
	return false
}

// Select shows entry i.
func (v *View) Select(i int) {
	if i < 0 || i >= len(v.entries) {
		return
	}
	v.selected = i
	e := v.entries[i]
	c := v.api()
	if c == nil {
		return
	}
	var hub HubAPI
	if v.hub != nil {
		hub = v.hub()
	}
	vault := v.doc.Vault()
	var content string
	v.do(func(ctx context.Context) error {
		switch {
		case e.Version != nil && !e.Version.Deleted:
			got, err := c.ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: vault, Version: e.Version.ID})
			if err != nil {
				return err
			}
			content = got.Content
		case e.Revision != nil && hub != nil:
			got, err := hub.ReadRevision(ctx, &api.SyncReadRevisionParams{VaultID: vault, Seq: e.Revision.Seq})
			if err != nil {
				return err
			}
			content = got.Content
		}
		return nil
	}, func(err error) {
		if v.selected < 0 || v.selected >= len(v.entries) || !v.entries[v.selected].same(e) {
			return
		}
		v.err = err
		v.content = content
		v.source, v.diff = nil, nil
	})
}

// Restore makes the selected version the note's content again, as a new
// version (note.revert).
func (v *View) Restore() {
	if v.selected < 0 || v.selected >= len(v.entries) {
		return
	}
	e := v.entries[v.selected]
	if e.Version == nil || e.Version.Deleted {
		return
	}
	c := v.api()
	if c == nil {
		return
	}
	v.doc.Flush(func() {
		params := &api.NoteRevertParams{VaultID: v.doc.Vault(), Path: v.doc.Path(), Version: e.Version.ID, Base: v.doc.Base()}
		v.do(func(ctx context.Context) error {
			_, err := c.Revert(ctx, params)
			return err
		}, func(err error) {
			v.err = err
			if err == nil {
				v.doc.Reload()
				v.Reload()
			}
		})
	})
}

// View draws the history: the list beside the selected entry.
func (v *View) View(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(280).AlignItems(ui.Stretch).Children(func() {
			if v.err != nil && len(v.entries) == 0 {
				ui.Text(c, "The history could not be read: "+v.err.Error()).TextColor(t.Danger).Padding(12)
				return
			}
			if v.loaded && len(v.entries) == 0 {
				ui.Text(c, "No versions yet").TextColor(t.TextMuted).Padding(12)
				return
			}
			ui.List(c, &v.list, len(v.entries), func(i int) {
				e := v.entries[i]
				row := ui.Column(c).Padding(6, 12).Gap(2).Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(e.Title())
				if i == v.selected {
					row.Background(t.Surface)
				}
				if row.Clicked() {
					v.Select(i)
				}
				row.Children(func() {
					ui.Text(c, e.Title()).SingleLine()
					ui.Text(c, e.Detail()).FontSize(12).TextColor(t.TextMuted).SingleLine()
				})
			}).Grow(1)
		})
		ui.Box(c).Width(1).Background(t.Border)
		ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() { v.detail(c) })
	})
}

func (v *View) detail(c *ui.Context) {
	if v.selected < 0 || v.selected >= len(v.entries) {
		return
	}
	e := v.entries[v.selected]
	ui.Row(c).Gap(8).Padding(8, 12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, e.Title()).Bold().Grow(1).SingleLine()
		label := "Show changes"
		if v.ShowChanges {
			label = "Hide changes"
		}
		if ui.Button(c, label).Clicked() {
			v.ShowChanges = !v.ShowChanges
			v.source, v.diff = nil, nil
		}
		if e.Version != nil && !e.Version.Deleted && ui.Button(c, "Restore this version").Clicked() {
			v.Restore()
		}
		if v.Compare != nil && ui.Button(c, "Compare").Clicked() {
			v.Compare(v.content, "Comparing with "+e.Title())
		}
	})
	if v.ShowChanges {
		if v.diff == nil {
			old := code.NewBuffer(v.content)
			cur := code.NewBuffer(v.doc.Text())
			old.SetLanguage(editor.Language())
			cur.SetLanguage(editor.Language())
			v.diff = &code.DiffState{Old: old, New: cur, Layout: code.DiffUnified, Wrap: true}
		}
		code.DiffView(c, v.diff).Grow(1).FillWidth()
		return
	}
	if v.source == nil {
		buf := code.NewBuffer(v.content)
		buf.SetLanguage(editor.Language())
		v.source = &code.State{Buffer: buf, ReadOnly: true, SoftWrap: true, LineNumbers: code.LineNumbersOff, Label: "Version"}
	}
	code.View(c, v.source).Grow(1).FillWidth()
}
