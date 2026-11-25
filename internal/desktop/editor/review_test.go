package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/markup"
)

// lateSources answers a subpath only when the test says.
type lateSources struct {
	fakeSources
	answer func()
}

func (s *lateSources) Subpaths(_ string, _ bool, done func([]markup.Target)) {
	s.answer = func() { done([]markup.Target{{Insert: "Goals", Label: "Goals"}}) }
}

func TestALateHeadingAnswerAfterTheCaretMoved(t *testing.T) {
	f := openEditor(t, "a.md", "start\n")
	src := &lateSources{}
	f.ed.Sources = src
	f.caret(6)
	for _, k := range []string{"[", "[", "P", "#"} {
		f.tt.Type(k)
	}
	f.tt.Frame()
	if src.answer == nil {
		t.Fatal("no request")
	}
	f.caret(0)
	src.answer()
	f.tt.Frame()
	if f.tt.HasText("Goals") {
		t.Fatalf("a completion opened at the old place: %q", f.tt.Texts())
	}
}

func TestCommittingAFieldThatHasTheFocus(t *testing.T) {
	f, v, tt := openView(t, "---\nstatus: draft\n---\nbody\n")
	tt.Frame()
	if err := tt.Click("status"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("final")
	tt.Frame()
	if strings.Contains(f.doc.Text(), "final") {
		t.Fatal("committed before the close")
	}
	v.Properties.Commit()
	if !strings.Contains(f.doc.Text(), "status: final") {
		t.Fatalf("%q", f.doc.Text())
	}
}

// askedSources records what the link list was asked for.
type askedSources struct {
	fakeSources
	asked []string
}

func (s *askedSources) LinkTargets(query string, _ int) []markup.Target {
	s.asked = append(s.asked, query)
	return []markup.Target{{Insert: "garden", Label: "garden"}} // keeps the list open
}

func TestTheLinkListIsAskedAgainAsTheQueryGrows(t *testing.T) {
	f := openEditor(t, "a.md", "")
	src := &askedSources{}
	f.ed.Sources = src
	for _, k := range []string{"[", "[", "g", "a", "r"} {
		f.tt.Type(k)
		f.tt.Frame()
	}
	if !slices.Contains(src.asked, "gar") {
		t.Fatalf("asked %q", src.asked)
	}
}
