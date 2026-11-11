package notes

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
)

// fakeBuffer is an editor's buffer: its text follows the document's.
type fakeBuffer struct {
	doc    *Document
	text   string
	remote int
}

func (b *fakeBuffer) Sync() {}

func (b *fakeBuffer) Apply(cs ChangeSet) {
	b.text, _ = cs.Apply(b.text)
	b.remote++
}

func (b *fakeBuffer) Edit(cs ChangeSet) { b.text, _ = cs.Apply(b.text) }

// typeAt makes a user's edit in the buffer and reports it.
func (b *fakeBuffer) typeAt(t *testing.T, from, to int, insert string) {
	t.Helper()
	cs, err := ChangesOf(len(b.text), Change{From: from, To: to, Insert: insert})
	if err != nil {
		t.Fatal(err)
	}
	b.text, _ = cs.Apply(b.text)
	b.doc.Local(cs)
}

type fixture struct {
	*desktoptest.Notesd
	t    *testing.T
	loop *desktoptest.Loop
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{Notesd: desktoptest.StartNotesd(t), t: t, loop: desktoptest.NewLoop()}
}

func (f *fixture) client() NoteAPI {
	if c := f.AppNotes(); c != nil {
		return c
	}
	return nil
}

// open loads a document with a buffer attached.
func (f *fixture) open(path string) (*Document, *fakeBuffer) {
	f.t.Helper()
	d := NewDocument(f.loop, f.client, f.Vault, path)
	d.Load()
	f.loop.Until(f.t, "the note to load", func() bool { return d.Loaded() || d.Status() == StatusMissing })
	b := &fakeBuffer{doc: d, text: d.Text()}
	d.Attach(b)
	return d, b
}

func (f *fixture) settle(d *Document) {
	f.t.Helper()
	f.loop.Until(f.t, "the save", func() bool { return d.Status() == StatusClean })
}

func TestTypingSavesAfterTheDelay(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "hello\n")
	d, b := f.open("a.md")
	b.typeAt(t, 5, 5, " world")
	if d.Status() != StatusDirty {
		t.Fatalf("status %v after typing", d.Status())
	}
	f.loop.Advance(SaveDelay - time.Millisecond)
	if d.Status() != StatusDirty {
		t.Fatal("saved before the delay")
	}
	f.loop.Advance(time.Millisecond)
	f.settle(d)
	if got := f.Read("a.md").Content; got != "hello world\n" {
		t.Fatalf("file holds %q", got)
	}
	if d.Base() != f.Read("a.md").Hash {
		t.Fatal("the base is not the saved content's hash")
	}
}

func TestTypingWithoutPauseSavesAtTheMaximum(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "x")
	d, b := f.open("a.md")
	for i := range 20 {
		b.typeAt(t, len(b.text), len(b.text), "y")
		f.loop.Advance(SaveMax / 20)
		if i < 19 && d.Status() == StatusSaving {
			t.Fatalf("saved after %d keys", i+1)
		}
	}
	if d.Status() != StatusSaving {
		t.Fatalf("status %v after %v of typing", d.Status(), SaveMax)
	}
	f.settle(d)
}

func TestAMergedAnswerKeepsWhatWasTypedMeanwhile(t *testing.T) {
	f := newFixture(t)
	base := f.Create("a.md", "one\ntwo\nthree\n")
	d, b := f.open("a.md")

	// Another writer changes the last line.
	if _, err := f.Other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.Vault, Path: "a.md", Base: base, Content: "one\ntwo\nTHREE\n"}); err != nil {
		t.Fatal(err)
	}
	// The app changes the first, saves, and types more before the answer.
	b.typeAt(t, 0, 3, "ONE")
	f.loop.Advance(SaveDelay)
	if d.Status() != StatusSaving {
		t.Fatal("no save in flight")
	}
	b.typeAt(t, 7, 7, " and a half") // "ONE\ntwo and a half\n…"
	f.loop.Until(t, "the merged answer", func() bool { return d.Status() != StatusSaving })

	want := "ONE\ntwo and a half\nTHREE\n"
	if d.Text() != want || b.text != want {
		t.Fatalf("document %q, buffer %q, want %q", d.Text(), b.text, want)
	}
	if b.remote == 0 {
		t.Fatal("the merge did not reach the buffer as a remote change")
	}
	f.settle(d)
	if got := f.Read("a.md").Content; got != want {
		t.Fatalf("file holds %q", got)
	}
}

func TestAConflictLeavesANotice(t *testing.T) {
	f := newFixture(t)
	base := f.Create("a.md", "line\n")
	d, b := f.open("a.md")
	if _, err := f.Other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.Vault, Path: "a.md", Base: base, Content: "theirs\n"}); err != nil {
		t.Fatal(err)
	}
	b.typeAt(t, 0, 4, "mine")
	f.loop.Advance(SaveDelay)
	f.settle(d)
	n := d.Notice()
	if n == nil || n.Kind != NoticeConflict || n.Conflicts == 0 || n.Version == 0 {
		t.Fatalf("notice %+v", n)
	}
	if d.Text() != f.Read("a.md").Content {
		t.Fatal("the document is not the file after the merge")
	}
	d.Dismiss()
	if d.Notice() != nil {
		t.Fatal("Dismiss left the notice")
	}
}

func TestAnotherWritersChangeReachesACleanDocument(t *testing.T) {
	f := newFixture(t)
	base := f.Create("a.md", "a\n")
	d, b := f.open("a.md")
	res, err := f.Other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.Vault, Path: "a.md", Base: base, Content: "a\nb\n"})
	if err != nil {
		t.Fatal(err)
	}
	d.Changed(res.Hash)
	f.loop.Until(t, "the reload", func() bool { return d.Base() == res.Hash })
	if d.Text() != "a\nb\n" || b.text != "a\nb\n" || b.remote != 1 {
		t.Fatalf("document %q, buffer %q, %d remote changes", d.Text(), b.text, b.remote)
	}
	if d.Status() != StatusClean {
		t.Fatal("a reload made the document dirty")
	}
	d.Changed(res.Hash) // its own base: nothing to do
	if d.Status() != StatusClean {
		t.Fatal("its own hash reloaded")
	}
}

func TestARefusedBaseKeepsTheTypedText(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "file\n")
	d, b := f.open("a.md")
	d.base = "0000000000000000000000000000000000000000000000000000000000000000"
	b.typeAt(t, 0, 4, "typed")
	f.loop.Advance(SaveDelay)
	f.loop.Until(t, "the refusal", func() bool { return d.Notice() != nil })
	n := d.Notice()
	if n.Kind != NoticeRefused || n.Text != "typed\n" {
		t.Fatalf("notice %+v", n)
	}
	if d.Text() != "file\n" || b.text != "file\n" || d.Status() != StatusClean {
		t.Fatalf("document %q, buffer %q, status %v", d.Text(), b.text, d.Status())
	}
}

func TestADeletedNote(t *testing.T) {
	f := newFixture(t)
	hash := f.Create("a.md", "gone soon\n")
	d, b := f.open("a.md")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	if n := d.Notice(); n == nil || n.Kind != NoticeDeleted {
		t.Fatalf("notice %+v", n)
	}
	d.Restore()
	f.loop.Until(t, "the restore", func() bool { return d.Notice() == nil })
	if f.Read("a.md").Content != "gone soon\n" || b.text != "gone soon\n" {
		t.Fatal("the note did not come back")
	}
}

func TestUnsavedTextCreatesADeletedNoteAgain(t *testing.T) {
	f := newFixture(t)
	hash := f.Create("a.md", "text\n")
	d, b := f.open("a.md")
	b.typeAt(t, 4, 4, " more")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	f.settle(d)
	if got := f.Read("a.md").Content; got != "text more\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestCRLFNotesKeepTheirLineEndings(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "one\r\ntwo\r\n")
	d, b := f.open("a.md")
	if d.Text() != "one\ntwo\n" || !d.CRLF() || d.ReadOnly() != "" {
		t.Fatalf("text %q crlf %v read-only %q", d.Text(), d.CRLF(), d.ReadOnly())
	}
	b.typeAt(t, 3, 3, "!")
	f.loop.Advance(SaveDelay)
	f.settle(d)
	if got := f.Read("a.md").Content; got != "one!\r\ntwo\r\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestMixedLineEndingsAreReadOnly(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "one\r\ntwo\n")
	d, b := f.open("a.md")
	if d.ReadOnly() == "" {
		t.Fatal("a note with mixed line endings is editable")
	}
	b.typeAt(t, 0, 0, "x")
	f.loop.Advance(SaveMax)
	if d.Status() != StatusClean {
		t.Fatal("a read-only note saved")
	}
	if got := f.Read("a.md").Content; got != "one\r\ntwo\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestASaveWithoutNotesdRetries(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "a")
	d, b := f.open("a.md")
	f.Down = true
	b.typeAt(t, 1, 1, "b")
	f.loop.Advance(SaveDelay)
	if n := d.Notice(); n == nil || n.Kind != NoticeFailed {
		t.Fatalf("notice %+v", n)
	}
	f.Down = false
	f.loop.Advance(RetryFirst)
	f.settle(d)
	if d.Notice() != nil {
		t.Fatal("the failure notice stayed after the save")
	}
	if f.Read("a.md").Content != "ab" {
		t.Fatal("the retry did not save")
	}
}

func TestFlushWaitsForTheSave(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "a")
	d, b := f.open("a.md")
	b.typeAt(t, 1, 1, "b")
	flushed := false
	d.Flush(func() { flushed = true })
	f.loop.Until(t, "the flush", func() bool { return flushed })
	if d.Unsaved() || f.Read("a.md").Content != "ab" {
		t.Fatal("flushed before the save")
	}
}

func TestAMissingNote(t *testing.T) {
	f := newFixture(t)
	d, _ := f.open("nope.md")
	if d.Status() != StatusMissing {
		t.Fatalf("status %v", d.Status())
	}
}

func TestEditWithoutABufferSaves(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "- [ ] task\n")
	d, _ := f.open("a.md")
	d.Attach(nil)
	cs, _ := ChangesOf(len(d.Text()), Change{From: 3, To: 4, Insert: "x"})
	d.Edit(cs)
	f.loop.Advance(SaveDelay)
	f.settle(d)
	if got := f.Read("a.md").Content; got != "- [x] task\n" {
		t.Fatalf("file holds %q", got)
	}
}

// An outside writer (an editor saving in place) changes the file on disk;
// the document follows it through note.changed.
func TestAnOutsideEditReachesTheDocument(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "start\n")
	d, b := f.open("a.md")
	if err := os.WriteFile(filepath.Join(f.Dir, "a.md.tmp"), []byte("start\nvim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(f.Dir, "a.md.tmp"), filepath.Join(f.Dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		n := f.Read("a.md")
		if n.Content == "start\nvim\n" {
			d.Changed(n.Hash)
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("notesd did not see the outside edit")
		case <-time.After(20 * time.Millisecond):
		}
	}
	f.loop.Until(t, "the reload", func() bool { return b.text == "start\nvim\n" })
}

// gate is a NoteAPI whose reads and creates wait, while held, for the test
// to let them through, so a test can type during one or answer them out of
// order. The rest go straight to notesd.
type gate struct {
	NoteAPI
	loop    *desktoptest.Loop // woken when a request arrives
	held    atomic.Bool
	reads   chan *heldRead
	creates chan chan struct{}
	writes  atomic.Int32
}

// heldRead is a read waiting for its answer.
type heldRead struct {
	path   string
	answer chan func() (*api.Note, error)
}

func (g *gate) Read(ctx context.Context, p *api.NoteReadParams) (*api.Note, error) {
	if !g.held.Load() {
		return g.NoteAPI.Read(ctx, p)
	}
	r := &heldRead{path: p.Path, answer: make(chan func() (*api.Note, error))}
	g.reads <- r
	g.loop.Post(func() {})
	return (<-r.answer)()
}

func (g *gate) Create(ctx context.Context, p *api.NoteCreateParams) (*api.WriteResult, error) {
	if g.held.Load() {
		release := make(chan struct{})
		g.creates <- release
		g.loop.Post(func() {})
		<-release
	}
	return g.NoteAPI.Create(ctx, p)
}

func (g *gate) Write(ctx context.Context, p *api.NoteWriteParams) (*api.WriteResult, error) {
	g.writes.Add(1)
	return g.NoteAPI.Write(ctx, p)
}

// countedEnv counts the answers the UI thread has run.
type countedEnv struct {
	*desktoptest.Loop
	ran atomic.Int32
}

func (e *countedEnv) Post(f func()) {
	e.Loop.Post(func() {
		f()
		e.ran.Add(1)
	})
}

// openGated loads a document behind a gate, which holds from then on.
func (f *fixture) openGated(path string) (*Document, *fakeBuffer, *gate, *countedEnv) {
	f.t.Helper()
	g := &gate{NoteAPI: f.client(), loop: f.loop, reads: make(chan *heldRead, 4), creates: make(chan chan struct{}, 4)}
	env := &countedEnv{Loop: f.loop}
	d := NewDocument(env, func() NoteAPI { return g }, f.Vault, path)
	d.Load()
	f.loop.Until(f.t, "the note to load", d.Loaded)
	b := &fakeBuffer{doc: d, text: d.Text()}
	d.Attach(b)
	g.held.Store(true)
	return d, b, g, env
}

func (f *fixture) nextRead(g *gate) *heldRead {
	f.t.Helper()
	var r *heldRead
	f.loop.Until(f.t, "a read", func() bool {
		select {
		case r = <-g.reads:
			return true
		default:
			return false
		}
	})
	return r
}

func TestOnlyTheNewestReadCounts(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "start\n")
	d, b, g, env := f.openGated("a.md")
	before := env.ran.Load()
	d.Reload()
	older := f.nextRead(g)
	d.Reload()
	newer := f.nextRead(g)
	newer.answer <- func() (*api.Note, error) { return &api.Note{Content: "newer\n", Hash: "new"}, nil }
	f.loop.Until(t, "the newer answer", func() bool { return env.ran.Load() == before+1 })
	older.answer <- func() (*api.Note, error) { return &api.Note{Content: "older\n", Hash: "old"}, nil }
	f.loop.Until(t, "the older answer", func() bool { return env.ran.Load() == before+2 })
	if b.text != "newer\n" || d.Base() != "new" {
		t.Fatalf("text %q, base %q", b.text, d.Base())
	}
}

func TestARenameDuringAReadReadsTheNewPath(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "start\n")
	d, _, g, _ := f.openGated("a.md")
	d.Reload()
	first := f.nextRead(g)
	if first.path != "a.md" {
		t.Fatalf("read %q", first.path)
	}
	d.Moved("b.md")
	first.answer <- func() (*api.Note, error) { return nil, &api.Error{Code: api.CodeNotFound} }
	second := f.nextRead(g)
	if second.path != "b.md" {
		t.Fatalf("read %q after the move", second.path)
	}
	second.answer <- func() (*api.Note, error) { return &api.Note{Content: "start\n", Hash: d.Base()}, nil }
}

func TestTypingDuringARefusalIsHeld(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "file\n")
	d, b, g, _ := f.openGated("a.md")
	d.base = strings.Repeat("0", 64)
	b.typeAt(t, 0, 4, "typed")
	f.loop.Advance(SaveDelay)
	r := f.nextRead(g)
	b.typeAt(t, 5, 5, " more")
	reply := f.Read("a.md") // finish the RPC before the test can enter cleanup
	r.answer <- func() (*api.Note, error) { return reply, nil }
	f.loop.Until(t, "the refusal", func() bool { return d.Notice() != nil })
	if n := d.Notice(); n.Kind != NoticeRefused || n.Text != "typed more\n" {
		t.Fatalf("notice %+v", n)
	}
}

func TestTypingDuringARestoreStays(t *testing.T) {
	f := newFixture(t)
	hash := f.Create("a.md", "kept\n")
	d, b, g, _ := f.openGated("a.md")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	d.Restore()
	r := f.nextRead(g)
	b.typeAt(t, 4, 4, " typed")
	reply := f.Read("a.md") // finish the RPC before the test can enter cleanup
	r.answer <- func() (*api.Note, error) { return reply, nil }
	f.loop.Until(t, "the restore", func() bool { return d.Notice() == nil })
	f.settle(d)
	if b.text != "kept typed\n" {
		t.Fatalf("buffer %q", b.text)
	}
	if got := f.Read("a.md").Content; got != "kept typed\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestARecreationIsTheOneWriteInFlight(t *testing.T) {
	f := newFixture(t)
	hash := f.Create("a.md", "text\n")
	d, b, g, _ := f.openGated("a.md")
	b.typeAt(t, 4, 4, " more")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	var release chan struct{}
	f.loop.Until(t, "the create", func() bool {
		select {
		case release = <-g.creates:
			return true
		default:
			return false
		}
	})
	// While it runs: typing, a save, and the removal's event again.
	b.typeAt(t, 9, 9, " again")
	d.Save()
	d.Removed()
	if n := g.writes.Load(); n != 0 {
		t.Fatalf("%d writes during the create", n)
	}
	if len(g.creates) != 0 {
		t.Fatal("a second create during the first")
	}
	close(release)
	r := f.nextRead(g)      // the second removal is checked
	reply := f.Read("a.md") // finish the RPC before the test can enter cleanup
	r.answer <- func() (*api.Note, error) { return reply, nil }
	f.settle(d)
	if got := f.Read("a.md").Content; got != "text more again\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestARestoreDuringASaveWaitsForIt(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "kept\n")
	d, _, _, _ := f.openGated("a.md")
	d.flight = &flight{sent: d.text, since: EmptyChanges(len(d.text))}
	d.Restore()
	if len(d.landed) != 1 {
		t.Fatal("the restore was dropped")
	}
}

func TestARenameDuringARestoreReadsTheNewPath(t *testing.T) {
	f := newFixture(t)
	hash := f.Create("a.md", "kept\n")
	d, _, g, _ := f.openGated("a.md")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	d.Restore()
	r := f.nextRead(g)
	d.Moved("b.md")
	reply := f.Read("a.md") // finish the RPC before the test can enter cleanup
	r.answer <- func() (*api.Note, error) { return reply, nil }
	next := f.nextRead(g)
	if next.path != "b.md" {
		t.Fatalf("read %q", next.path)
	}
	next.answer <- func() (*api.Note, error) { return &api.Note{Content: "kept\n", Hash: d.Base()}, nil }
}

func TestAFailedFirstReadSaysSoAndRetriesTheRead(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "text\n")
	g := &gate{NoteAPI: f.client(), loop: f.loop, reads: make(chan *heldRead, 4), creates: make(chan chan struct{}, 4)}
	g.held.Store(true)
	d := NewDocument(f.loop, func() NoteAPI { return g }, f.Vault, "a.md")
	d.Load()
	r := f.nextRead(g)
	r.answer <- func() (*api.Note, error) { return nil, &api.Error{Code: api.CodeUnavailable, Message: "busy"} }
	f.loop.Until(t, "the notice", func() bool { return d.Notice() != nil })
	if d.Notice().Kind != NoticeLoadFailed {
		t.Fatalf("notice %+v", d.Notice())
	}
	d.RetryNow()
	r = f.nextRead(g)
	reply := f.Read("a.md") // finish the RPC before the test can enter cleanup
	r.answer <- func() (*api.Note, error) { return reply, nil }
	f.loop.Until(t, "the note", d.Loaded)
	if d.Notice() != nil {
		t.Fatalf("notice %+v", d.Notice())
	}
}

func TestARefusedTextCountsAsUnsavedUntilDismissed(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "file\n")
	d, b := f.open("a.md")
	d.base = "0000000000000000000000000000000000000000000000000000000000000000"
	b.typeAt(t, 0, 4, "typed")
	f.loop.Advance(SaveDelay)
	f.loop.Until(t, "the refusal", func() bool { return d.Notice() != nil })
	if !d.Unsaved() {
		t.Fatal("the refused text does not count as unsaved")
	}
	d.Dismiss()
	if d.Unsaved() {
		t.Fatal("still unsaved after the user dismissed it")
	}
}

func TestAFrozenNoteSavesButTakesNoEdits(t *testing.T) {
	f := newFixture(t)
	f.Create("a.md", "file\n")
	d, b := f.open("a.md")
	b.typeAt(t, 0, 0, "typed ")
	thaw := d.Freeze("Busy.")
	if d.ReadOnly() != "Busy." {
		t.Fatalf("read-only for %q", d.ReadOnly())
	}
	cs, err := ChangesOf(len(d.Text()), Change{From: 0, To: 0, Insert: "late "})
	if err != nil {
		t.Fatal(err)
	}
	d.Edit(cs)
	if d.Text() != "typed file\n" {
		t.Fatalf("a frozen note took an edit: %q", d.Text())
	}
	done := false
	d.Flush(func() { done = true })
	f.loop.Until(t, "the save", func() bool { return done })
	if got := f.Read("a.md").Content; got != "typed file\n" {
		t.Fatalf("file holds %q", got)
	}
	thaw()
	if d.ReadOnly() != "" {
		t.Fatalf("still read-only for %q", d.ReadOnly())
	}
}
