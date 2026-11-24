package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	moveNote    = "Rename or move note"
	trashNote   = "Move to trash"
	restoreNote = "Restore deleted note"
)

func (w *workspace) forgetRemoved(id int64, path string) {
	w.removed[id] = slices.DeleteFunc(w.removed[id], func(p string) bool { return path == p })
}
func (w *workspace) rememberRemoved(id int64, path string) {
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		return
	}
	w.forgetRemoved(id, path)
	recent := w.removed[id]
	w.removed[id] = append([]string{path}, recent[:min(49, len(recent))]...)
}

func (w *workspace) manageDialog(cx *core.Scope, action string, d *notes.Document) core.Node {
	id, from := d.Vault(), d.Path()
	shownBase, wasDirty := d.Base(), d.Unsaved()
	dest := core.NewSignal(cx, from)
	links := core.NewSignal(cx, true)
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	move := action == moveNote
	submit := func() {
		if busy.GetUntracked() {
			return
		}
		if d != w.doc.GetUntracked() || d.Path() != from || !d.Loaded() || d.ReadOnly() != "" || needsRecovery(d) {
			problem.Set("The note changed. Reopen this action from the current note.")
			return
		}
		to := strings.TrimSpace(dest.GetUntracked())
		if move && (to == "" || to == from) {
			return
		}
		if move && !strings.HasSuffix(strings.ToLower(to), ".md") {
			to += ".md"
		}
		c := w.session.Client()
		if c == nil {
			problem.Set("Reconnect to manage this note.")
			return
		}
		if !move && !wasDirty && shownBase != d.Base() {
			problem.Set("This note changed after the confirmation opened. Cancel and review its current contents.")
			return
		}
		update := links.GetUntracked()
		busy.Set(true)
		problem.Set("")
		thaw := d.Freeze("Saving before changing the note’s path.")
		d.Flush(func() {
			if d.Unsaved() || d.Path() != from || w.doc.GetUntracked() != d || c != w.session.Client() || d.Notice() != nil {
				thaw()
				busy.Set(false)
				problem.Set("The note changed or could not be saved. Check its save status before trying again.")
				return
			}
			baseHash := d.Base()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				var err error
				if move {
					_, err = c.Note().Move(ctx, &api.NoteMoveParams{VaultID: id, Path: from, To: to, UpdateLinks: &update})
				} else {
					err = c.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: id, Path: from, Base: baseHash})
				}
				// Thaw even when the successful delete already removed its view.
				w.cx.Post(func() {
					thaw()
					if cx.Disposed() {
						return
					}
					busy.Set(false)
					if err != nil {
						problem.Set(err.Error())
						return
					}
					w.action.Set("")
					w.loadFiles()
				})
			}()
		})
	}
	caption, button := "The note can be restored from retained version history.", "Trash note"
	children := []core.Node{core.Text{Content: from}}
	if move {
		caption, button = "Change the name or enter a destination folder and name.", "Move"
		children = []core.Node{ui.Input{Label: "Destination path", Value: dest.Get, OnChange: dest.Set, AutoFocus: true, OnSubmit: func(string) { submit() }}, ui.Checkbox{Text: "Update links to this note", Checked: links.Get, OnChange: links.Set}}
	}
	children = append(children, core.Text{ContentFunc: problem.Get, Style: ui.TypeCaption})
	return ui.Dialog{Open: func() bool { return true }, OnOpenChange: func(open bool, r base.Reason) {
		if !busy.GetUntracked() {
			w.closeDialog(open, r)
		}
	}, Title: action, Description: caption, Children: children, Footer: []core.Node{
		ui.Button{Text: "Cancel", Disabled: busy.Get, OnPress: func() { w.action.Set("") }},
		ui.Button{Text: button, Disabled: func() bool {
			return busy.Get() || move && (strings.TrimSpace(dest.Get()) == "" || strings.TrimSpace(dest.Get()) == from)
		}, OnPress: submit},
	}}
}

func (w *workspace) restoreDialog(cx *core.Scope) core.Node {
	id := w.vault.GetUntracked()
	path := core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	restore := func() {
		p := strings.TrimSpace(path.GetUntracked())
		if p == "" || busy.GetUntracked() {
			return
		}
		if !strings.HasSuffix(strings.ToLower(p), ".md") {
			p += ".md"
		}
		c := w.session.Client()
		if c == nil {
			problem.Set("Reconnect to restore a note.")
			return
		}
		busy.Set(true)
		problem.Set("")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err := restoreDeleted(ctx, c, id, p)
			cx.Post(func() {
				busy.Set(false)
				if err != nil {
					problem.Set(err.Error())
					return
				}
				w.forgetRemoved(id, p)
				w.action.Set("")
				w.loadFiles()
				w.open(p)
			})
		}()
	}
	children := []core.Node{ui.Input{Label: "Deleted note path", Placeholder: "Folder/Note.md", Value: path.Get, OnChange: path.Set, AutoFocus: true, OnSubmit: func(string) { restore() }}}
	recent := slices.Clone(w.removed[id])
	if len(recent) > 0 {
		children = append(children, core.Text{Content: "Recently removed", Style: ui.TypeCaption}, core.List{Style: core.Style{Height: core.Pt(core.Points(min(4, len(recent)) * 34))}, Count: func() int { return len(recent) }, EstimatedSize: 34, Row: func(cx *core.Scope, r core.ListRow) core.Node {
			return ui.Button{TextFunc: func() string { return recent[r.Index()] }, OnPress: func() { path.Set(recent[r.Index()]) }}
		}})
	}
	children = append(children, core.Text{ContentFunc: problem.Get, Style: ui.TypeCaption})
	return ui.Dialog{Open: func() bool { return true }, OnOpenChange: w.closeDialog, Title: restoreNote, Description: "Enter the original vault-relative path to restore its last saved contents.", Children: children, Footer: []core.Node{
		ui.Button{Text: "Cancel", OnPress: func() { w.action.Set("") }}, ui.Button{Text: "Restore", Disabled: func() bool { return busy.Get() || strings.TrimSpace(path.Get()) == "" }, OnPress: restore},
	}}
}

// An empty base asks notesd to restore only while the path is still absent.
// A new file at the old path wins even if it appeared after Versions returned.
func restoreDeleted(ctx context.Context, c *api.Client, id int64, path string) error {
	vs, err := c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: id, Path: path})
	if err != nil {
		return err
	}
	if len(vs) == 0 || !vs[0].Deleted {
		return errors.New("No retained deletion at this path.")
	}
	i := slices.IndexFunc(vs, func(v api.Version) bool { return !v.Deleted && !v.Conflict })
	if i < 0 {
		return errors.New("No retained contents to restore.")
	}
	_, err = c.Note().Revert(ctx, &api.NoteRevertParams{VaultID: id, Path: path, Version: vs[i].ID})
	return err
}
