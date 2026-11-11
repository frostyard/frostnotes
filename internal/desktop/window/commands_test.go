package window

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/commands"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// openNote leaves a note with a link under the caret and two tabs, so that
// every command applies.
func openNote(t *testing.T) *fixture {
	t.Helper()
	f := start(t)
	f.Create("a.md", "see [[b]]\n")
	f.Create("b.md", "b\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("a.md") })
	f.a.open("b.md", workspace.NewTab)
	f.a.open("a.md", workspace.NewTab)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	f.a.activeEditor().State.Select(6, 6)
	f.tt.Frame()
	return f
}

// counted replaces every action with a counter.
func counted(a *App) map[string]int {
	runs := map[string]int{}
	for id := range a.actions {
		a.actions[id] = func() { runs[id]++ }
	}
	return runs
}

// a state in which even the commands that need a history apply.
func makeApply(f *fixture) {
	f.a.Store.Update(func(w *workspace.Workspace) {
		w.Closed = append(w.Closed, workspace.ClosedTab{Tab: workspace.Tab{ID: "closed", View: workspace.Note("b.md", workspace.ModeEdit)}})
		t := w.ActiveTab()
		t.Back = append(t.Back, workspace.Note("b.md", workspace.ModeEdit))
		t.Forward = append(t.Forward, workspace.Note("b.md", workspace.ModeEdit))
	})
	f.tt.Frame()
}

func TestEveryCommandRunsFromThePalette(t *testing.T) {
	f := openNote(t)
	makeApply(f)
	listed := f.a.table
	runs := counted(f.a)
	for _, s := range listed {
		if s.Unlisted {
			continue
		}
		if !f.a.applies(s.ID) {
			t.Errorf("%s does not apply", s.ID)
			continue
		}
		f.a.Shell.Palette.ShowSource(frostui.PaletteCommands)
		f.tt.Frame()
		f.tt.Type(s.Title)
		f.tt.Frame()
		found := -1
		for i, r := range frostui.Filter(f.a.cmds.Commands(), s.Title) {
			if r.ID == s.ID {
				found = i
			}
		}
		if found < 0 {
			t.Errorf("%s is not listed", s.ID)
			f.a.Shell.Palette.Open = false
			continue
		}
		for range found {
			f.tt.Key(0, ui.KeyDown)
		}
		f.tt.Frame()
		f.tt.Key(0, ui.KeyEnter)
		f.tt.Frame()
		if runs[s.ID] != 1 {
			t.Errorf("%s ran %d times from the palette", s.ID, runs[s.ID])
		}
	}
}

func TestAPaletteCommandShowsInTheFrameItRuns(t *testing.T) {
	f := openNote(t)
	// A view takes every toast waiting for it, so one still waiting after
	// a view came too late for that frame.
	late := 0
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.a.View(c)
		late += len(f.a.toasts)
	}, 1200, 800)
	ran := false
	spec := f.a.table[0]
	f.a.actions[spec.ID] = func() {
		ran = true
		f.a.notify("It ran.")
	}
	f.a.Shell.Palette.ShowSource(frostui.PaletteCommands)
	f.tt.Frame()
	f.tt.Type(spec.Title)
	f.tt.Frame()
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Frame()
	f.tt.Frame()
	if !ran {
		t.Fatal("the command did not run")
	}
	if late != 0 {
		t.Fatalf("%d toasts waited for a later frame", late)
	}
}

func TestEveryKeyRunsItsCommand(t *testing.T) {
	f := openNote(t)
	makeApply(f)
	runs := counted(f.a)
	for _, s := range f.a.table {
		for _, k := range s.Keys {
			if k.Scope == commands.InEditor || isPaletteKey(k) {
				continue
			}
			name := fmt.Sprintf("%s by %v", s.ID, keyCaps(k))
			before := runs[s.ID]
			f.tt.Key(k.Mods, k.Key)
			f.tt.Frame()
			if f.a.Shell.Palette.Open {
				f.a.Shell.Palette.Open = false
				f.tt.Frame()
			}
			if runs[s.ID] != before+1 {
				t.Errorf("%s ran %d times", name, runs[s.ID]-before)
			}
		}
	}
}

// isPaletteKey is a key of frostui's own palette command.
func isPaletteKey(k commands.Key) bool {
	return slices.ContainsFunc(frostui.PaletteBindings(), func(b frostui.KeyBinding) bool { return b.Mods == k.Mods && b.Key == k.Key })
}

func TestThePalettesOwnKeysOpenIt(t *testing.T) {
	f := openNote(t)
	for _, k := range frostui.PaletteBindings() {
		f.tt.Key(k.Mods, k.Key)
		f.tt.Frame()
		if !f.a.Shell.Palette.Open || f.a.Shell.Palette.ShownSource() != "" {
			t.Errorf("%s: open %v on %q", k, f.a.Shell.Palette.Open, f.a.Shell.Palette.ShownSource())
		}
		f.a.Shell.Palette.Open = false
		f.tt.Frame()
	}
}

func TestCtrlKIsTheEditorsInAnEditor(t *testing.T) {
	f := openNote(t)
	if err := f.tt.Click("Note"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if !f.a.inEditor() {
		t.Fatal("the editor has no focus")
	}
	f.a.activeEditor().State.Select(0, 3)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	if f.a.Shell.Palette.Open {
		t.Fatal("Ctrl K opened the palette in an editor")
	}
	if got := f.a.activeEditor().Binding().Buffer().String(); got != "[see]() [[b]]\n" {
		t.Fatalf("Ctrl K made %q", got)
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	f.tt.Frame()
	if !f.a.Shell.Palette.Open {
		t.Fatal("Ctrl Shift P did not open the palette in an editor")
	}
}

func TestFollowingATagSearchesForIt(t *testing.T) {
	f := openNote(t)
	e := f.a.activeEditor()
	e.Binding().Document().Edit(mustInsert(t, e.Binding().Document().Text(), "\n#garden\n"))
	f.tt.Frame()
	at := len("see [[b]]\n\n#gar")
	e.State.Select(at, at)
	f.tt.Frame()
	if err := f.tt.Click("Note"); err != nil {
		t.Fatal(err)
	}
	e.State.Select(at, at)
	f.tt.Frame()
	f.tt.Key(ui.Alt, ui.KeyEnter)
	f.tt.Frame()
	if f.a.search.Query != "tag:#garden" {
		t.Fatalf("the search is %q", f.a.search.Query)
	}
}

func mustInsert(t *testing.T, text, add string) notes.ChangeSet {
	t.Helper()
	cs, err := notes.ChangesOf(len(text), notes.Change{From: len(text), To: len(text), Insert: add})
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestInsertingATemplateReachesTheFile(t *testing.T) {
	f := start(t)
	f.Create("Templates/Day.md", "---\nmood: good\n---\nBody of {{title}}\n")
	f.Create("a.md", "start\n")
	f.openVault()
	f.a.settingChanged("vault:templateFolder", "Templates")
	f.until("the setting", func() bool { return f.a.settings.TemplateFolder == "Templates" })
	f.a.open("a.md", workspace.Here)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	e := f.a.activeEditor()
	e.State.Select(len("start\n"), len("start\n"))
	f.tt.Frame()
	f.a.insertTemplate("Templates/Day.md")
	want := "---\nmood: good\n---\nstart\nBody of a\n"
	d := e.Binding().Document()
	f.until("the template", func() bool {
		e.Binding().Sync()
		return d.Text() == want
	})
	f.loop.Advance(notes.SaveDelay)
	f.until("the save", func() bool { return d.Status() == notes.StatusClean && !d.Unsaved() })
	if got := f.Read("a.md").Content; got != want {
		t.Fatalf("the file holds %q", got)
	}
}

func TestAddingAFilePropertyMakesFrontmatter(t *testing.T) {
	f := openNote(t)
	f.a.addFileProperty()
	f.tt.Frame()
	d := f.a.activeEditor().Binding().Document()
	if got := d.Text(); got != "---\n---\nsee [[b]]\n" {
		t.Fatalf("the note is %q", got)
	}
	if got := f.a.activeEditor().Binding().Buffer().String(); got != d.Text() {
		t.Fatalf("the editor shows %q", got)
	}
}
