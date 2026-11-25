package markup

import "strconv"

// Command is one of the formatting commands, as Obsidian names it (the
// MyGo client's table, internal/desktop/commands): its ID, its title in
// the palette, its key in the editor as NSGo writes keys ("mod+b"; ""
// for none), and the edit it makes of a note's text and selections.
type Command struct {
	ID, Title, Keys string
	Edit            func(text string, sels []Sel) Edit
}

// Commands are the formatting commands that edit the note's text alone, in
// the palette's order. The commands that ask the editor itself (find,
// indent, undo) or the app (following a link) are the client's.
func Commands() []Command {
	mark := func(m string) func(string, []Sel) Edit {
		return func(t string, s []Sel) Edit { return ToggleMark(t, s, m) }
	}
	pair := func(before, after string) func(string, []Sel) Edit {
		return func(t string, s []Sel) Edit { return InsertPair(t, s, before, after) }
	}
	out := []Command{
		{ID: "editor:toggle-bold", Title: "Toggle bold", Keys: "mod+b", Edit: mark("**")},
		{ID: "editor:toggle-italics", Title: "Toggle italics", Keys: "mod+i", Edit: mark("*")},
		{ID: "editor:toggle-strikethrough", Title: "Toggle strikethrough", Edit: mark("~~")},
		{ID: "editor:toggle-highlight", Title: "Toggle highlight", Edit: mark("==")},
		{ID: "editor:toggle-code", Title: "Toggle code", Edit: mark("`")},
		{ID: "editor:insert-link", Title: "Insert Markdown link", Keys: "mod+k", Edit: InsertLink},
		{ID: "editor:toggle-checklist-status", Title: "Toggle checkbox status", Keys: "mod+l", Edit: ToggleChecklist},
		{ID: "editor:toggle-comments", Title: "Toggle comment", Keys: "mod+/", Edit: ToggleComment},
		{ID: "editor:delete-paragraph", Title: "Delete paragraph", Keys: "mod+d", Edit: DeleteParagraph},
		{ID: "editor:set-heading-0", Title: "Remove heading", Edit: func(t string, s []Sel) Edit { return SetHeading(t, s, 0) }},
		{ID: "editor:insert-wikilink", Title: "Add internal link", Edit: pair("[[", "]]")},
		{ID: "editor:insert-embed", Title: "Add embed", Edit: pair("![[", "]]")},
		{ID: "editor:insert-tag", Title: "Add tag", Edit: pair("#", "")},
		{ID: "editor:toggle-blockquote", Title: "Toggle blockquote", Edit: ToggleBlockquote},
		{ID: "editor:toggle-bullet-list", Title: "Toggle bullet list", Edit: ToggleBulletList},
		{ID: "editor:toggle-numbered-list", Title: "Toggle numbered list", Edit: ToggleNumberedList},
	}
	for level := 1; level <= 6; level++ {
		n := strconv.Itoa(level)
		out = append(out, Command{ID: "editor:set-heading-" + n, Title: "Set heading " + n,
			Edit: func(t string, s []Sel) Edit { return SetHeading(t, s, level) }})
	}
	return out
}
