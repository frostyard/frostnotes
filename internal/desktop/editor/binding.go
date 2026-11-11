package editor

import (
	code "github.com/egoist/mygo/ui/editor"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// Binding joins a note's document to the MyGo buffer every tab of the
// note edits: MyGo's buffers are shared by editors, as the web app's
// documents are shared by views. It implements notes.Buffer. UI thread
// only.
type Binding struct {
	doc *notes.Document
	buf *code.Buffer
	// last is the buffer's text as the document last had it, and version
	// the buffer's version then: an edit since is the user's.
	last    string
	version uint64
	loaded  bool
}

// Bind makes the buffer of doc and attaches it.
func Bind(doc *notes.Document) *Binding {
	b := &Binding{doc: doc, buf: code.NewBuffer(doc.Text())}
	b.buf.SetLanguage(Language())
	b.last, b.version, b.loaded = doc.Text(), b.buf.Version(), doc.Loaded()
	doc.OnChange(b.changed)
	doc.Attach(b)
	return b
}

// Document is the bound document.
func (b *Binding) Document() *notes.Document { return b.doc }

// Buffer is the bound buffer, which every tab's editor of the note shows.
func (b *Binding) Buffer() *code.Buffer { return b.buf }

func (b *Binding) changed() {
	if !b.loaded && b.doc.Loaded() {
		b.loaded = true
		b.buf.Reset(b.doc.Text())
		b.last, b.version = b.doc.Text(), b.buf.Version()
	}
}

// Sync reports the user's edits since the last report to the document.
func (b *Binding) Sync() {
	v := b.buf.Version()
	if v == b.version {
		return
	}
	b.version = v
	text := b.buf.String()
	if text == b.last {
		return
	}
	cs := notes.Diff(b.last, text)
	b.last = text
	b.doc.Local(cs)
	if b.doc.Text() != text {
		// The document took no edit (it is read-only, or not loaded): the
		// buffer shows the document again, which overlaps the step and takes
		// it out of the history.
		b.Apply(notes.Diff(text, b.doc.Text()))
	}
}

// Apply shows another writer's change or a merge, outside the buffer's
// undo history: undo moves its steps past it (ADR-0022).
func (b *Binding) Apply(cs notes.ChangeSet) { b.show(cs, b.buf.EditRemote) }

// Edit shows a user's change made outside the editor, as one step of undo.
func (b *Binding) Edit(cs notes.ChangeSet) { b.show(cs, b.buf.Edit) }

// show makes a change the document made in the buffer; the buffer does not
// report it.
func (b *Binding) show(cs notes.ChangeSet, edit func(...code.Edit)) {
	if cs.Empty() {
		return
	}
	changes := cs.Changes()
	edits := make([]code.Edit, len(changes))
	for i, c := range changes {
		edits[i] = code.Edit{Start: c.From, End: c.To, Text: c.Insert}
	}
	edit(edits...)
	if next, err := cs.Apply(b.last); err == nil {
		b.last = next
	}
	if s := b.buf.String(); s != b.last {
		b.last = s
	}
	b.version = b.buf.Version()
}

// Undo takes back the user's newest step, never another writer's change,
// for a view without an editor of its own to run it.
func (b *Binding) Undo() bool { return b.step(b.buf.Undo) }

// Redo makes the newest undone step again.
func (b *Binding) Redo() bool { return b.step(b.buf.Redo) }

func (b *Binding) step(take func() bool) bool {
	b.Sync()
	if !b.doc.Loaded() || b.doc.ReadOnly() != "" {
		return false
	}
	done := take()
	b.Sync()
	return done
}

// Close detaches the buffer from the document, which saves at once.
func (b *Binding) Close() {
	b.Sync()
	b.doc.Attach(nil)
}
