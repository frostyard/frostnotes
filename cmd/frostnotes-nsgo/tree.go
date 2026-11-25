package main

import (
	"maps"
	"path"
	"slices"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The vault's notes are a tree: a folder holds its folders, then its notes,
// and opens and closes. It is NSGo's tree (the kit's control map: "a tree
// of folders"), in place of a sidebar with a section for each folder's
// path, which showed no folder inside another.

// folder is the tree's key of a folder: its path in the vault, "" for the
// vault itself. A note's key is its path, a string.
type folder string

func (f folder) parent() folder {
	p := path.Dir(string(f))
	if p == "." || p == "/" {
		return ""
	}
	return folder(p)
}

func (f folder) name() string { return path.Base(string(f)) }

// noteTree is what each folder holds, in the order it is shown, and the
// notes' titles.
type noteTree struct {
	roots    []any
	children map[folder][]any
	titles   map[string]string
}

// buildTree makes the tree of a vault's notes. A folder's folders come
// first, by name, then its notes in the order notesd lists them. A folder
// is there because a note is in it or under it: notesd lists notes alone.
func buildTree(files []api.NoteEntry) noteTree {
	t := noteTree{children: map[folder][]any{}, titles: map[string]string{}}
	inside, notes := map[folder][]folder{}, map[folder][]string{}
	seen := map[folder]bool{"": true}
	var add func(folder)
	add = func(dir folder) {
		if seen[dir] {
			return
		}
		seen[dir] = true
		add(dir.parent())
		inside[dir.parent()] = append(inside[dir.parent()], dir)
	}
	for _, f := range files {
		dir := folder(path.Dir(f.Path))
		if dir == "." || dir == "/" {
			dir = ""
		}
		add(dir)
		notes[dir] = append(notes[dir], f.Path)
		t.titles[f.Path] = f.Title
	}
	holds := func(dir folder) []any {
		folders := inside[dir]
		slices.Sort(folders)
		out := make([]any, 0, len(folders)+len(notes[dir]))
		for _, f := range folders {
			out = append(out, f)
		}
		for _, n := range notes[dir] {
			out = append(out, n)
		}
		return out
	}
	for dir := range seen {
		if dir == "" {
			t.roots = holds(dir)
		} else {
			t.children[dir] = holds(dir)
		}
	}
	return t
}

// notesTree is the tree of the vault's notes. A press of a note's row
// opens it, and the row of the note that is open is the chosen one.
func (w *workspace) notesTree(cx *core.Scope) core.Node {
	tree := core.NewMemoFunc(cx, func() noteTree { return buildTree(w.files.Get()) }, func(noteTree, noteTree) bool { return false })
	// A vault's folders start open, and those the user closes stay closed
	// while the vault is shown.
	closed := core.NewSignalFunc(cx, map[folder]bool{}, func(_, _ map[folder]bool) bool { return false })
	cx.Effect(func() {
		w.vault.Get()
		closed.Set(map[folder]bool{})
	})
	selection := ui.NewSelection(cx, false)
	w.chosen = selection
	chosen := func() (string, bool) {
		if keys := selection.Keys(); len(keys) == 1 {
			p, ok := keys[0].(string)
			return p, ok
		}
		return "", false
	}
	// The open note's row is the chosen one. While no note is shown, as
	// after the narrow layout's way back to the tree, the choice is the
	// user's.
	cx.Effect(func() {
		p := w.selectedPath()
		if !w.preview.Get() || p == "" {
			return
		}
		cx.Untrack(func() {
			if now, ok := chosen(); !ok || now != p {
				selection.Set(p)
			}
		})
	})
	// A press of a note's row opens it. A folder's row is only chosen.
	cx.Effect(func() {
		p, ok := chosen()
		if !ok {
			return
		}
		if p != core.Untracked(cx, w.selectedPath) || !w.preview.GetUntracked() {
			w.open(p)
		}
	})
	return ui.Tree{Label: "Notes", TestID: "notes-tree", Style: ui.FillRest, Selection: selection, ToggleLabel: "Expand", RowStyleFunc: workspaceui.SoftRows(cx),
		Roots: func() []any { return tree.Get().roots },
		Children: func(k any) []any {
			if f, ok := k.(folder); ok {
				return tree.Get().children[f]
			}
			return nil
		},
		Expanded: func(k any) bool {
			f, ok := k.(folder)
			return ok && !closed.Get()[f]
		},
		OnExpand: func(k any, open bool) {
			f, ok := k.(folder)
			if !ok {
				return
			}
			next := maps.Clone(closed.GetUntracked())
			if open {
				delete(next, f)
			} else {
				next[f] = true
			}
			closed.Set(next)
		},
		RowLabel: func(k any) string {
			switch v := k.(type) {
			case folder:
				return v.name()
			case string:
				return tree.Get().titles[v]
			}
			return ""
		},
		Row: func(_ *core.Scope, item ui.TreeItem) core.Node {
			return core.Dynamic(func(*core.Scope) core.Node {
				switch v := item.Key().(type) {
				case folder:
					icon := lucide.Folder
					if item.Expanded() {
						icon = lucide.FolderOpen
					}
					return workspaceui.NavLabel{Icon: icon, Text: v.name()}
				case string:
					return workspaceui.NavLabel{Icon: lucide.FileText, Text: tree.Get().titles[v]}
				}
				return nil
			})
		},
		// An empty tree is a vault with no notes only once its notes have
		// been read.
		Empty: core.Dynamic(func(*core.Scope) core.Node {
			if !w.listed.Get() {
				return ui.Skeleton{Lines: 4, Label: "Loading notes"}
			}
			return workspaceui.EmptyState{Icon: lucide.FileText, Title: "No notes yet", Hint: "New note adds the first."}
		})}
}
