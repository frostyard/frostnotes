// Package commands is the desktop app's table of commands
// (docs/specs/ui.md#keyboard-map): each command's ID, as Obsidian names it,
// title, group, icon and keys. It is plain data: the window gives each
// command its action and binds its keys (frostui's command table, from
// v0.2.0). A test holds it to the spec's keyboard map.
package commands

import "github.com/egoist/mygo/ui"

// Scope says where a key works.
type Scope uint8

// The scopes, as frostui's KeyScope.
const (
	// OutsideEditors keys step aside while a text editor has the focus.
	OutsideEditors Scope = iota
	// Everywhere keys work wherever the focus is.
	Everywhere
	// InEditor keys are the note editor's own: the table lists them for
	// the palette's key caps, and the editor binds them.
	InEditor
)

// Key is a key of a command.
type Key struct {
	Mods  ui.Modifiers
	Key   ui.Key
	Scope Scope
}

// Spec is one command.
type Spec struct {
	ID, Title, Group, Icon string
	Keys                   []Key
	// Unlisted commands have keys but no entry in the palette.
	Unlisted bool
}

// The groups of the palette.
const (
	GroupEditor    = "Editor"
	GroupFiles     = "Files"
	GroupWorkspace = "Workspace"
	GroupApp       = "Frostnotes"
)

func ctrl(k ui.Key) Key      { return Key{Mods: ui.Cmd, Key: k} }
func ctrlShift(k ui.Key) Key { return Key{Mods: ui.Cmd | ui.Shift, Key: k} }
func ctrlAlt(k ui.Key) Key   { return Key{Mods: ui.Cmd | ui.Alt, Key: k} }
func plain(k ui.Key) Key     { return Key{Key: k} }
func inEditor(k Key) Key     { k.Scope = InEditor; return k }
func everywhere(k Key) Key   { k.Scope = Everywhere; return k }
func keys(ks ...Key) []Key   { return ks }
func edit(k ui.Key) []Key    { return keys(inEditor(ctrl(k))) }

// Table lists every command of the app, in the palette's order.
func Table() []Spec {
	t := []Spec{
		{ID: "switcher:open", Title: "Open quick switcher", Group: GroupApp, Icon: "system-search", Keys: keys(everywhere(ctrl(ui.KeyO)))},
		{ID: "command-palette:open", Title: "Open command palette", Group: GroupApp, Icon: "utilities-terminal",
			Keys: keys(everywhere(ctrl(ui.KeyP)), ctrl(ui.KeyK), everywhere(ctrlShift(ui.KeyP)))},
		{ID: "file-explorer:new-file", Title: "Create new note", Group: GroupFiles, Icon: "document-new", Keys: keys(everywhere(ctrl(ui.KeyN)))},
		{ID: "file-explorer:new-file-in-new-pane", Title: "Create new note to the right", Group: GroupFiles, Icon: "document-new", Keys: keys(everywhere(ctrlShift(ui.KeyN)))},
		{ID: "daily-notes", Title: "Open today's daily note", Group: GroupFiles, Icon: "x-office-calendar"},
		{ID: "insert-template", Title: "Insert template", Group: GroupEditor, Icon: "edit-paste"},
		{ID: "workspace:new-tab", Title: "New tab", Group: GroupWorkspace, Icon: "list-add", Keys: keys(everywhere(ctrl(ui.KeyT)))},
		{ID: "workspace:close", Title: "Close current tab", Group: GroupWorkspace, Icon: "window-close", Keys: keys(everywhere(ctrl(ui.KeyW)))},
		{ID: "workspace:close-others", Title: "Close all other tabs", Group: GroupWorkspace},
		{ID: "workspace:close-window", Title: "Close window", Group: GroupWorkspace, Keys: keys(everywhere(ctrlShift(ui.KeyW)))},
		{ID: "workspace:undo-close-pane", Title: "Undo close tab", Group: GroupWorkspace, Keys: keys(everywhere(ctrlShift(ui.KeyT)))},
		{ID: "workspace:next-tab", Title: "Go to next tab", Group: GroupWorkspace, Keys: keys(everywhere(ctrl(ui.KeyTab)), everywhere(ctrl(ui.KeyPageDown)))},
		{ID: "workspace:previous-tab", Title: "Go to previous tab", Group: GroupWorkspace, Keys: keys(everywhere(ctrlShift(ui.KeyTab)), everywhere(ctrl(ui.KeyPageUp)))},
		{ID: "workspace:goto-last-tab", Title: "Go to last tab", Group: GroupWorkspace, Keys: keys(everywhere(ctrl(ui.Key9)))},
		{ID: "workspace:split-vertical", Title: "Split right", Group: GroupWorkspace},
		{ID: "workspace:split-horizontal", Title: "Split down", Group: GroupWorkspace},
		{ID: "workspace:toggle-pin", Title: "Toggle pin", Group: GroupWorkspace},
		{ID: "workspace:edit-file-title", Title: "Rename file", Group: GroupFiles, Keys: keys(everywhere(plain(ui.KeyF2)))},
		{ID: "app:go-back", Title: "Navigate back", Group: GroupWorkspace, Icon: "go-previous", Keys: keys(everywhere(ctrlAlt(ui.KeyLeft)))},
		{ID: "app:go-forward", Title: "Navigate forward", Group: GroupWorkspace, Icon: "go-next", Keys: keys(everywhere(ctrlAlt(ui.KeyRight)))},
		{ID: "app:toggle-left-sidebar", Title: "Toggle left sidebar", Group: GroupWorkspace},
		{ID: "app:toggle-right-sidebar", Title: "Toggle right sidebar", Group: GroupWorkspace},
		{ID: "app:open-settings", Title: "Open settings", Group: GroupApp, Icon: "open-menu", Keys: keys(everywhere(ctrl(ui.KeyComma)))},
		{ID: "app:open-vault", Title: "Open another vault", Group: GroupApp, Icon: "folder-open"},
		{ID: "app:open-help", Title: "Open help", Group: GroupApp, Icon: "dialog-question", Keys: keys(everywhere(plain(ui.KeyF1)))},
		{ID: "frostnotes:reset-workspace-layout", Title: "Reset workspace layout", Group: GroupWorkspace},
		{ID: "sync:view-version-history", Title: "Open version history", Group: GroupFiles, Icon: "document-open"},
		{ID: "markdown:toggle-preview", Title: "Toggle reading view", Group: GroupEditor, Keys: keys(everywhere(ctrl(ui.KeyE)))},
		{ID: "markdown:add-metadata-property", Title: "Add file property", Group: GroupEditor, Keys: keys(everywhere(ctrl(ui.KeySemicolon)))},
		{ID: "global-search:open", Title: "Search in all files", Group: GroupApp, Icon: "system-search", Keys: keys(everywhere(ctrlShift(ui.KeyF)))},
		{ID: "editor:open-search", Title: "Search current file", Group: GroupEditor, Keys: edit(ui.KeyF)},
		{ID: "editor:open-search-replace", Title: "Search & replace in current file", Group: GroupEditor, Keys: edit(ui.KeyH)},
		{ID: "editor:save-file", Title: "Save current file", Group: GroupEditor, Keys: edit(ui.KeyS)},
		{ID: "editor:toggle-bold", Title: "Toggle bold", Group: GroupEditor, Keys: edit(ui.KeyB)},
		{ID: "editor:toggle-italics", Title: "Toggle italics", Group: GroupEditor, Keys: edit(ui.KeyI)},
		{ID: "editor:toggle-strikethrough", Title: "Toggle strikethrough", Group: GroupEditor},
		{ID: "editor:toggle-highlight", Title: "Toggle highlight", Group: GroupEditor},
		{ID: "editor:toggle-code", Title: "Toggle code", Group: GroupEditor},
		{ID: "editor:insert-link", Title: "Insert Markdown link", Group: GroupEditor, Keys: edit(ui.KeyK)},
		{ID: "editor:toggle-checklist-status", Title: "Toggle checkbox status", Group: GroupEditor, Keys: edit(ui.KeyL)},
		{ID: "editor:toggle-comments", Title: "Toggle comment", Group: GroupEditor, Keys: edit(ui.KeySlash)},
		{ID: "editor:delete-paragraph", Title: "Delete paragraph", Group: GroupEditor, Keys: edit(ui.KeyD)},
		{ID: "editor:set-heading", Title: "Set heading", Group: GroupEditor},
		{ID: "editor:set-heading-0", Title: "Remove heading", Group: GroupEditor},
		{ID: "editor:undo", Title: "Undo", Group: GroupEditor},
		{ID: "editor:redo", Title: "Redo", Group: GroupEditor},
		{ID: "editor:insert-wikilink", Title: "Add internal link", Group: GroupEditor},
		{ID: "editor:insert-embed", Title: "Add embed", Group: GroupEditor},
		{ID: "editor:insert-tag", Title: "Add tag", Group: GroupEditor},
		{ID: "editor:toggle-blockquote", Title: "Toggle blockquote", Group: GroupEditor},
		{ID: "editor:toggle-bullet-list", Title: "Toggle bullet list", Group: GroupEditor},
		{ID: "editor:toggle-numbered-list", Title: "Toggle numbered list", Group: GroupEditor},
		{ID: "editor:indent-list", Title: "Indent list", Group: GroupEditor},
		{ID: "editor:unindent-list", Title: "Unindent list", Group: GroupEditor},
		{ID: "editor:attach-file", Title: "Attach file", Group: GroupEditor},
		{ID: "editor:follow-link", Title: "Follow link under cursor", Group: GroupEditor, Keys: keys(inEditor(Key{Mods: ui.Alt, Key: ui.KeyEnter}))},
		{ID: "editor:open-link-in-new-leaf", Title: "Open link under cursor in new tab", Group: GroupEditor, Keys: edit(ui.KeyEnter)},
		{ID: "editor:open-link-in-new-split", Title: "Open link under cursor to the right", Group: GroupEditor, Keys: keys(inEditor(ctrlAlt(ui.KeyEnter)))},
		{ID: "editor:open-link-in-new-window", Title: "Open link under cursor in new window", Group: GroupEditor, Keys: keys(inEditor(Key{Mods: ui.Cmd | ui.Alt | ui.Shift, Key: ui.KeyEnter}))},
		{ID: "graph:open", Title: "Open graph view", Group: GroupApp, Keys: keys(everywhere(ctrl(ui.KeyG))), Unlisted: true},
		{ID: "window:zoom-in", Title: "Zoom in", Group: GroupApp, Icon: "zoom-in", Keys: keys(everywhere(ctrl(ui.KeyEqual)))},
		{ID: "window:zoom-out", Title: "Zoom out", Group: GroupApp, Icon: "zoom-out", Keys: keys(everywhere(ctrl(ui.KeyMinus)))},
		{ID: "window:reset-zoom", Title: "Reset zoom", Group: GroupApp, Keys: keys(everywhere(ctrl(ui.Key0)))},
	}
	digits := []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8}
	for i, k := range digits {
		n := string(rune('1' + i))
		t = append(t, Spec{ID: "workspace:goto-tab-" + n, Title: "Go to tab " + n, Group: GroupWorkspace, Keys: keys(everywhere(ctrl(k)))})
	}
	for level := 1; level <= 6; level++ {
		n := string(rune('0' + level))
		t = append(t, Spec{ID: "editor:set-heading-" + n, Title: "Set heading " + n, Group: GroupEditor})
	}
	return t
}
