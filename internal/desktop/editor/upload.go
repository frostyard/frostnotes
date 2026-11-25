package editor

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/markup"
)

// Attachments: the rules (where a file goes, its name, its embed) are
// markup's; the editor takes files from the clipboard and from drops.

// Uploader stores attachments beside a note (ADR-0023) and reports the
// embeds to insert, through done on the UI thread.
type Uploader interface {
	Upload(files []markup.File, done func(embeds string, err error))
}

// Clipboard is what the editor asks of the system's clipboard when text is
// pasted; the window gives it MyGo's.
type Clipboard interface {
	// Files are the local paths on the clipboard.
	Files() []string
	// PNG is an image on the clipboard, nil for none.
	PNG() []byte
}

// insertEmbeds writes text at the caret, the caret after it; at the start
// of a line that holds text, the text gets a line of its own.
func (e *NoteEditor) insertEmbeds(text string) {
	e.Run(func(doc string, sels []markup.Sel) markup.Edit { return markup.InsertEmbeds(doc, sels, text) })
}

func (e *NoteEditor) report(err error) {
	if e.OnError != nil {
		e.OnError(err)
	}
}

// Attach uploads files beside the note and inserts their embeds at the
// caret.
func (e *NoteEditor) Attach(files []markup.File) {
	if e.Uploads == nil || len(files) == 0 {
		return
	}
	release := func() {}
	if e.Hold != nil {
		release = e.Hold()
	}
	e.Uploads.Upload(files, func(embeds string, err error) {
		defer release()
		if err != nil {
			e.report(err)
			return
		}
		e.insertEmbeds(embeds)
	})
}

// AttachPaths attaches local files, which the upload reads off the UI
// thread.
func (e *NoteEditor) AttachPaths(paths []string) {
	files := make([]markup.File, 0, len(paths))
	for _, p := range paths {
		files = append(files, markup.File{Name: markup.CleanName(filepath.Base(p)), Path: p})
	}
	e.Attach(files)
}

// paste attaches the files or image on the clipboard; it reports whether
// there was one, else the editor pastes text.
func (e *NoteEditor) paste() bool {
	if e.Clipboard == nil || e.Uploads == nil || e.State.ReadOnly {
		return false
	}
	if paths := e.Clipboard.Files(); len(paths) > 0 {
		e.AttachPaths(paths)
		return true
	}
	if png := e.Clipboard.PNG(); png != nil {
		if len(png) > markup.MaxAttach {
			e.report(fmt.Errorf("the pasted image is over 10 MB"))
			return true
		}
		e.Attach([]markup.File{{Name: markup.PastedName(e.now(), "png"), Data: png}})
		return true
	}
	return false
}

func (e *NoteEditor) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// dropOptions accept files dropped from another app.
var dropOptions = transfer.DropOptions{Formats: []transfer.Format{transfer.FileList}}

// acceptDrops attaches files dropped on the editor, at its caret.
func (e *NoteEditor) acceptDrops(el ui.Element) {
	if e.Uploads == nil || e.State.ReadOnly {
		return
	}
	if _, over := ui.DataDragOver(el, dropOptions); over {
		el.Opacity(0.8)
	}
	if drop, ok := ui.DropData(el, dropOptions); ok {
		paths, err := drop.Data.Files()
		if err != nil {
			e.report(err)
			return
		}
		e.AttachPaths(paths)
	}
}
