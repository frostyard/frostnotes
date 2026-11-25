//go:build linux && integration

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/markup"
	"github.com/frostyard/frostnotes/internal/desktop/prefs"
	"github.com/nsgo-dev/nsgo/base/editor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const source = "label:Markdown source"

// sourceText is the text source mode shows: its rows in view, in order,
// a row that shows nothing standing for an empty line. Headless an empty
// row shows no text view, and on a target it does, so the text has no
// line break at its end either way. Each row is 1 line, which holds for
// lines shorter than the editor is wide.
func sourceText(app *nsgotest.App) string {
	rows := app.Query("id:editor-text >> text:/.*/")
	if len(rows) == 0 {
		return ""
	}
	slices.SortFunc(rows, func(a, b *nsgotest.View) int { return int(a.Bounds().Origin.Y - b.Bounds().Origin.Y) })
	top, height := rows[0].Bounds().Origin.Y, rows[0].Bounds().Size.Height
	var lines []string
	for _, r := range rows {
		at := int((r.Bounds().Origin.Y - top + height/2) / height)
		for len(lines) < at {
			lines = append(lines, "")
		}
		lines = append(lines, r.Text)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// waitSource waits until source mode shows text, its line breaks at the
// end aside.
func waitSource(t *testing.T, app *nsgotest.App, timeout time.Duration, text string) {
	t.Helper()
	want := strings.TrimRight(text, "\n")
	deadline := time.Now().Add(timeout)
	for got := sourceText(app); got != want; got = sourceText(app) {
		if time.Now().After(deadline) {
			t.Fatalf("source mode shows %q, want %q", got, want)
		}
		app.Advance(100 * time.Millisecond)
	}
}

// replaceSource types text over the note's whole text in source mode, a
// line at a time, with Shift+Enter between, which the editor binds to a
// plain line break: Enter continues a list (smart lists). On a
// target, an input method's commit of a line break reaches the editor's
// 1-line field, which turns it into a space on GTK, and a line break in
// typed text left the line before in that field, typed again after it
// (NSGo v0.3.0, 2026-11-25).
func replaceSource(app *nsgotest.App, text string) {
	app.Focus(source)
	app.Key("mod+a")
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			app.Key("shift+enter")
		}
		if line != "" {
			app.Type(source, line)
		}
	}
}

// typeAtEnd types text at the note's end in source mode.
func typeAtEnd(app *nsgotest.App, text string) {
	app.Focus(source)
	app.Key("mod+end")
	app.Type(source, text)
}

// otherWriter is a second client of the test's notesd, as another window,
// a device's pull or an agent writes, and the vault's id.
func otherWriter(t *testing.T) (*api.Client, int64) {
	t.Helper()
	c, _, err := api.Dial(t.Context(), os.Getenv("FROSTNOTES_SOCKET"), "other-writer")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	vs, err := c.Vault().List(t.Context(), &api.VaultListParams{})
	if err != nil || len(vs) == 0 {
		t.Fatalf("vaults %v, %v", vs, err)
	}
	return c, vs[0].ID
}

// write writes content to a note as another writer, from the note's
// current version.
func write(t *testing.T, c *api.Client, vault int64, path, content string) {
	t.Helper()
	n, err := c.Note().Read(t.Context(), &api.NoteReadParams{VaultID: vault, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: vault, Path: path, Base: n.Hash, Content: content}); err != nil {
		t.Fatal(err)
	}
}

// TestUndoNeverTakesBackAnotherWritersChange holds the undo rule of
// ADR-0022 in source mode (ADR-0049, decision 3): another writer's change,
// and a merge of it with what is typed, reach the editor outside its undo
// history. Ctrl+Z after either leaves the other writer's text, and takes
// back only what was typed after it.
func TestUndoNeverTakesBackAnotherWritersChange(t *testing.T) {
	app, vault := startVault(t, map[string]string{"Garden.md": "# Garden\nquince\n"}, 1150, 760)
	app.WaitFor("label:Garden", 10*time.Second)
	app.Press("label:Garden")
	app.WaitFor("text:Garden.md · Saved", 10*time.Second)
	c, id := otherWriter(t)

	// Typing, saved, then another writer's change of the saved note.
	typeAtEnd(app, "fig")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:Garden.md · Saved") })
	write(t, c, id, "Garden.md", "# Garden\nquince\nfig\nmint from the phone\n")
	waitSource(t, app, 10*time.Second, "# Garden\nquince\nfig\nmint from the phone\n")
	// The caret stayed after what was typed, on its line.
	app.Type(source, "s")
	waitSource(t, app, time.Second, "# Garden\nquince\nfigs\nmint from the phone\n")
	app.Key("mod+z")
	waitSource(t, app, time.Second, "# Garden\nquince\nfig\nmint from the phone\n")
	// The history ended at the other writer's change: nothing before it
	// comes back, and the change stays.
	for range 3 {
		app.Key("mod+z")
	}
	waitSource(t, app, time.Second, "# Garden\nquince\nfig\nmint from the phone\n")

	// Typing not yet saved when another writer changes the note: notesd
	// merges the two, and the merge comes back as another writer's change.
	typeAtEnd(app, "sorrel")
	write(t, c, id, "Garden.md", "# Garden plan\nquince\nfig\nmint from the phone\n")
	waitTimers(t, app, 10*time.Second, func() bool {
		return sourceText(app) == "# Garden plan\nquince\nfig\nmint from the phone\nsorrel"
	})
	for range 3 {
		app.Key("mod+z")
	}
	waitSource(t, app, time.Second, "# Garden plan\nquince\nfig\nmint from the phone\nsorrel")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:Garden.md · Saved") })
	got, err := os.ReadFile(filepath.Join(vault, "Garden.md"))
	if err != nil || string(got) != "# Garden plan\nquince\nfig\nmint from the phone\nsorrel" {
		t.Fatalf("the note on disk is %q, %v", got, err)
	}
}

// TestAnotherWritersChangeKeepsTheCaretAndTheView checks what source mode
// keeps when it starts again on another writer's change above what is in
// view: the caret stays on its text, and the lines in view stay in view
// where they were.
func TestAnotherWritersChangeKeepsTheCaretAndTheView(t *testing.T) {
	var b strings.Builder
	for i := range 200 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	text := b.String()
	app, vault := startVault(t, map[string]string{"Long.md": text}, 1150, 760)
	app.WaitFor("label:Long", 10*time.Second)
	app.Press("label:Long")
	app.WaitFor("text:Long.md · Saved", 10*time.Second)
	typeAtEnd(app, "end")
	waitTimers(t, app, 10*time.Second, func() bool { return app.Exists("text:Long.md · Saved") })
	// The first row in view: the editor draws rows above and below it too.
	firstRow := func() (string, core.Points) {
		top := app.Find("id:editor-scroll").Bounds().Origin.Y
		var first *nsgotest.View
		for _, r := range app.Query("id:editor-text >> text:/.*/") {
			if y := r.Bounds().Origin.Y; y >= top && (first == nil || y < first.Bounds().Origin.Y) {
				first = r
			}
		}
		if first == nil {
			t.Fatalf("no row in view\n%s", app.Dump())
		}
		return first.Text, first.Bounds().Origin.Y
	}
	before, y := firstRow()
	if before == "line 0" {
		t.Fatalf("the note's end is not in view\n%s", app.Dump())
	}
	// The rows' box grows by the lines that come.
	height := func() core.Points { return app.Find("id:editor-text").Bounds().Size.Height }
	h := height()
	c, id := otherWriter(t)
	write(t, c, id, "Long.md", "A\nB\nC\n"+text+"end")
	waitTimers(t, app, 10*time.Second, func() bool { return height() > h })
	if got, at := firstRow(); got != before || at != y {
		t.Errorf("the first line in view is %q at %v, was %q at %v", got, at, before, y)
	}
	app.Type(source, "!")
	want := "A\nB\nC\n" + text + "end!"
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := os.ReadFile(filepath.Join(vault, "Long.md"))
		if err == nil && string(got) == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the note on disk ends %q, %v; want it to end %q", got[max(len(got)-40, 0):], err, want[len(want)-40:])
		}
		app.Advance(100 * time.Millisecond)
	}
}

// TestSourceFoldsAHeadingsSection checks the folds of a note in source
// mode: a heading's section folds into the heading, and a fold stays
// folded through another writer's change above it.
func TestSourceFoldsAHeadingsSection(t *testing.T) {
	app, _ := startVault(t, map[string]string{"Plan.md": "Intro\n# Beds\nquince\n## Herbs\nmint\n# Paths\ngravel\n"}, 1150, 760)
	app.WaitFor("label:Plan", 10*time.Second)
	app.Press("label:Plan")
	app.WaitFor("text:Plan.md · Saved", 10*time.Second)
	app.Focus(source)
	app.Key("down")
	app.Key("ctrl+shift+[")
	// A folded line ends in the editor's placeholder.
	waitSource(t, app, time.Second, "Intro\n# Beds ⋯ \n# Paths\ngravel\n")
	c, id := otherWriter(t)
	write(t, c, id, "Plan.md", "A new intro\nIntro\n# Beds\nquince\n## Herbs\nmint\n# Paths\ngravel\n")
	waitSource(t, app, 10*time.Second, "A new intro\nIntro\n# Beds ⋯ \n# Paths\ngravel\n")
	app.Key("ctrl+shift+]")
	waitSource(t, app, time.Second, "A new intro\nIntro\n# Beds\nquince\n## Herbs\nmint\n# Paths\ngravel\n")
}

// TestSourceTakesTheMyGoDefaults holds source mode's preferences to the
// MyGo client's defaults, which the web build cannot import.
func TestSourceTakesTheMyGoDefaults(t *testing.T) {
	d := prefs.Default()
	want := sourcePrefs{
		LineNumbers: d.LineNumbers, IndentGuides: d.IndentGuides, FoldHeadings: d.FoldHeadings, FoldIndent: d.FoldIndent,
		AutoPairBrackets: d.AutoPairBrackets, IndentWithTabs: d.IndentWithTabs, ReadableLineLength: d.ReadableLineLength, TabSize: d.TabSize,
	}
	if got := editorDefaults(); got != want {
		t.Errorf("source mode's defaults are %+v, the MyGo client's %+v", got, want)
	}
}

// TestObsidianHighlighterColorsEachKind checks the app's highlighter gives
// NSGo's editor a kind of its palette for each piece of syntax that has
// one, and none for emphasis's text, which NSGo has no kind for.
func TestObsidianHighlighterColorsEachKind(t *testing.T) {
	lines := []string{"# Title", "See [[Note]] and #tag, **bold**."}
	got := obsidian{}.Highlight(lines, 0, 2)
	want := [][]editor.Token{
		{{Start: 0, End: 7, Kind: "title"}},
		{{Start: 4, End: 12, Kind: "built"}, {Start: 17, End: 21, Kind: "keyword"}, {Start: 23, End: 25, Kind: "comment"}, {Start: 29, End: 31, Kind: "comment"}},
	}
	if !slices.EqualFunc(got, want, func(a, b []editor.Token) bool { return slices.Equal(a, b) }) {
		t.Errorf("tokens %v, want %v", got, want)
	}
	for k := markup.KindFrontmatter; k <= markup.KindRule; k++ {
		if k != markup.KindEmphasis && k != markup.KindStrong && tokenKind(k) == "" {
			t.Errorf("kind %d has no color", k)
		}
	}
}

// TestAppKeysAroundTheEditor finds out whether the app's key bindings take
// Enter, Backspace and mod+V before NSGo's editor does (plan 0011,
// Phase 2: smart lists and paste need it). Of the bindings of a key, the
// newest runs (core.OnKey), and the editor binds its keys as it takes the
// keyboard. So a binding made around the editor before that loses Enter
// and Backspace to the editor's own, and one made after it, in a Show
// that follows the editor's focus, takes all 3 keys first, and the editor
// does nothing with them. mod+V the editor does not bind: a binding takes
// it either way, before the field's paste.
func TestAppKeysAroundTheEditor(t *testing.T) {
	keys := []string{"enter", "backspace", "mod+v"}
	for _, tc := range []struct {
		name       string
		afterFocus bool
		heard      []string
		// texts are the editor's texts after each change.
		texts []string
	}{
		{"bound around the editor", false, []string{"mod+v"}, []string{"ab\n", "ab"}},
		{"bound once the editor has the keyboard", true, keys, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var heard, texts []string
			ref := &editor.Ref{}
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				focused := core.NewSignal(cx, false)
				bind := func(cx *core.Scope) {
					for _, k := range keys {
						cx.OnKeyWith(k, core.KeyOptions{When: core.KeyAlways}, func() { heard = append(heard, k) })
					}
				}
				if !tc.afterFocus {
					bind(cx)
				}
				return core.View{Children: []core.Node{
					editor.Editor{Ref: ref, Label: "Code", DefaultValue: "ab", OnChange: func(s string) { texts = append(texts, s) },
						OnFocus: func() { focused.Set(true) }, OnBlur: func() { focused.Set(false) },
						Style: core.Style{Width: core.Pt(400), Height: core.Pt(300)}},
					core.Show{When: focused.Get, Then: func(cx *core.Scope) core.Node {
						if tc.afterFocus {
							bind(cx)
						}
						return core.Group{}
					}},
				}}
			})
			app.Focus("label:Code")
			app.Key("end")
			for _, k := range keys {
				app.Key(k)
			}
			if !slices.Equal(heard, tc.heard) || !slices.Equal(texts, tc.texts) {
				t.Errorf("the app heard %v and the editor's texts were %q; want %v and %q", heard, texts, tc.heard, tc.texts)
			}
		})
	}
}
