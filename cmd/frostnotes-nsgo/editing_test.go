//go:build linux && integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// openNote opens the note named name (its tree row's label) in source mode
// and waits until it is saved, with path its file.
func openNote(t *testing.T, app *nsgotest.App, name, path string) {
	t.Helper()
	app.WaitFor("label:"+name, 10*time.Second)
	app.Press("label:" + name)
	app.WaitFor("text:"+path+" · Saved", 10*time.Second)
}

// newLine puts the caret on a new line at the note's end.
func newLine(app *nsgotest.App) {
	app.Focus(source)
	app.Key("mod+end")
	app.Key("shift+enter")
}

// TestCompletionsInSourceMode completes a note after [[ in the vault's link
// format, a heading after [[note#, a block after [[note#^ and a tag after
// #, each accepted with Enter.
func TestCompletionsInSourceMode(t *testing.T) {
	app, _ := startVault(t, map[string]string{
		"Ideas.md":      "Ideas",
		"Garden.md":     "# Garden\n## Beds\nquince ^b1\n",
		"Plants/Fig.md": "#fruit #tree\n",
		"Plan.md":       "#tree\n",
	}, 1150, 760)
	openNote(t, app, "Ideas", "Ideas.md")
	newLine(app)
	app.Type(source, "[[")
	app.Type(source, "Gar")
	// The list's row, beside the tree's.
	waitTimers(t, app, 10*time.Second, func() bool { return len(app.Query("text:Garden")) > 1 })
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "Ideas\n[[Garden]]")

	newLine(app)
	app.Type(source, "[[Garden#Be")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:Beds") })
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "Ideas\n[[Garden]]\n[[Garden#Beds]]")

	newLine(app)
	app.Type(source, "[[Garden#^")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:b1") })
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "Ideas\n[[Garden]]\n[[Garden#Beds]]\n[[Garden#^b1]]")

	newLine(app)
	app.Type(source, "#")
	app.Type(source, "tr")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:tree") })
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "Ideas\n[[Garden]]\n[[Garden#Beds]]\n[[Garden#^b1]]\n#tree")
}

// TestFindAndReplaceInTheNote finds in the note from the editor's mod+F,
// with case, whole words and regular expressions, replaces every match as
// 1 step of undo, and closes with Escape.
func TestFindAndReplaceInTheNote(t *testing.T) {
	app, vault := startVault(t, map[string]string{"Fruit.md": "fig and Fig and figs\nfig\n"}, 1150, 760)
	openNote(t, app, "Fruit", "Fruit.md")
	app.Focus(source)
	app.Key("mod+f")
	const field = "label:Find in note"
	app.WaitUntil(5*time.Second, func() bool { return app.Exists(field) && app.Find(field).Focused })
	app.Type(field, "fig")
	app.WaitFor("text:4 results", 5*time.Second)
	app.Submit(field)
	app.WaitFor("text:1 of 4", 5*time.Second)
	app.Press("label:Next match")
	app.WaitFor("text:2 of 4", 5*time.Second)
	app.Press("label:Match case")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:3 results") || app.Exists("text:1 of 3") })
	app.Press("label:Match whole word")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:2 results") || app.Exists("text:1 of 2") })
	app.Press("label:Use regular expression")
	app.Clear(field)
	app.Type(field, "fig(")
	app.WaitFor("text:Not a regular expression", 5*time.Second)
	app.Clear(field)
	app.Type(field, "f(i)g")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists("text:2 results") || app.Exists("text:1 of 2") })
	app.Key("mod+h")
	app.Type("label:Replace with", "p${1}e")
	app.Press("label:Replace all")
	waitSource(t, app, 5*time.Second, "pie and Fig and figs\npie")
	app.Key("escape")
	app.WaitForGone(field, 5*time.Second)
	app.Key("mod+z")
	waitSource(t, app, 5*time.Second, "fig and Fig and figs\nfig")
	waitTimers(t, app, 10*time.Second, func() bool {
		got, err := os.ReadFile(filepath.Join(vault, "Fruit.md"))
		return err == nil && string(got) == "fig and Fig and figs\nfig\n"
	})
}

// TestFormattingCommandsInSourceMode runs Obsidian's formatting commands by
// their keys in the editor, where mod+K inserts a link and not the palette,
// and from the palette.
func TestFormattingCommandsInSourceMode(t *testing.T) {
	app, _ := startVault(t, map[string]string{"Draft.md": "word\nitem\ngone\nnote"}, 1150, 760)
	openNote(t, app, "Draft", "Draft.md")
	app.Focus(source)
	app.Key("mod+home")
	app.Key("shift+end")
	app.Key("mod+b")
	waitSource(t, app, time.Second, "**word**\nitem\ngone\nnote")
	app.Key("mod+b")
	waitSource(t, app, time.Second, "word\nitem\ngone\nnote")
	app.Key("mod+k")
	waitSource(t, app, time.Second, "[word]()\nitem\ngone\nnote")
	if app.Exists("label:Find note") {
		t.Fatal("mod+K in the editor opened the palette")
	}
	app.Key("down")
	app.Key("mod+l")
	waitSource(t, app, time.Second, "[word]()\n- [ ] item\ngone\nnote")
	app.Key("mod+l")
	waitSource(t, app, time.Second, "[word]()\n- [x] item\ngone\nnote")
	app.Key("down")
	app.Key("mod+d")
	waitSource(t, app, time.Second, "[word]()\n- [x] item\nnote")
	app.Key("mod+/")
	waitSource(t, app, time.Second, "[word]()\n- [x] item\n%%note%%")
	// Each command is 1 step of undo.
	app.Key("mod+z")
	waitSource(t, app, time.Second, "[word]()\n- [x] item\nnote")
	// From the palette, which mod+O opens from the editor.
	app.Key("mod+o")
	app.WaitFor("label:Find note", 5*time.Second)
	app.Type("label:Find note", "Set heading 2")
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "[word]()\n- [x] item\n## note")
	app.Key("mod+o")
	app.WaitFor("label:Find note", 5*time.Second)
	app.Type("label:Find note", "Toggle blockquote")
	app.Key("enter")
	waitSource(t, app, 5*time.Second, "[word]()\n- [x] item\n> ## note")
}

// TestSmartListsAndMarksInSourceMode continues a list, a numbered list, a
// task and a quote with Enter, ends one with Enter on an empty item, takes
// a marker away with Backspace, and wraps a selection in a mark typed over
// it, which 1 undo takes back.
func TestSmartListsAndMarksInSourceMode(t *testing.T) {
	app, _ := startVault(t, map[string]string{"List.md": "- one"}, 1150, 760)
	openNote(t, app, "List", "List.md")
	app.Focus(source)
	app.Key("mod+end")
	app.Key("enter")
	app.Type(source, "two")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n- ")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n")
	app.Type(source, "1. a")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n1. a\n2. ")
	app.Key("backspace")
	waitSource(t, app, time.Second, "- one\n- two\n1. a\n")
	app.Type(source, "- [x] done")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n1. a\n- [x] done\n- [ ] ")
	app.Key("shift+home")
	app.Type(source, "> said")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n1. a\n- [x] done\n> said\n> ")
	app.Key("backspace")
	app.Key("backspace")
	app.Type(source, "plain")
	app.Key("enter")
	waitSource(t, app, time.Second, "- one\n- two\n1. a\n- [x] done\n> said\nplain\n")
	app.Key("up")
	app.Key("end")
	app.Key("shift+home")
	app.Type(source, "*")
	waitSource(t, app, 5*time.Second, "- one\n- two\n1. a\n- [x] done\n> said\n*plain*\n")
	app.Key("mod+z")
	waitSource(t, app, 5*time.Second, "- one\n- two\n1. a\n- [x] done\n> said\nplain\n")
}

// TestFollowingLinksInSourceMode follows a link to a note's heading with
// Alt+Enter, a heading of the note itself, a tag to the vault's search, and
// with Ctrl+click a link to a note that is not there, which makes it.
func TestFollowingLinksInSourceMode(t *testing.T) {
	app, vault := startVault(t, map[string]string{
		"Start.md":  "See [[Garden#Beds]] and #fruit\n[[#Top]]\n# Top\nSeeds: [[Seeds]]\n",
		"Garden.md": "# Garden\nintro\n## Beds\nquince\n",
	}, 1150, 760, nsgotest.Idiom(core.IdiomDesktop)) // the editor's desktop presses, Ctrl+click
	openNote(t, app, "Start", "Start.md")
	app.Focus(source)
	app.Key("mod+home")
	for range 7 {
		app.Key("right")
	}
	app.Key("alt+enter")
	app.WaitFor("text:Garden.md · Saved", 10*time.Second)
	// The caret is at the heading's line.
	waitTimers(t, app, 5*time.Second, func() bool { return app.Exists(source) && app.Find(source).Focused })
	app.Type(source, "X")
	waitSource(t, app, 5*time.Second, "# Garden\nintro\nX## Beds\nquince")
	app.Key("mod+z")

	openNote(t, app, "Start", "Start.md")
	app.Focus(source)
	app.Key("mod+home")
	app.Key("down")
	app.Key("right")
	app.Key("right")
	app.Key("right")
	app.Key("alt+enter")
	app.Type(source, "Y")
	waitSource(t, app, 5*time.Second, "See [[Garden#Beds]] and #fruit\n[[#Top]]\nY# Top\nSeeds: [[Seeds]]")
	app.Key("mod+z")
	waitSource(t, app, 5*time.Second, "See [[Garden#Beds]] and #fruit\n[[#Top]]\n# Top\nSeeds: [[Seeds]]")

	// A tag searches the vault for it.
	app.Key("mod+home")
	app.Key("end")
	app.Key("left")
	app.Key("alt+enter")
	app.WaitFor("label:Find note", 5*time.Second)
	if got := app.Find("label:Find note").Text; got != "tag:#fruit" {
		t.Errorf("the search holds %q", got)
	}
	waitTimers(t, app, 10*time.Second, func() bool { return strings.Contains(app.Dump(), "Start.md · ") })
	// Escape empties the search, then closes it.
	app.Key("escape")
	app.Key("escape")
	app.WaitForGone("label:Find note", 5*time.Second)

	// Ctrl+click on a link to a note that is not there makes the note.
	rows := app.Query("id:editor-text >> text:Seeds: [[Seeds]]")
	if len(rows) != 1 {
		t.Fatalf("no row of the link\n%s", app.Dump())
	}
	box, row := app.Find("id:editor-text").Bounds(), rows[0].Bounds()
	width := row.Size.Width / core.Points(len("Seeds: [[Seeds]]"))
	at := core.Point{X: row.Origin.X - box.Origin.X + width*11.5, Y: row.Origin.Y - box.Origin.Y + row.Size.Height/2}
	app.PressAtWith("id:editor-text", at, core.Modifiers{Control: true})
	app.WaitFor("text:Seeds.md · Saved", 10*time.Second)
	if _, err := os.Stat(filepath.Join(vault, "Seeds.md")); err != nil {
		t.Fatal(err)
	}
}

// TestInlineTitleRenamesTheNote renames a note by its title: Enter asks
// whether to update the links, as the vault does not update them by
// itself, and the note and its links move; an empty title goes back.
func TestInlineTitleRenamesTheNote(t *testing.T) {
	app, vault := startVault(t, map[string]string{"First.md": "First", "Second.md": "See [[First]]"}, 1150, 760)
	openNote(t, app, "First", "First.md")
	const title = "label:Note title"
	if got := app.Find(title).Text; got != "First" {
		t.Fatalf("the title is %q", got)
	}
	app.Clear(title)
	app.Submit(title)
	app.WaitUntil(5*time.Second, func() bool { return app.Find(title).Text == "First" })
	app.Clear(title)
	app.Type(title, "Orchard")
	app.Submit(title)
	app.WaitFor("text:"+updateLinks, 5*time.Second)
	app.Press("label:Update")
	app.WaitFor("text:Orchard.md · Saved", 10*time.Second)
	if got := app.Find(title).Text; got != "Orchard" {
		t.Errorf("the title is %q", got)
	}
	waitTimers(t, app, 10*time.Second, func() bool {
		got, err := os.ReadFile(filepath.Join(vault, "Second.md"))
		return err == nil && string(got) == "See [[Orchard]]"
	})
	if _, err := os.Stat(filepath.Join(vault, "First.md")); !os.IsNotExist(err) {
		t.Errorf("First.md is still there: %v", err)
	}
}

// TestSeveralCaretsInSourceMode adds carets with NSGo's mod+Alt+Down, which
// no key of the app takes, and types at each.
func TestSeveralCaretsInSourceMode(t *testing.T) {
	app, _ := startVault(t, map[string]string{"Rows.md": "a\nb\nc"}, 1150, 760)
	openNote(t, app, "Rows", "Rows.md")
	app.Focus(source)
	app.Key("mod+home")
	app.Key("mod+alt+down")
	app.Key("mod+alt+down")
	app.Type(source, "x")
	waitSource(t, app, time.Second, "xa\nxb\nxc")
	app.Key("escape")
	app.Type(source, "!")
	waitSource(t, app, time.Second, "xa\nxb\nx!c")
}
