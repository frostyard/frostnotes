package main

import (
	"context"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	newNote     = "New note"
	quickOpen   = "Quick open"
	vaultSearch = "Search vault"
)

func (w *workspace) showAction(action string) {
	if w.vault.GetUntracked() != 0 {
		w.action.Set(action)
	}
}
func (w *workspace) bindNoteActions() {
	for key, action := range map[string]string{"mod+n": newNote, "mod+o": quickOpen, "mod+shift+f": vaultSearch} {
		w.cx.OnKey(key, func() { w.showAction(action) })
	}
}
func (w *workspace) noteActions() []core.Node {
	button := func(action string, icon func() *core.SVG) core.Node {
		return ui.IconButton{Label: action, Icon: icon, Disabled: func() bool { return w.vault.Get() == 0 }, OnPress: func() { w.showAction(action) }}
	}
	return []core.Node{button(newNote, lucide.FilePlus), button(quickOpen, lucide.FolderOpen), button(vaultSearch, lucide.Search), ui.IconButton{Label: "Refresh vaults and notes", Icon: lucide.RefreshCw, OnPress: w.loadVaults}}
}
func (w *workspace) noteDialog(cx *core.Scope) core.Node {
	action := w.action.Get()
	switch action {
	case newNote:
		return w.createDialog(cx)
	case quickOpen, vaultSearch:
		return w.findDialog(cx, action)
	default:
		return nil
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
	return ui.Dialog{Open: func() bool { return true }, Title: newNote, OnOpenChange: w.closeDialog, Description: "Enter a name or a path relative to this vault.", Children: []core.Node{
		ui.Input{Label: "Note path", Placeholder: "Folder/Note.md", AutoFocus: true, Value: name.Get, OnChange: name.Set, OnSubmit: func(string) { create() }},
		core.Text{ContentFunc: problem.Get, Style: ui.TypeCaption},
	}, Footer: []core.Node{ui.Button{Text: "Cancel", OnPress: func() { w.action.Set("") }}, ui.Button{Text: "Create", OnPress: create, Disabled: func() bool { return busy.Get() || strings.TrimSpace(name.Get()) == "" }}}}
}

func (w *workspace) findDialog(cx *core.Scope, action string) core.Node {
	pick := ui.Command{Label: action, SearchLabel: "Find note", Placeholder: "Find by title or path", Empty: "No matching notes", OnSelect: func(p string) { w.action.Set(""); w.open(p) }}
	if action == quickOpen {
		pick.Items = func() []base.CommandItem {
			files := w.files.Get()
			items := make([]base.CommandItem, 0, len(files))
			for _, f := range files {
				items = append(items, base.CommandItem{ID: f.Path, Label: f.Title, Description: f.Path, Keywords: []string{f.Path}})
			}
			return items
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
	return ui.CommandDialog{Open: func() bool { return true }, OnOpenChange: w.closeDialog, Command: pick}
}
