package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// shown lists what the editor replaces, each as its source and what stands
// in for it: nothing, its text, or "box" for an element.
func (f *editorFixture) shown() []string {
	f.tt.Frame()
	text := f.text()
	var out []string
	for _, r := range f.ed.State.Replacements() {
		stand := r.Text
		if r.Content != nil {
			stand = "box"
		}
		out = append(out, text[r.Start:r.End]+"=>"+stand)
	}
	return out
}

const liveNote = "# Head\n\n**bold** text\n\n- item\n- [ ] task\n> quote\n\n---\n\nlast\n"

func TestLivePreviewHidesTheMarksOffTheCaretsLines(t *testing.T) {
	f := openEditor(t, "a.md", liveNote)
	rule := "---=>" + strings.Repeat("─", ruleWidth)
	want := []string{"# =>", "**=>", "**=>", "-=>•", "- [ ]=>box", "> =>▏ ", rule}
	if got := f.shown(); !slices.Equal(got, want) {
		t.Fatalf("away from the marks: %q, want %q", got, want)
	}
	f.caret(strings.Index(liveNote, "bold"))
	want = []string{"# =>", "-=>•", "- [ ]=>box", "> =>▏ ", rule}
	if got := f.shown(); !slices.Equal(got, want) {
		t.Fatalf("on the bold line: %q, want %q", got, want)
	}
	// A selection across lines shows the source of each.
	f.ed.State.Select(0, strings.Index(liveNote, "- item"))
	want = []string{"- [ ]=>box", "> =>▏ ", rule}
	if got := f.shown(); !slices.Equal(got, want) {
		t.Fatalf("across lines: %q, want %q", got, want)
	}
}

func TestSourceModeShowsTheText(t *testing.T) {
	f := openEditor(t, "a.md", liveNote)
	if len(f.shown()) == 0 {
		t.Fatal("live preview replaced nothing")
	}
	f.ed.Prefs.Live = false
	if got := f.shown(); len(got) != 0 {
		t.Fatalf("source mode replaced %q", got)
	}
}

func TestLivePreviewLeavesTheFileAlone(t *testing.T) {
	f := openEditor(t, "a.md", liveNote)
	f.caret(strings.Index(liveNote, "last"))
	f.tt.Type("the ")
	f.save()
	want := strings.Replace(liveNote, "last", "the last", 1)
	if got := f.Read("a.md").Content; got != want {
		t.Fatalf("file holds %q", got)
	}
}

func TestATaskBoxTogglesItsTask(t *testing.T) {
	f := openEditor(t, "a.md", liveNote)
	f.tt.Frame()
	if _, ok := f.tt.AccessByRole(ui.RoleButton, "Not done"); !ok {
		t.Fatalf("no box for assistive technology: %+v", f.tt.AccessAll())
	}
	before := f.ed.State.Cursor()
	if err := f.tt.Click("Not done"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	want := strings.Replace(liveNote, "- [ ] task", "- [x] task", 1)
	if f.text() != want {
		t.Fatalf("the click made %q", f.text())
	}
	if f.ed.State.Cursor() != before {
		t.Errorf("the caret moved from %d to %d", before, f.ed.State.Cursor())
	}
	f.tt.Frame()
	if _, ok := f.tt.AccessByRole(ui.RoleButton, "Done"); !ok {
		t.Fatal("the box does not show the task done")
	}
	f.save()
	if got := f.Read("a.md").Content; got != want {
		t.Fatalf("file holds %q", got)
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	if f.text() != liveNote {
		t.Fatalf("undo made %q", f.text())
	}
}

func TestTheFrontmatterHidesBehindTheProperties(t *testing.T) {
	content := "---\ntags: [a]\n---\nbody\n"
	f := openEditor(t, "a.md", content)
	if got := f.shown(); len(got) != 0 {
		t.Fatalf("without the panel: %q", got)
	}
	f.ed.HideFront = true
	if got, want := f.shown(), []string{"---\ntags: [a]\n---\n=>"}; !slices.Equal(got, want) {
		t.Fatalf("behind the panel: %q, want %q", got, want)
	}
}
