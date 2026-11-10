package history

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"

	"github.com/frostyard/frostnotes/internal/desktop/editor"
)

// Merge is the merge view (specs/ui.md#merge-view): another text (a
// version, a hub revision, or what the editor held when notesd refused
// its base) beside the note's own buffer, which stays editable and saves
// as typing does. Each changed chunk can be taken back from the other
// side. UI thread only.
type Merge struct {
	b     *editor.Binding
	Title string
	State *code.DiffState
	// Done closes the view: the window dismisses the note's notice and,
	// for a hub conflict, resolves it (sync.resolve).
	Done func()
}

// undoKeys leave undo and redo to the window, which runs the note's own:
// MyGo's Undo restores a step's whole text before it, so it would take
// back another writer's change made since (egoist/mygo#1652). The keys are
// the only way to it here: on Linux MyGo's editing menu has no Undo item
// (Windows' has, and would need a guard).
var undoKeys = []code.Binding{
	{Mods: ui.Ctrl, Key: ui.KeyZ, Command: code.NoCommand},
	{Mods: ui.Ctrl | ui.Shift, Key: ui.KeyZ, Command: code.NoCommand},
	{Mods: ui.Ctrl, Key: ui.KeyY, Command: code.NoCommand},
}

// NewMerge makes the merge view of a note's binding against other.
func NewMerge(b *editor.Binding, other, title string) *Merge {
	old := code.NewBuffer(other)
	old.SetLanguage(editor.Language())
	return &Merge{b: b, Title: title, State: &code.DiffState{
		Old: old, New: b.Buffer(), Layout: code.DiffSplit, Editable: true,
		HunkActions: true, Wrap: true, Bindings: undoKeys,
	}}
}

// TakeBack puts the other side's text of chunk i back in the note, as
// the user's edit.
func (m *Merge) TakeBack(i int) bool {
	if i < 0 || i >= len(m.State.Hunks()) {
		return false
	}
	m.State.Revert(i)
	m.b.Sync()
	return true
}

// View draws the merge view.
func (m *Merge) View(c *ui.Context) ui.Element {
	m.b.Sync()
	doc := m.b.Document()
	c.OnWindowShortcut(ui.Cmd, ui.KeyZ, func() { doc.Undo() })
	c.OnWindowShortcut(ui.Cmd|ui.Shift, ui.KeyZ, func() { doc.Redo() })
	c.OnWindowShortcut(ui.Cmd, ui.KeyY, func() { doc.Redo() })
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(8).Padding(8, 12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.Title).Bold().Grow(1).SingleLine()
			n := len(m.State.Hunks())
			ui.Text(c, changes(n)).TextColor(c.Theme().TextMuted)
			if ui.Button(c, "Done").Clicked() && m.Done != nil {
				m.Done()
			}
		})
		code.DiffView(c, m.State).Grow(1).FillWidth()
	})
}

func changes(n int) string {
	if n == 1 {
		return "1 change"
	}
	return strconv.Itoa(n) + " changes"
}
