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
		// The document took no edit (it is read-only, or not loaded):
		// the buffer shows the document again.
		b.Apply(notes.Diff(text, b.doc.Text()))
	}
}

// Apply shows a change the document made; the buffer does not report it.
func (b *Binding) Apply(cs notes.ChangeSet) {
	if cs.Empty() {
		return
	}
	changes := cs.Changes()
	edits := make([]code.Edit, len(changes))
	for i, c := range changes {
		edits[i] = code.Edit{Start: c.From, End: c.To, Text: c.Insert}
	}
	b.buf.Edit(edits...)
	if next, err := cs.Apply(b.last); err == nil {
		b.last = next
	}
	if s := b.buf.String(); s != b.last {
		b.last = s
	}
	b.version = b.buf.Version()
}

// Close detaches the buffer from the document, which saves at once.
func (b *Binding) Close() {
	b.Sync()
	b.doc.Attach(nil)
}
