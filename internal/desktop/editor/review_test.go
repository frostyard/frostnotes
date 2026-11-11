package editor

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestToggleChecklistReplacesAWholeCharacter(t *testing.T) {
	for _, in := range []string{"- [✓] done|", "- [x] done|"} {
		got := run(t, ToggleChecklist, in)
		if !utf8.ValidString(got) || !strings.HasPrefix(got, "- [ ] done") {
			t.Errorf("%q made %q", in, got)
		}
	}
}

func TestLinkAtBalancedParensAndEscapedHash(t *testing.T) {
	text := "see [w](https://en.wikipedia.org/wiki/A_(b)) now"
	l, ok := LinkAt(text, 6)
	if !ok || l.URL != "https://en.wikipedia.org/wiki/A_(b)" {
		t.Fatalf("%+v", l)
	}
	l, ok = LinkAt("[d](draft%231.png#part)", 1)
	if !ok || l.Target != "draft#1.png" || l.Subpath != "#part" {
		t.Fatalf("%+v", l)
	}
}

func TestEmbedForANameAWikilinkCannotHold(t *testing.T) {
	for _, name := range []string{"draft#1.png", "a|b.png", "a]b.png"} {
		if got := EmbedFor(name, true, true); strings.HasPrefix(got, "![[") {
			t.Errorf("%q gives %q", name, got)
		}
	}
	if got := EmbedFor("draft#1.png", true, true); got != "![](draft%231.png)" {
		t.Errorf("%q", got)
	}
}

func TestReadingAFileRefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	if _, err := (File{Name: "pipe", Path: fifo}).Read(); err == nil {
		t.Fatal("read a FIFO")
	}
	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, make([]byte, MaxAttach+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (File{Name: "big", Path: big}).Read(); err == nil {
		t.Fatal("read a file over the limit")
	}
	small := filepath.Join(dir, "small")
	if err := os.WriteFile(small, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := (File{Name: "small", Path: small}).Read(); err != nil || string(data) != "x" {
		t.Fatalf("%q %v", data, err)
	}
}

func TestSmartListsLeaveCodeAlone(t *testing.T) {
	for _, in := range []string{"```\n- a|\n```", "~~~\n> q|\n~~~", "text\n\n    - a|"} {
		doc, sels := at(in)
		if _, ok := ContinueMarkup(doc, sels); ok {
			t.Errorf("%q was continued", in)
		}
	}
	for _, in := range []string{"```\nx\n```\n- a|", "- parent\n\n    - nested|"} {
		doc, sels := at(in)
		if _, ok := ContinueMarkup(doc, sels); !ok {
			t.Errorf("%q was not continued", in)
		}
	}
}

func TestYAMLAliasesAreBounded(t *testing.T) {
	var laughs strings.Builder
	laughs.WriteString("---\na0: &a0 [x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 20; i++ {
		laughs.WriteString("a" + string(rune('a'+i)) + ": &a" + string(rune('a'+i)) + " [")
		prev := "a0"
		if i > 1 {
			prev = "a" + string(rune('a'+i-1))
		}
		laughs.WriteString(strings.TrimSuffix(strings.Repeat("*"+prev+", ", 8), ", ") + "]\n")
	}
	laughs.WriteString("---\nbody\n")
	for name, in := range map[string]string{"a cycle": "---\na: &a [*a]\n---\nbody\n", "laughs": laughs.String()} {
		done := make(chan Frontmatter, 1)
		go func() { done <- ReadFrontmatter(in) }()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
}

func TestAKeyStartingWithADashIsItsOwn(t *testing.T) {
	next, err := SetProperties("---\na: 1\n-b: 2\n---\nbody\n", map[string]any{"a": int64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "-b: 2\n") || !strings.Contains(next, "a: 3\n") {
		t.Fatalf("%q", next)
	}
}

func TestFrontmatterEndsAtItsFirstClosingLine(t *testing.T) {
	if got := Body("---\n---\na: 1\n---\nbody"); got != "a: 1\n---\nbody" {
		t.Fatalf("empty frontmatter left %q", got)
	}
	if f := ReadFrontmatter("---\n---\na: 1\n---\nbody"); len(f.Properties) != 0 {
		t.Fatalf("the body reached the panel: %+v", f)
	}
	if got := Body("---\na: 1\n...\nbody"); got != "body" {
		t.Fatalf("the dots left %q", got)
	}
	if f := ReadFrontmatter("---\na: 1\n...\nbody"); len(f.Properties) != 1 {
		t.Fatalf("%+v", f)
	}
}

// lateSources answers a subpath only when the test says.
type lateSources struct {
	fakeSources
	answer func()
}

func (s *lateSources) Subpaths(_ string, _ bool, done func([]Target)) {
	s.answer = func() { done([]Target{{Insert: "Goals", Label: "Goals"}}) }
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
