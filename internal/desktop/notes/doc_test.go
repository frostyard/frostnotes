package notes

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// testLoop stands in for the UI thread: workers Post to it from any
// goroutine, and the test runs what they posted on its own goroutine.
// Its clock moves only when the test advances it.
type testLoop struct {
	posted chan func()
	mu     sync.Mutex
	now    time.Duration
	timers []*testTimer
}

type testTimer struct {
	at      time.Duration
	f       func()
	stopped bool
}

func newTestLoop() *testLoop { return &testLoop{posted: make(chan func(), 64)} }

var epoch = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func (l *testLoop) Now() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return epoch.Add(l.now)
}

func (l *testLoop) Post(f func()) { l.posted <- f }

func (l *testLoop) After(d time.Duration, f func()) func() {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := &testTimer{at: l.now + d, f: f}
	l.timers = append(l.timers, t)
	return func() { t.stopped = true }
}

// advance moves the clock by d and runs the timers due, in order.
func (l *testLoop) advance(d time.Duration) {
	l.mu.Lock()
	l.now += d
	var due []*testTimer
	keep := l.timers[:0]
	for _, t := range l.timers {
		if t.at <= l.now {
			due = append(due, t)
		} else {
			keep = append(keep, t)
		}
	}
	l.timers = keep
	l.mu.Unlock()
	slices.SortStableFunc(due, func(a, b *testTimer) int { return int(a.at - b.at) })
	for _, t := range due {
		if !t.stopped {
			t.f()
		}
	}
}

// until runs what workers post until cond holds.
func (l *testLoop) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case f := <-l.posted:
			f()
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// fakeBuffer is an editor's buffer: its text follows the document's.
type fakeBuffer struct {
	doc    *Document
	text   string
	remote int
}

func (b *fakeBuffer) Apply(cs ChangeSet) {
	b.text, _ = cs.Apply(b.text)
	b.remote++
}

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
	t     *testing.T
	srv   *rpctest.Server
	c     *api.Client // the app's connection
	other *api.Client // another writer's
	vault int64
	dir   string
	loop  *testLoop
	conn  bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	srv := rpctest.Start(t)
	f := &fixture{t: t, srv: srv, c: srv.Dial(t), other: srv.Dial(t), dir: t.TempDir(), loop: newTestLoop(), conn: true}
	v, err := f.c.Vault().Add(t.Context(), &api.VaultAddParams{Path: f.dir})
	if err != nil {
		t.Fatal(err)
	}
	f.vault = v.ID
	return f
}

func (f *fixture) create(path, content string) string {
	f.t.Helper()
	res, err := f.other.Note().Create(f.t.Context(), &api.NoteCreateParams{VaultID: f.vault, Path: path, Content: content})
	if err != nil {
		f.t.Fatal(err)
	}
	return res.Hash
}

func (f *fixture) read(path string) *api.Note {
	f.t.Helper()
	n, err := f.other.Note().Read(f.t.Context(), &api.NoteReadParams{VaultID: f.vault, Path: path})
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

// open loads a document with a buffer attached.
func (f *fixture) open(path string) (*Document, *fakeBuffer) {
	f.t.Helper()
	d := NewDocument(f.loop, func() NoteAPI {
		if !f.conn {
			return nil
		}
		return f.c.Note()
	}, f.vault, path)
	d.Load()
	f.loop.until(f.t, "the note to load", func() bool { return d.Loaded() || d.Status() == StatusMissing })
	b := &fakeBuffer{doc: d, text: d.Text()}
	d.Attach(b)
	return d, b
}

func (f *fixture) settle(d *Document) {
	f.t.Helper()
	f.loop.until(f.t, "the save", func() bool { return d.Status() == StatusClean })
}

func TestTypingSavesAfterTheDelay(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "hello\n")
	d, b := f.open("a.md")
	b.typeAt(t, 5, 5, " world")
	if d.Status() != StatusDirty {
		t.Fatalf("status %v after typing", d.Status())
	}
	f.loop.advance(SaveDelay - time.Millisecond)
	if d.Status() != StatusDirty {
		t.Fatal("saved before the delay")
	}
	f.loop.advance(time.Millisecond)
	f.settle(d)
	if got := f.read("a.md").Content; got != "hello world\n" {
		t.Fatalf("file holds %q", got)
	}
	if d.Base() != f.read("a.md").Hash {
		t.Fatal("the base is not the saved content's hash")
	}
}

func TestTypingWithoutPauseSavesAtTheMaximum(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "x")
	d, b := f.open("a.md")
	for i := range 20 {
		b.typeAt(t, len(b.text), len(b.text), "y")
		f.loop.advance(SaveMax / 20)
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
	base := f.create("a.md", "one\ntwo\nthree\n")
	d, b := f.open("a.md")

	// Another writer changes the last line.
	if _, err := f.other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.vault, Path: "a.md", Base: base, Content: "one\ntwo\nTHREE\n"}); err != nil {
		t.Fatal(err)
	}
	// The app changes the first, saves, and types more before the answer.
	b.typeAt(t, 0, 3, "ONE")
	f.loop.advance(SaveDelay)
	if d.Status() != StatusSaving {
		t.Fatal("no save in flight")
	}
	b.typeAt(t, 7, 7, " and a half") // "ONE\ntwo and a half\n…"
	f.loop.until(t, "the merged answer", func() bool { return d.Status() != StatusSaving })

	want := "ONE\ntwo and a half\nTHREE\n"
	if d.Text() != want || b.text != want {
		t.Fatalf("document %q, buffer %q, want %q", d.Text(), b.text, want)
	}
	if b.remote == 0 {
		t.Fatal("the merge did not reach the buffer as a remote change")
	}
	f.settle(d)
	if got := f.read("a.md").Content; got != want {
		t.Fatalf("file holds %q", got)
	}
}

func TestAConflictLeavesANotice(t *testing.T) {
	f := newFixture(t)
	base := f.create("a.md", "line\n")
	d, b := f.open("a.md")
	if _, err := f.other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.vault, Path: "a.md", Base: base, Content: "theirs\n"}); err != nil {
		t.Fatal(err)
	}
	b.typeAt(t, 0, 4, "mine")
	f.loop.advance(SaveDelay)
	f.settle(d)
	n := d.Notice()
	if n == nil || n.Kind != NoticeConflict || n.Conflicts == 0 || n.Version == 0 {
		t.Fatalf("notice %+v", n)
	}
	if d.Text() != f.read("a.md").Content {
		t.Fatal("the document is not the file after the merge")
	}
	d.Dismiss()
	if d.Notice() != nil {
		t.Fatal("Dismiss left the notice")
	}
}

func TestAnotherWritersChangeReachesACleanDocument(t *testing.T) {
	f := newFixture(t)
	base := f.create("a.md", "a\n")
	d, b := f.open("a.md")
	res, err := f.other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.vault, Path: "a.md", Base: base, Content: "a\nb\n"})
	if err != nil {
		t.Fatal(err)
	}
	d.Changed(res.Hash)
	f.loop.until(t, "the reload", func() bool { return d.Base() == res.Hash })
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
	f.create("a.md", "file\n")
	d, b := f.open("a.md")
	d.base = "0000000000000000000000000000000000000000000000000000000000000000"
	b.typeAt(t, 0, 4, "typed")
	f.loop.advance(SaveDelay)
	f.loop.until(t, "the refusal", func() bool { return d.Notice() != nil })
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
	hash := f.create("a.md", "gone soon\n")
	d, b := f.open("a.md")
	if err := f.other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	if n := d.Notice(); n == nil || n.Kind != NoticeDeleted {
		t.Fatalf("notice %+v", n)
	}
	d.Restore()
	f.loop.until(t, "the restore", func() bool { return d.Notice() == nil })
	if f.read("a.md").Content != "gone soon\n" || b.text != "gone soon\n" {
		t.Fatal("the note did not come back")
	}
}

func TestUnsavedTextCreatesADeletedNoteAgain(t *testing.T) {
	f := newFixture(t)
	hash := f.create("a.md", "text\n")
	d, b := f.open("a.md")
	b.typeAt(t, 4, 4, " more")
	if err := f.other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.vault, Path: "a.md", Base: hash}); err != nil {
		t.Fatal(err)
	}
	d.Removed()
	f.settle(d)
	if got := f.read("a.md").Content; got != "text more\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestCRLFNotesKeepTheirLineEndings(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "one\r\ntwo\r\n")
	d, b := f.open("a.md")
	if d.Text() != "one\ntwo\n" || !d.CRLF() || d.ReadOnly() != "" {
		t.Fatalf("text %q crlf %v read-only %q", d.Text(), d.CRLF(), d.ReadOnly())
	}
	b.typeAt(t, 3, 3, "!")
	f.loop.advance(SaveDelay)
	f.settle(d)
	if got := f.read("a.md").Content; got != "one!\r\ntwo\r\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestMixedLineEndingsAreReadOnly(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "one\r\ntwo\n")
	d, b := f.open("a.md")
	if d.ReadOnly() == "" {
		t.Fatal("a note with mixed line endings is editable")
	}
	b.typeAt(t, 0, 0, "x")
	f.loop.advance(SaveMax)
	if d.Status() != StatusClean {
		t.Fatal("a read-only note saved")
	}
	if got := f.read("a.md").Content; got != "one\r\ntwo\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestASaveWithoutNotesdRetries(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "a")
	d, b := f.open("a.md")
	f.conn = false
	b.typeAt(t, 1, 1, "b")
	f.loop.advance(SaveDelay)
	if n := d.Notice(); n == nil || n.Kind != NoticeFailed {
		t.Fatalf("notice %+v", n)
	}
	f.conn = true
	f.loop.advance(RetryFirst)
	f.settle(d)
	if d.Notice() != nil {
		t.Fatal("the failure notice stayed after the save")
	}
	if f.read("a.md").Content != "ab" {
		t.Fatal("the retry did not save")
	}
}

func TestFlushWaitsForTheSave(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "a")
	d, b := f.open("a.md")
	b.typeAt(t, 1, 1, "b")
	flushed := false
	d.Flush(func() { flushed = true })
	f.loop.until(t, "the flush", func() bool { return flushed })
	if d.Unsaved() || f.read("a.md").Content != "ab" {
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
	f.create("a.md", "- [ ] task\n")
	d, _ := f.open("a.md")
	d.Attach(nil)
	cs, _ := ChangesOf(len(d.Text()), Change{From: 3, To: 4, Insert: "x"})
	d.Edit(cs)
	f.loop.advance(SaveDelay)
	f.settle(d)
	if got := f.read("a.md").Content; got != "- [x] task\n" {
		t.Fatalf("file holds %q", got)
	}
}

// An outside writer (an editor saving in place) changes the file on disk;
// the document follows it through note.changed.
func TestAnOutsideEditReachesTheDocument(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "start\n")
	d, b := f.open("a.md")
	if err := os.WriteFile(filepath.Join(f.dir, "a.md.tmp"), []byte("start\nvim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(f.dir, "a.md.tmp"), filepath.Join(f.dir, "a.md")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		n := f.read("a.md")
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
	f.loop.until(t, "the reload", func() bool { return b.text == "start\nvim\n" })
}

func TestUndoLeavesAnotherWritersChange(t *testing.T) {
	f := newFixture(t)
	f.create("a.md", "one\ntwo\n")
	d, b := f.open("a.md")
	// The user types a word, a pause, then another.
	b.typeAt(t, 3, 3, " 1")
	f.loop.advance(SaveDelay)
	f.settle(d)
	b.typeAt(t, 9, 9, " 2")
	// Another writer adds a line at the top.
	res, err := f.other.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: f.vault, Path: "a.md", Base: d.Base(), Content: "zero\none 1\ntwo\n"})
	if err != nil {
		t.Fatal(err)
	}
	d.Changed(res.Hash) // dirty: saves, and notesd merges
	f.settle(d)
	if b.text != "zero\none 1\ntwo 2\n" {
		t.Fatalf("after the merge the buffer holds %q", b.text)
	}
	if _, ok := d.Undo(); !ok {
		t.Fatal("nothing to undo")
	}
	if b.text != "zero\none 1\ntwo\n" || d.Text() != b.text {
		t.Fatalf("undo made %q", b.text)
	}
	if _, ok := d.Undo(); !ok {
		t.Fatal("the first word cannot be undone")
	}
	if b.text != "zero\none\ntwo\n" {
		t.Fatalf("second undo made %q", b.text)
	}
	if _, ok := d.Undo(); ok {
		t.Fatal("undo took back the other writer's line")
	}
	if _, ok := d.Redo(); !ok || b.text != "zero\none 1\ntwo\n" {
		t.Fatalf("redo made %q", b.text)
	}
	f.loop.advance(SaveDelay)
	f.settle(d)
	if got := f.read("a.md").Content; got != "zero\none 1\ntwo\n" {
		t.Fatalf("file holds %q", got)
	}
}
