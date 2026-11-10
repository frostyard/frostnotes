package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// M3's writers evidence (plan 0005), against the native app: the user
// types in the editor while an agent writes through notesd and an outside
// editor saves the file. No token is lost, and the caret stays where the
// user types (plan 0009, Phase 2).
func TestNoTokenIsLostWithThreeWriters(t *testing.T) {
	f := start(t)
	f.Create("Log.md", "typed:\nlog:\n")
	d := f.s.Open(f.Vault, "Log.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	ed := editor.NewNoteEditor(editor.Bind(d))
	tt := ui.NewTester(func(c *ui.Context) { ed.View(c).Fill() }, 800, 600)
	if err := tt.Click("Note"); err != nil {
		t.Fatal(err)
	}
	ed.State.Select(len("typed:"), len("typed:"))
	tt.Frame()

	var want []string
	settle := func(what string) {
		t.Helper()
		f.loop.Until(t, what, func() bool {
			tt.Frame()
			f.loop.Advance(notes.SaveDelay)
			return d.Status() == notes.StatusClean && d.Text() == f.Read("Log.md").Content
		})
	}
	appendLine := func(content, line string) string {
		return content + line + "\n"
	}
	const rounds = 20
	for i := range rounds {
		tok := fmt.Sprintf(" a%d", i)
		tt.Type(tok)
		want = append(want, tok)
		tt.Frame()
		if i%2 == 0 {
			// The agent appends through notesd, from the content it read.
			n := f.Read("Log.md")
			tok := fmt.Sprintf("m%d", i)
			if _, err := f.Other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.Vault, Path: "Log.md", Base: n.Hash, Content: appendLine(n.Content, tok)}); err != nil {
				t.Fatal(err)
			}
			want = append(want, tok)
		}
		if i%5 == 4 {
			// The outside editor saves at a quiet moment, as a person in
			// vim does, by writing a temporary file and renaming it.
			settle("a quiet moment")
			n := f.Read("Log.md")
			tok := fmt.Sprintf("v%d", i)
			full := filepath.Join(f.Dir, "Log.md")
			if err := os.WriteFile(full+".swp", []byte(appendLine(n.Content, tok)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(full+".swp", full); err != nil {
				t.Fatal(err)
			}
			want = append(want, tok)
			f.loop.Until(t, "the outside edit to reach the editor", func() bool {
				tt.Frame()
				return strings.Contains(d.Text(), tok)
			})
		}
		f.loop.Advance(notes.SaveDelay / 2)
		f.loop.Drain()
	}
	settle("the end")
	content := f.Read("Log.md").Content
	for _, tok := range want {
		if !strings.Contains(content, strings.TrimSpace(tok)) {
			t.Errorf("token %q lost: %q", tok, content)
		}
	}
	typed := "typed:"
	for i := range rounds {
		typed += fmt.Sprintf(" a%d", i)
	}
	if first := strings.SplitN(content, "\n", 2)[0]; first != typed {
		t.Errorf("the typed line is %q, want %q: the caret moved", first, typed)
	}
	if ed.Binding().Buffer().String() != content {
		t.Errorf("the editor shows %q, the file holds %q", ed.Binding().Buffer().String(), content)
	}
}
