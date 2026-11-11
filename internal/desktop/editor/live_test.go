package editor

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/reading"
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

func TestAClickFollowsALinkATagAndAFootnote(t *testing.T) {
	note := "see [[Plan|the plan]] and #garden and a ref[^n]\n\n[^n]: the note\n\nlast\n"
	f := openEditor(t, "a.md", note)
	var followed Link
	var where Where
	var tag string
	f.ed.Follow = func(l Link, w Where) { followed, where = l, w }
	f.ed.FollowTag = func(s string) { tag = s }
	f.tt.Frame()
	if err := f.tt.Click("the plan"); err != nil {
		t.Fatal(err)
	}
	if followed.Target != "Plan" || where != WhereHere {
		t.Fatalf("followed %+v to %v", followed, where)
	}
	if err := f.tt.ClickWith(ui.Cmd, "the plan"); err != nil {
		t.Fatal(err)
	}
	if where != WhereTab {
		t.Fatalf("Ctrl + click went to %v", where)
	}
	if err := f.tt.Click("#garden"); err != nil {
		t.Fatal(err)
	}
	if tag != "garden" {
		t.Fatalf("searched for %q", tag)
	}
	if err := f.tt.Click("n"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if got, want := f.ed.State.Cursor(), strings.Index(note, "[^n]:"); got != want {
		t.Fatalf("the caret is at %d, want %d", got, want)
	}
}

func TestADoneTasksTextIsDim(t *testing.T) {
	f := openEditor(t, "a.md", "- [x] gone\n\nlast\n")
	got := f.shown()
	if want := []string{"- [x]=>box", " gone=> gone"}; !slices.Equal(got, want) {
		t.Fatalf("%q, want %q", got, want)
	}
	if r := f.ed.State.Replacements()[1]; r.Color == (ui.Color{}) {
		t.Fatal("the text kept its color")
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

// vaultSources resolves the fixture's files by name, as the catalog does.
type vaultSources struct {
	fakeSources
	files []string
}

func (s vaultSources) Resolve(target string) string {
	for _, f := range s.files {
		if strings.TrimSuffix(f, ".md") == target || f == target {
			return f
		}
	}
	return ""
}

// openBlocks opens a note whose editor draws rendered blocks, the caret
// on its last line.
func openBlocks(t *testing.T, content string, files ...string) *editorFixture {
	t.Helper()
	f := openEditor(t, "a.md", content)
	f.ed.Sources = vaultSources{files: files}
	f.ed.Rendered = reading.NewBlocks(f.loop, func() reading.Renderer {
		if c := f.AppNotes(); c != nil {
			return c
		}
		return nil
	}, f.doc, f.Blobs)
	f.ed.RefreshLive()
	return f
}

// settle draws until a block has what it asked notesd for.
func (f *editorFixture) settleOn(text string) {
	f.t.Helper()
	f.loop.Until(f.t, text, func() bool {
		f.tt.Frame()
		f.loop.Post(func() {})
		return f.tt.HasText(text)
	})
	for range 3 {
		f.tt.Frame()
	}
}

func (f *editorFixture) blockLabels() []string {
	f.tt.Frame()
	var out []string
	for _, b := range f.ed.State.Blocks() {
		out = append(out, b.Label)
	}
	return out
}

const blocksNote = "top\n\n> [!note] Heads up\n> callout body\n\n| Name | Age |\n|------|-----|\n| Ada | 36 |\n\n```go\nx := 1\n```\n\nlast\n"

func TestCalloutsAndTablesDrawInPlace(t *testing.T) {
	f := openBlocks(t, blocksNote)
	want := []string{"Callout", "Table", "Code block"}
	if got := f.blockLabels(); !slices.Equal(got, want) {
		t.Fatalf("blocks %q, want %q", got, want)
	}
	f.settleOn("callout body")
	f.settleOn("Ada")
	// A caret on a block's lines shows its source, in place of the block.
	f.caret(strings.Index(blocksNote, "callout body"))
	if got, want := f.blockLabels(), []string{"Table", "Code block"}; !slices.Equal(got, want) {
		t.Fatalf("with a caret in the callout: %q, want %q", got, want)
	}
}

func TestAClickOnABlockShowsItsSource(t *testing.T) {
	f := openBlocks(t, blocksNote)
	f.settleOn("Ada")
	info, ok := f.tt.Access("Table")
	if !ok {
		t.Fatalf("no table block for assistive technology: %+v", f.tt.AccessAll())
	}
	f.tt.ClickAt(info.Bounds.X+info.Bounds.W-4, info.Bounds.Y+info.Bounds.H/2)
	for range 3 {
		f.tt.Frame()
	}
	if got, want := f.blockLabels(), []string{"Callout", "Code block"}; !slices.Equal(got, want) {
		t.Fatalf("after the click: %q, want %q", got, want)
	}
	if !f.ed.State.Focused() {
		t.Fatal("the click left the editor without the focus")
	}
}

func TestACodeHeaderCopiesItsCode(t *testing.T) {
	f := openBlocks(t, blocksNote)
	f.tt.Frame()
	f.tt.Frame()
	if !f.tt.HasText("go") {
		t.Fatalf("no language: %q", f.tt.Texts())
	}
	if err := f.tt.Click("Copy code"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if got := f.tt.Clipboard(); got != "x := 1" {
		t.Fatalf("copied %q", got)
	}
}

func TestEmbedsDrawFromTheVault(t *testing.T) {
	note := "top\n\n![[Plan]]\n\n![[Missing]]\n\nlast\n"
	f := openBlocks(t, note, "Plan.md")
	f.Create("Plan.md", "the plan body\n")
	if got, want := f.blockLabels(), []string{"Embedded note"}; !slices.Equal(got, want) {
		t.Fatalf("blocks %q, want %q", got, want)
	}
	f.settleOn("the plan body")
	if got := f.shown(); !slices.Equal(got, []string{"![[Plan]]\n=>"}) {
		t.Fatalf("replaced %q", got)
	}
	// An embedded note follows its edits, a moment after the vault changes.
	f.Write("Plan.md", f.Read("Plan.md").Hash, "a new body\n")
	f.ed.Rendered.Changed()
	f.loop.Advance(reading.RefreshDelay)
	f.settleOn("a new body")
}

func TestAnImageDrawsFromTheBlobStore(t *testing.T) {
	f := openBlocks(t, "text ![[dot.png]] more\n\nlast\n", "dot.png")
	var dot bytes.Buffer
	if err := png.Encode(&dot, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Other.Note().Upload(t.Context(), &api.NoteUploadParams{VaultID: f.Vault, Path: "dot.png", Data: base64.StdEncoding.EncodeToString(dot.Bytes())}); err != nil {
		t.Fatal(err)
	}
	if got, want := f.blockLabels(), []string{"Image"}; !slices.Equal(got, want) {
		t.Fatalf("blocks %q, want %q", got, want)
	}
	if got := f.shown(); !slices.Equal(got, []string{"![[dot.png]]=>"}) {
		t.Fatalf("replaced %q", got)
	}
	f.loop.Until(t, "the rendering", func() bool {
		f.tt.Frame()
		f.loop.Post(func() {})
		return !f.tt.HasText("![[dot.png]]")
	})
}

func TestBlocksLeaveTheFileAlone(t *testing.T) {
	f := openBlocks(t, blocksNote)
	f.settleOn("Ada")
	f.caret(strings.Index(blocksNote, "last"))
	f.tt.Type("the ")
	f.save()
	if got, want := f.Read("a.md").Content, strings.Replace(blocksNote, "last", "the last", 1); got != want {
		t.Fatalf("file holds %q", got)
	}
}
