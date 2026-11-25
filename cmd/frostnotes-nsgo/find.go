package main

import (
	"context"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	newNote     = "New note"
	quickOpen   = "Quick open"
	vaultSearch = "Search vault"
)

// paletteLabel names the palette where the window shows it: the toolbar's
// field, and the menu of a window with no room for the field. The 3 apps
// on the kit name it so.
const paletteLabel = "Search or run a command"

// commandID starts the id of a command in the palette. A note's id is its
// path in the vault, which starts with no colon.
const commandID = ":command:"

// commands are what the palette runs: every action of the toolbar and its
// menu but the palette itself, the editor's 2 modes, and Settings where
// the app has it.
func (w *workspace) commands() []tool {
	out := []tool{{label: newNote, keys: "mod+n", off: w.noVault, run: func() { w.showAction(newNote) }}}
	out = append(out, w.noteTools()...)
	out = append(out, w.vaultTools()...)
	out = append(out, w.restTools()...)
	out = append(out,
		tool{label: "Show source", off: func() bool { return !w.live.Get() }, run: func() { w.live.Set(false) }},
		tool{label: "Show live preview", off: w.live.Get, run: func() { w.live.Set(true) }})
	if w.hasSettings {
		out = append(out, tool{label: "Settings", keys: "mod+,", run: func() { w.settings.Set(true) }})
	}
	return out
}

func (w *workspace) showAction(action string) {
	if w.vault.GetUntracked() != 0 {
		w.action.Set(action)
	}
}

// bindNoteActions binds the app's keys (the kit's standard for an app,
// rule 7): mod+K the palette, which is Quick open with the app's commands
// under the notes, and mod+N the new note. mod+O stays the palette's older
// key.
func (w *workspace) bindNoteActions() {
	for key, action := range map[string]string{"mod+n": newNote, "mod+k": quickOpen, "mod+o": quickOpen, "mod+shift+f": vaultSearch} {
		w.cx.OnKey(key, func() { w.showAction(action) })
	}
}
func (w *workspace) noteDialog(cx *core.Scope) core.Node {
	action := w.action.Get()
	switch action {
	case newNote:
		return w.createDialog(cx)
	case moveNote, trashNote:
		if d := w.doc.Get(); d != nil {
			return w.manageDialog(cx, action, d)
		}
		return nil
	case restoreNote:
		return w.restoreDialog(cx)
	case quickOpen, vaultSearch:
		return w.findDialog(cx, action)
	default:
		return w.vaultDialog(cx, action)
	}
}
func (w *workspace) closeDialog(open bool, _ base.Reason) {
	if !open {
		w.action.Set("")
	}
}

func (w *workspace) createDialog(cx *core.Scope) core.Node {
	name := core.NewSignal(cx, "")
	problem := core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	create := func() {
		p := strings.TrimSpace(name.GetUntracked())
		if p == "" || busy.GetUntracked() {
			return
		}
		if !strings.HasSuffix(strings.ToLower(p), ".md") {
			p += ".md"
		}
		c := w.session.Client()
		if c == nil {
			problem.Set("Reconnect to create a note.")
			return
		}
		id := w.vault.GetUntracked()
		busy.Set(true)
		problem.Set("")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: id, Path: p})
			cx.Post(func() {
				busy.Set(false)
				if err != nil {
					problem.Set(err.Error())
					return
				}
				if id != w.vault.GetUntracked() {
					return
				}
				w.action.Set("")
				w.loadFiles()
				w.open(result.Path)
			})
		}()
	}
	path := field("Note path", "Folder/Note.md", name, nil, create)
	path.AutoFocus = true
	return w.form(cx, form{title: newNote, detail: "Enter a name or a path relative to this vault.", fields: []core.Node{path},
		problem: problem.Get, busy: busy.Get, ready: func() bool { return strings.TrimSpace(name.Get()) != "" }, confirm: "Create", submit: create})
}

func (w *workspace) findDialog(cx *core.Scope, action string) core.Node {
	pick := ui.Command{Label: action, SearchLabel: "Find note", Placeholder: "Find by title or path", Empty: "No matching notes", OnSelect: func(p string) { w.action.Set(""); w.open(p) }}
	if action == quickOpen {
		// The palette: the vault's notes, by title or path, and then what
		// the app can do, by name.
		commands := w.commands()
		pick.Placeholder, pick.Empty = "Find a note or run a command", "No matching notes or commands"
		pick.Items = func() []base.CommandItem {
			files := w.files.Get()
			items := make([]base.CommandItem, 0, len(files)+len(commands))
			for _, f := range files {
				items = append(items, base.CommandItem{ID: f.Path, Label: f.Title, Description: f.Path, Keywords: []string{f.Path}, Group: "Notes"})
			}
			for _, c := range commands {
				if c.off == nil || !c.off() {
					items = append(items, base.CommandItem{ID: commandID + c.label, Label: c.label, Shortcut: c.keys, Group: "Commands"})
				}
			}
			return items
		}
		pick.OnSelect = func(id string) {
			w.action.Set("")
			name, isCommand := strings.CutPrefix(id, commandID)
			if !isCommand {
				w.open(id)
				return
			}
			for _, c := range commands {
				if c.label == name {
					c.run()
				}
			}
		}
	} else {
		query := core.NewSignal(cx, "")
		results := core.NewSignalFunc(cx, []base.CommandItem(nil), nil)
		loading := core.NewSignal(cx, false)
		problem := core.NewSignal(cx, "")
		pick.Placeholder = "Search note contents"
		pick.NoFilter = true
		pick.OnQueryChange = query.Set
		pick.Items = results.Get
		pick.Loading = loading.Get
		pick.Error = problem.Get
		// Each query owns its debounce and its RPC. Closing the palette or
		// typing again disposes both before an old response can replace rows.
		cx.Effect(func() {
			q := strings.TrimSpace(query.Get())
			id := w.vault.Get()
			w.connection.Get()
			c := w.session.Client()
			results.Set(nil)
			problem.Set("")
			loading.Set(false)
			if q == "" {
				return
			}
			if c == nil {
				problem.Set("Reconnect to search the vault.")
				return
			}
			loading.Set(true)
			life := cx.Context()
			cx.AfterFunc(250*time.Millisecond, func() {
				go func() {
					ctx, cancel := context.WithTimeout(life, 15*time.Second)
					defer cancel()
					limit := int64(100)
					hits, err := c.Note().Search(ctx, &api.NoteSearchParams{VaultID: id, Query: q, Limit: &limit})
					cx.Post(func() {
						if life.Err() != nil || c != w.session.Client() {
							return
						}
						loading.Set(false)
						if err != nil {
							problem.Set(err.Error())
							return
						}
						items := make([]base.CommandItem, 0, len(hits))
						for _, h := range hits {
							var snippet strings.Builder
							for _, s := range h.Snippet {
								snippet.WriteString(s.Text)
							}
							items = append(items, base.CommandItem{ID: h.Path, Label: h.Title, Description: h.Path + " · " + snippet.String()})
						}
						results.Set(items)
					})
				}()
			})
		})
	}
	return ui.CommandDialog{Style: core.Style{Background: workspaceui.ColorsOf(cx).Panel()}, Open: func() bool { return true }, OnOpenChange: w.closeDialog, Command: pick}
}
