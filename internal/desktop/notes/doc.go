// Package notes holds the desktop app's open notes: each note's text, the
// version it was read at, and the loop that saves it as it is typed and
// applies merges and other writers' changes
// (docs/design/app.md#documents-saves-and-merges, ADR-0022). It is the Go
// port of app/src/editor/document.ts and imports no UI: an editor attaches
// a Buffer, and tests drive it with a fake Env.
package notes

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// The save loop's timing, as the web app's (ADR-0022).
const (
	// SaveDelay and SaveMax are the debounce: 1 s after the last change,
	// at most 10 s after the first.
	SaveDelay = time.Second
	SaveMax   = 10 * time.Second
	// RetryFirst and RetryMax bound a failed save's retries: 2, 4, 8 …
	// 60 s.
	RetryFirst = 2 * time.Second
	RetryMax   = 60 * time.Second
	// callTimeout bounds one call to notesd.
	callTimeout = 30 * time.Second
)

// Env is how a document reaches the UI thread and the clock. Its methods
// may be called from any goroutine.
type Env interface {
	// Post runs f on the UI thread.
	Post(f func())
	// After runs f on the UI thread after d; stop cancels it.
	After(d time.Duration, f func()) (stop func())
	// Now is the time.
	Now() time.Time
}

// NoteAPI is what a document needs of notesd; api.NoteClient has it.
type NoteAPI interface {
	Read(ctx context.Context, p *api.NoteReadParams) (*api.Note, error)
	Write(ctx context.Context, p *api.NoteWriteParams) (*api.WriteResult, error)
	Create(ctx context.Context, p *api.NoteCreateParams) (*api.WriteResult, error)
	Versions(ctx context.Context, p *api.NoteVersionsParams) ([]api.Version, error)
	Revert(ctx context.Context, p *api.NoteRevertParams) (*api.WriteResult, error)
}

// Buffer is the editor's side of a document: the text the user edits.
type Buffer interface {
	// Apply shows another writer's change or a merge in the editor's text,
	// outside its undo history (ADR-0022). The buffer does not report it
	// back as the user's edit.
	Apply(cs ChangeSet)
	// Edit shows a user's change made outside the editor, as one step of
	// its undo. The buffer does not report it back either.
	Edit(cs ChangeSet)
	// Sync reports what the user typed since the last report, through
	// Document.Local. The document calls it before it changes or sends
	// the text, so its changes apply to the text the user sees.
	Sync()
}

// Status is where a document stands.
type Status int

// The statuses.
const (
	StatusLoading Status = iota
	StatusMissing
	StatusClean
	StatusDirty
	StatusSaving
)

// NoticeKind says what a notice is about (docs/specs/ui.md#notices-in-a-tab).
type NoticeKind int

// The kinds of notice.
const (
	// NoticeConflict: a merge kept the other side of some hunks; the
	// writer's text is version Version.
	NoticeConflict NoticeKind = iota + 1
	// NoticeRefused: notesd refused the base; the document now holds the
	// file, and Text what the editor held.
	NoticeRefused
	// NoticeFailed: a save failed and retries.
	NoticeFailed
	// NoticeDeleted: the note was deleted and nothing was unsaved.
	NoticeDeleted
	// NoticeLoadFailed: the note could not be read; Reason says why.
	NoticeLoadFailed
)

// Notice is what a document's tabs say about it.
type Notice struct {
	Kind      NoticeKind
	Conflicts int64
	By        string
	Version   int64  // the conflict version, 0 for none
	Text      string // NoticeRefused: what the editor held
	Reason    string // NoticeFailed
}

// Document is one open note, shared by every tab that shows it. Its
// methods run on the UI thread.
type Document struct {
	env   Env
	api   func() NoteAPI
	vault int64
	path  string

	// text is what the editor shows, with "\n" line endings; crlf says
	// the file has "\r\n" ones, which saves put back.
	text     string
	crlf     bool
	readOnly string
	frozen   int    // Freeze calls not yet thawed
	frozenBy string // why the last of them stops edits
	base     string // the hash notesd last confirmed: the next write's base
	baseText string // what base names, as text is
	buf      Buffer

	loaded, missing, dirty, disposed bool
	flight                           *flight
	seen                             string // a note.changed hash that came during a write
	notice                           *Notice

	debounce, maxWait, retry func()
	retryDelay               time.Duration
	listeners                []func()
	waiters                  []func() // Flush's callbacks
	landed                   []func() // what waits for the write in flight
	reads                    int      // numbers Reload's reads: only the newest counts
}

// flight is a write in progress, or a request that stands in for one: the
// text it sent, and what was typed since. creating says it makes a deleted
// note again, and removed that the note was removed again meanwhile.
type flight struct {
	sent     string
	since    ChangeSet
	creating bool
	removed  bool
}

// NewDocument makes the document of a note; client returns notesd's note
// domain, or nil while it is not connected. Load reads it.
func NewDocument(env Env, client func() NoteAPI, vault int64, path string) *Document {
	return &Document{env: env, api: client, vault: vault, path: path, retryDelay: RetryFirst}
}

// Path is the note's path in its vault.
func (d *Document) Path() string { return d.path }

// Vault is the note's vault.
func (d *Document) Vault() int64 { return d.vault }

// Text is the document's text, with "\n" line endings.
func (d *Document) Text() string { return d.text }

// Base is the hash of the content notesd last confirmed.
func (d *Document) Base() string { return d.base }

// CRLF reports whether the file has "\r\n" line endings.
func (d *Document) CRLF() bool { return d.crlf }

// ReadOnly says why the note cannot be edited here, "" when it can.
func (d *Document) ReadOnly() string {
	if d.frozen > 0 {
		return d.frozenBy
	}
	return d.readOnly
}

// Freeze stops the user's edits, for reason, until the func it returns runs.
// Saves and other writers' changes go on.
func (d *Document) Freeze(reason string) (thaw func()) {
	d.syncBuffer()
	d.frozen++
	d.frozenBy = reason
	d.notify()
	return func() {
		d.frozen--
		d.notify()
	}
}

// Loaded reports whether the note has been read.
func (d *Document) Loaded() bool { return d.loaded }

// Unsaved reports whether typed text has not reached notesd yet: text to
// save, or text notesd refused, which only its notice holds until the user
// merges or dismisses it.
func (d *Document) Unsaved() bool {
	return d.dirty || d.flight != nil || (d.notice != nil && d.notice.Kind == NoticeRefused)
}

// Notice is what the tabs say about the document, nil for nothing.
func (d *Document) Notice() *Notice { return d.notice }

// Status is where the document stands.
func (d *Document) Status() Status {
	switch {
	case !d.loaded && d.missing:
		return StatusMissing
	case !d.loaded:
		return StatusLoading
	case d.flight != nil:
		return StatusSaving
	case d.dirty:
		return StatusDirty
	}
	return StatusClean
}

// OnChange calls fn on the UI thread whenever the status, the notice or
// the text changes.
func (d *Document) OnChange(fn func()) { d.listeners = append(d.listeners, fn) }

func (d *Document) notify() {
	for _, fn := range d.listeners {
		fn()
	}
}

// Dismiss clears the notice.
func (d *Document) Dismiss() {
	d.notice = nil
	d.notify()
}

// Attach gives the document its editor buffer; nil detaches it, which
// saves at once.
func (d *Document) Attach(b Buffer) {
	d.buf = b
	if b == nil {
		d.Save()
	}
}

// fromFile splits a file's content into the editor's text and its line
// endings: "\r\n" everywhere is crlf; a mix of "\r\n" and "\n" cannot be
// edited without changing bytes the user did not touch.
func fromFile(content string) (text string, crlf, mixed bool) {
	r := strings.Count(content, "\r\n")
	if r == 0 {
		return content, false, false
	}
	if r == strings.Count(content, "\n") {
		return strings.ReplaceAll(content, "\r\n", "\n"), true, false
	}
	return content, false, true
}

// toFile is the content a text saves as.
func (d *Document) toFile(text string) string {
	if d.crlf {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// mixedEndings is ReadOnly's reason for a note with mixed line endings.
const mixedEndings = "This note mixes Windows and Unix line endings; edit it in another editor, or make them one kind."

// adopt takes a file's content as the document's base text; it reports
// the text the editor should show.
func (d *Document) adopt(content string) string {
	text, crlf, mixed := fromFile(content)
	d.crlf = crlf
	d.readOnly = ""
	if mixed {
		d.readOnly = mixedEndings
		text = strings.ReplaceAll(content, "\r\n", "\n")
	}
	d.baseText = text
	return text
}

func (d *Document) call(f func(ctx context.Context, c NoteAPI) error, done func(error)) {
	c := d.api()
	if c == nil {
		done(errNotConnected)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := f(ctx, c)
		d.env.Post(func() {
			d.syncBuffer()
			done(err)
		})
	}()
}

// syncBuffer has the buffer report what was typed since its last report.
func (d *Document) syncBuffer() {
	if d.buf != nil {
		d.buf.Sync()
	}
}

var errNotConnected = errors.New("not connected to notesd")

// Load reads the note.
func (d *Document) Load() {
	path := d.path
	var n *api.Note
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		n, err = c.Read(ctx, &api.NoteReadParams{VaultID: d.vault, Path: path})
		return err
	}, func(err error) {
		if d.loaded || d.disposed {
			return
		}
		if d.path != path {
			d.Load() // moved meanwhile
			return
		}
		switch {
		case err == nil:
			d.text = d.adopt(n.Content)
			d.base = n.Hash
			d.loaded, d.missing = true, false
			if d.notice != nil && d.notice.Kind == NoticeLoadFailed {
				d.notice = nil
			}
		case isCode(err, api.CodeNotFound):
			d.missing = true
		case errors.Is(err, errNotConnected):
			return // Reconnected loads it
		default:
			d.notice = &Notice{Kind: NoticeLoadFailed, Reason: err.Error()}
		}
		d.notify()
	})
}

// Local takes an edit the user made in the editor: the buffer already
// shows it. cs applies to the document's text.
func (d *Document) Local(cs ChangeSet) {
	if d.change(cs) {
		d.notify()
	}
}

// change applies the user's change cs to the text: a typed edit, an undo,
// a redo or an edit from outside the editor.
func (d *Document) change(cs ChangeSet) bool {
	if cs.Empty() || !d.loaded || d.ReadOnly() != "" {
		return false
	}
	text, err := cs.Apply(d.text)
	if err != nil {
		return false // the buffer and the document disagree; the next sync diffs them
	}
	d.text = text
	if d.flight != nil {
		if since, err := d.flight.since.Compose(cs); err == nil {
			d.flight.since = since
		}
	}
	d.markDirty()
	return true
}

// Edit makes a user's change from outside the editor, as a property in
// the panel: the buffer shows it, and its undo takes it back.
func (d *Document) Edit(cs ChangeSet) {
	d.syncBuffer()
	if d.change(cs) {
		if d.buf != nil {
			d.buf.Edit(cs)
		}
		d.notify()
	}
}

// EditIfText applies an outside edit only while the visible buffer still has
// base. Rendered controls use it to reject clicks from an obsolete rendering.
func (d *Document) EditIfText(base string, cs ChangeSet) bool {
	d.syncBuffer()
	if d.text != base || !d.change(cs) {
		return false
	}
	if d.buf != nil {
		d.buf.Edit(cs)
	}
	d.notify()
	return true
}

func (d *Document) markDirty() {
	first := !d.dirty
	d.dirty = true
	stop(&d.debounce)
	d.debounce = d.env.After(SaveDelay, d.Save)
	if first && d.maxWait == nil {
		d.maxWait = d.env.After(SaveMax, d.Save)
	}
}

func stop(f *func()) {
	if *f != nil {
		(*f)()
		*f = nil
	}
}

// applyRemote applies another writer's change to the text and the buffer.
func (d *Document) applyRemote(cs ChangeSet) {
	if cs.Empty() {
		return
	}
	text, err := cs.Apply(d.text)
	if err != nil {
		return
	}
	d.text = text
	if d.buf != nil {
		d.buf.Apply(cs)
	}
}

// Save writes the text now, unless nothing is unsaved or a write is in
// flight.
func (d *Document) Save() {
	d.syncBuffer()
	if !d.loaded || !d.dirty || d.disposed || d.flight != nil || d.readOnly != "" {
		d.settle()
		return
	}
	stop(&d.debounce)
	stop(&d.maxWait)
	stop(&d.retry)
	sent := d.text
	f := &flight{sent: sent, since: EmptyChanges(len(sent))}
	d.flight = f
	d.dirty = false
	d.notify()
	params := &api.NoteWriteParams{VaultID: d.vault, Path: d.path, Base: d.base, Content: d.toFile(sent)}
	var res *api.WriteResult
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		res, err = c.Write(ctx, params)
		return err
	}, func(err error) {
		if err != nil {
			d.failed(err)
		} else {
			d.answered(res)
		}
		d.land()
	})
}

// land runs what waited for the request in flight to end.
func (d *Document) land() {
	landed := d.landed
	d.landed = nil
	for _, f := range landed {
		f()
	}
	d.settle()
}

// catchUp handles a note.changed that came during the request in flight.
func (d *Document) catchUp() {
	seen := d.seen
	d.seen = ""
	if seen != "" && seen != d.base {
		d.Changed(seen)
	}
}

// Flush saves now and calls done once nothing is unsaved, or a failed
// save waits to retry.
func (d *Document) Flush(done func()) {
	d.waiters = append(d.waiters, done)
	d.Save()
}

// settle calls the Flush callbacks once the document has nothing to save.
func (d *Document) settle() {
	if len(d.waiters) == 0 || d.flight != nil {
		return
	}
	if d.dirty && d.retry == nil && d.loaded && !d.disposed && d.readOnly == "" {
		return // a save follows
	}
	ws := d.waiters
	d.waiters = nil
	for _, w := range ws {
		w()
	}
}

// RetryNow runs a waiting retry at once.
func (d *Document) RetryNow() {
	if !d.loaded {
		d.Load()
		return
	}
	stop(&d.retry)
	d.Save()
}

func (d *Document) answered(res *api.WriteResult) {
	f := d.flight
	d.flight = nil
	if f == nil {
		return
	}
	d.base = res.Hash
	d.retryDelay = RetryFirst
	if d.notice != nil && d.notice.Kind == NoticeFailed {
		d.notice = nil
	}
	if res.Merged && res.Content != nil {
		merged := d.adopt(*res.Content)
		if remote, err := Diff(f.sent, merged).Map(f.since, false); err == nil {
			d.applyRemote(remote)
		}
		if res.Conflicts > 0 {
			by := "another writer"
			if res.MergedAuthor != nil && *res.MergedAuthor != "" {
				by = *res.MergedAuthor
			} else if res.MergedSource != nil && *res.MergedSource != "" {
				by = string(*res.MergedSource)
			}
			n := &Notice{Kind: NoticeConflict, Conflicts: res.Conflicts, By: by}
			if res.ConflictVersion != nil {
				n.Version = *res.ConflictVersion
			}
			d.notice = n
		}
	} else {
		d.baseText = f.sent
	}
	if !f.since.Empty() {
		d.dirty = true
	}
	d.notify()
	if d.dirty {
		d.Save()
	}
	d.catchUp()
}

// refusedBase matches notesd's conflict when it has no base to merge from.
var refusedBase = regexp.MustCompile(`no version|is gone`)

func (d *Document) failed(err error) {
	d.flight = nil
	d.dirty = true
	switch {
	case isCode(err, api.CodeNotFound):
		d.Removed()
	case isCode(err, api.CodeConflict) && refusedBase.MatchString(err.Error()):
		d.refused()
	default:
		d.retryLater(err.Error())
	}
}

func (d *Document) retryLater(reason string) {
	d.notice = &Notice{Kind: NoticeFailed, Reason: reason}
	stop(&d.retry)
	d.retry = d.env.After(d.retryDelay, func() {
		d.retry = nil
		d.Save()
	})
	d.retryDelay = min(d.retryDelay*2, RetryMax)
	d.notify()
}

// refused: notesd no longer has the base, so it cannot merge. The document
// takes the file's text, and the editor's is kept for the merge view, so
// nothing typed is lost and nothing is overwritten.
func (d *Document) refused() {
	path := d.path
	d.flight = &flight{sent: d.text, since: EmptyChanges(len(d.text))}
	d.reads++
	var n *api.Note
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		n, err = c.Read(ctx, &api.NoteReadParams{VaultID: d.vault, Path: path})
		return err
	}, func(err error) {
		d.flight = nil
		switch {
		case d.path != path:
			d.refused() // moved meanwhile
			return
		case err != nil:
			d.retryLater(err.Error())
		default:
			held := d.text // with what was typed during the read
			text := d.adopt(n.Content)
			d.applyRemote(Diff(d.text, text))
			d.base = n.Hash
			stop(&d.debounce)
			stop(&d.maxWait)
			d.dirty = false
			d.notice = &Notice{Kind: NoticeRefused, Text: held}
			d.notify()
			d.catchUp()
		}
		d.land()
	})
}

// Changed handles note.changed: another writer's content, unless it is
// the base.
func (d *Document) Changed(hash string) {
	d.syncBuffer()
	if !d.loaded || hash == d.base {
		return
	}
	if d.flight != nil {
		d.seen = hash
		return
	}
	if d.dirty {
		d.Save() // notesd merges what is unsaved into the new content
		return
	}
	d.Reload()
}

// Reload reads the note and applies the difference as another writer's
// change.
func (d *Document) Reload() {
	if !d.loaded {
		return
	}
	path := d.path
	d.reads++
	read := d.reads
	var n *api.Note
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		n, err = c.Read(ctx, &api.NoteReadParams{VaultID: d.vault, Path: path})
		return err
	}, func(err error) {
		if read != d.reads || d.disposed {
			return // a newer read answers
		}
		if d.path != path {
			d.Reload() // moved meanwhile
			return
		}
		if err != nil {
			return // a note that is gone says so through note.removed
		}
		if d.flight != nil || d.dirty {
			if d.dirty {
				d.Save()
			}
			return
		}
		if n.Hash == d.base {
			return
		}
		text := d.adopt(n.Content)
		d.applyRemote(Diff(d.text, text))
		d.base = n.Hash
		if d.notice != nil && d.notice.Kind == NoticeDeleted {
			d.notice = nil
		}
		d.notify()
	})
}

// Reconnected catches up after the connection came back: saves what is
// unsaved, else reads again.
func (d *Document) Reconnected() {
	switch {
	case !d.loaded:
		d.Load()
	case d.dirty:
		d.Save()
	default:
		d.Reload()
	}
}

// Moved follows the note to a new path.
func (d *Document) Moved(to string) {
	d.path = to
	d.notify()
}

// Removed handles note.removed: unsaved text creates the note again, as
// an edit beats a delete (ADR-0005); otherwise the tabs say it is gone.
func (d *Document) Removed() {
	if !d.loaded {
		return
	}
	if d.flight != nil {
		if d.flight.creating {
			d.flight.removed = true
			return
		}
		d.landed = append(d.landed, d.Removed)
		return
	}
	if !d.dirty {
		d.notice = &Notice{Kind: NoticeDeleted}
		d.notify()
		return
	}
	text := d.text
	f := &flight{sent: text, since: EmptyChanges(len(text)), creating: true}
	d.flight = f
	d.dirty = false
	d.reads++
	stop(&d.debounce)
	stop(&d.maxWait)
	stop(&d.retry)
	params := &api.NoteCreateParams{VaultID: d.vault, Path: d.path, Content: d.toFile(text)}
	var res *api.WriteResult
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		res, err = c.Create(ctx, params)
		return err
	}, func(err error) {
		d.flight = nil
		if err != nil {
			d.dirty = true
			d.retryLater(err.Error())
			d.land()
			return
		}
		d.base = res.Hash
		d.baseText = text
		d.dirty = !f.since.Empty()
		d.notice = nil
		d.notify()
		if f.removed {
			d.recheck()
		}
		if d.dirty {
			d.Save()
		}
		d.catchUp()
		d.land()
	})
}

// recheck reads a note that was removed again while it was being made
// again: whichever came last decides.
func (d *Document) recheck() {
	path := d.path
	d.call(func(ctx context.Context, c NoteAPI) (err error) {
		_, err = c.Read(ctx, &api.NoteReadParams{VaultID: d.vault, Path: path})
		return err
	}, func(err error) {
		if isCode(err, api.CodeNotFound) && d.path == path && !d.disposed {
			d.Removed()
		}
	})
}

// Restore brings a deleted note back from its last version. What is typed
// meanwhile stays, on top of it.
func (d *Document) Restore() {
	if d.flight != nil {
		d.landed = append(d.landed, d.Restore)
		return
	}
	path := d.path
	f := &flight{sent: d.text, since: EmptyChanges(len(d.text))}
	d.flight = f
	d.reads++
	var res *api.WriteResult
	var n *api.Note
	d.call(func(ctx context.Context, c NoteAPI) error {
		versions, err := c.Versions(ctx, &api.NoteVersionsParams{VaultID: d.vault, Path: path})
		if err != nil {
			return err
		}
		var last *api.Version
		for i := range versions {
			if !versions[i].Deleted {
				last = &versions[i]
				break
			}
		}
		if last == nil {
			return errors.New("the note has no version to restore")
		}
		if res, err = c.Revert(ctx, &api.NoteRevertParams{VaultID: d.vault, Path: path, Version: last.ID}); err != nil {
			return err
		}
		n, err = c.Read(ctx, &api.NoteReadParams{VaultID: d.vault, Path: path})
		return err
	}, func(err error) {
		d.flight = nil
		switch {
		case d.path != path:
			d.Reload() // moved meanwhile
		case err != nil:
			d.notice = &Notice{Kind: NoticeFailed, Reason: err.Error()}
			d.notify()
		default:
			text := d.adopt(n.Content)
			if remote, err := Diff(f.sent, text).Map(f.since, false); err == nil {
				d.applyRemote(remote)
			}
			d.base = res.Hash
			d.dirty = !f.since.Empty()
			d.notice = nil
			d.notify()
		}
		if d.dirty {
			d.Save()
		}
		d.catchUp()
		d.land()
	})
}

// Dispose stops the document's timers; it does nothing more after.
func (d *Document) Dispose() {
	d.disposed = true
	stop(&d.debounce)
	stop(&d.maxWait)
	stop(&d.retry)
	d.listeners = nil
}

func isCode(err error, code api.ErrorCode) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Code == code
}
