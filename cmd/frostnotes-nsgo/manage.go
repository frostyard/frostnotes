package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
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

// manageDialog asks about the open note's move or its trash. A move is a
// form, with the note's new path. The trash is a confirmation.
func (w *workspace) manageDialog(cx *core.Scope, action string, d *notes.Document) core.Node {
	from := d.Path()
	shownBase, wasDirty := d.Base(), d.Unsaved()
	if action == trashNote {
		return w.destroy(cx, fmt.Sprintf("Move %q to the trash?", from), "The note can be restored from retained version history.", "Trash note", func() {
			w.manage(d, from, "", false, shownBase, wasDirty, func(bool) {}, w.problem.Set, func() {})
		})
	}
	dest := core.NewSignal(cx, from)
	links := core.NewSignal(cx, true)
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	changed := func() bool { to := strings.TrimSpace(dest.Get()); return to != "" && to != from }
	submit := func() {
		to := strings.TrimSpace(dest.GetUntracked())
		if busy.GetUntracked() || to == "" || to == from {
			return
		}
		if !strings.HasSuffix(strings.ToLower(to), ".md") {
			to += ".md"
		}
		w.manage(d, from, to, links.GetUntracked(), shownBase, wasDirty, func(on bool) {
			if !cx.Disposed() {
				busy.Set(on)
			}
		}, func(text string) {
			if !cx.Disposed() {
				problem.Set(text)
			}
		}, func() { w.action.Set("") })
	}
	path := field("Destination path", "", dest, busy.Get, submit)
	path.AutoFocus = true
	return w.form(cx, form{title: moveNote, detail: "Change the name or enter a destination folder and name.",
		fields:  []core.Node{path, ui.Checkbox{Text: "Update links to this note", Checked: links.Get, OnChange: links.Set, Disabled: busy.Get}},
		problem: problem.Get, busy: busy.Get, ready: changed, confirm: "Move", submit: submit})
}

// manage moves the note d from its path to to, or, with to empty, moves it
// to the trash. It saves the note first. busy follows the action, fail
// says why it did not happen, and done follows its success.
func (w *workspace) manage(d *notes.Document, from, to string, update bool, shownBase string, wasDirty bool, busy func(bool), fail func(string), done func()) {
	id, move := d.Vault(), to != ""
	if d != w.doc.GetUntracked() || d.Path() != from || !d.Loaded() || d.ReadOnly() != "" || needsRecovery(d) {
		fail("The note changed. Reopen this action from the current note.")
		return
	}
	c := w.session.Client()
	if c == nil {
		fail("Reconnect to manage this note.")
		return
	}
	if !move && !wasDirty && shownBase != d.Base() {
		fail("This note changed after the confirmation opened. Review its current contents and try again.")
		return
	}
	busy(true)
	fail("")
	thaw := d.Freeze("Saving before changing the note’s path.")
	d.Flush(func() {
		if d.Unsaved() || d.Path() != from || w.doc.GetUntracked() != d || c != w.session.Client() || d.Notice() != nil {
			thaw()
			busy(false)
			fail("The note changed or could not be saved. Check its save status before trying again.")
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
				busy(false)
				if err != nil {
					fail(err.Error())
					return
				}
				done()
				w.loadFiles()
			})
		}()
	})
}

func (w *workspace) restoreDialog(cx *core.Scope) core.Node {
	id := w.vault.GetUntracked()
	want := core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	restore := func() {
		p := strings.TrimSpace(want.GetUntracked())
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
	input := field("Deleted note path", "Folder/Note.md", want, busy.Get, restore)
	input.AutoFocus = true
	fields := []core.Node{input}
	if recent := slices.Clone(w.removed[id]); len(recent) > 0 {
		rows := make([]core.Node, len(recent))
		for i, p := range recent {
			rows[i] = ui.FormRow{Title: p, TitleMaxLines: 1, OnPress: func() { want.Set(p) }}
		}
		// A long list scrolls inside the dialog.
		fields = append(fields, core.Scroll{Style: core.Style{MaxHeight: core.Pt(210)}, Children: []core.Node{ui.FormSection{Title: "Recently removed", Children: rows}}})
	}
	return w.form(cx, form{title: restoreNote, detail: "Enter the original vault-relative path to restore its last saved contents.", fields: fields,
		problem: problem.Get, busy: busy.Get, ready: func() bool { return strings.TrimSpace(want.Get()) != "" }, confirm: "Restore", submit: restore})
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
