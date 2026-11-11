package editor

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// MaxAttach is the most bytes notesd takes in one note.upload.
const MaxAttach = 10 << 20

// File is a file to attach: its name, and its bytes or the local path to
// read them from.
type File struct {
	Name string
	Data []byte
	Path string
}

// Read is the file's bytes, at most MaxAttach of them. It may read the
// disk, so an upload calls it off the UI thread.
func (f File) Read() ([]byte, error) {
	data := f.Data
	if f.Path != "" {
		info, err := os.Stat(f.Path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s is not a regular file", f.Name)
		}
		in, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = in.Close() }()
		if data, err = io.ReadAll(io.LimitReader(in, MaxAttach+1)); err != nil {
			return nil, err
		}
	}
	if len(data) > MaxAttach {
		return nil, fmt.Errorf("%s is over 10 MB: copy it into the vault's folder instead", f.Name)
	}
	return data, nil
}

// Uploader stores attachments beside a note (ADR-0023) and reports the
// embeds to insert, through done on the UI thread.
type Uploader interface {
	Upload(files []File, done func(embeds string, err error))
}

// Clipboard is what the editor asks of the system's clipboard when text is
// pasted; the window gives it MyGo's.
type Clipboard interface {
	// Files are the local paths on the clipboard.
	Files() []string
	// PNG is an image on the clipboard, nil for none.
	PNG() []byte
}

// AttachmentFolder is where a note's attachments go, by the setting as
// Obsidian writes it: "/" the vault root, "./" the note's folder, "./NAME"
// a folder NAME beside the note, else a folder's path. "" is the root.
func AttachmentFolder(setting, notePath string) string {
	noteFolder := ""
	if i := strings.LastIndexByte(notePath, '/'); i >= 0 {
		noteFolder = notePath[:i]
	}
	switch {
	case setting == "" || setting == "/":
		return ""
	case setting == "./" || setting == ".":
		return noteFolder
	case strings.HasPrefix(setting, "./"):
		name := strings.TrimRight(setting[2:], "/")
		if noteFolder == "" {
			return name
		}
		if name == "" {
			return noteFolder
		}
		return noteFolder + "/" + name
	}
	return strings.Trim(setting, "/")
}

// FreePath is folder/name, or name with " 1", " 2"… before its extension,
// the first no file has.
func FreePath(folder, name string, taken func(string) bool) string {
	stem, ext := name, ""
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		stem, ext = name[:dot], name[dot:]
	}
	at := func(n string) string {
		if folder == "" {
			return n
		}
		return folder + "/" + n
	}
	if !taken(at(name)) {
		return at(name)
	}
	for i := 1; ; i++ {
		if c := at(fmt.Sprintf("%s %d%s", stem, i, ext)); !taken(c) {
			return c
		}
	}
}

// PastedName is Obsidian's name for a pasted image.
func PastedName(at time.Time, ext string) string {
	return "Pasted image " + at.Format("20060102150405") + "." + ext
}

// EmbedFor is the embed a note gets for an attachment: ![[name]] with
// wikilinks, the path when another file has its name; else a Markdown
// image with the path, encoded.
func EmbedFor(p string, wikilinks, nameIsUnique bool) string {
	if wikilinks && !strings.ContainsAny(p, "#|[]^") { // those end a wikilink's target
		if nameIsUnique {
			return "![[" + path.Base(p) + "]]"
		}
		return "![[" + p + "]]"
	}
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return "![](" + strings.Join(parts, "/") + ")"
}

// CleanName is the name a file is uploaded under.
func CleanName(name string) string {
	return strings.TrimSpace(strings.NewReplacer("\\", "-", "/", "-", ":", "-").Replace(name))
}

// insertEmbeds writes text at the caret, the caret after it; at the start
// of a line that holds text, the text gets a line of its own.
func (e *NoteEditor) insertEmbeds(text string) {
	e.Run(func(doc string, sels []Sel) Edit {
		pos := min(sels[len(sels)-1].Head, len(doc)) // the text may be shorter by now
		l := lineAt(doc, pos)
		own := ""
		if pos == l.from && l.to > l.from {
			own = "\n"
		}
		cs, err := notes.ChangesOf(len(doc), notes.Change{From: pos, To: pos, Insert: text + own})
		if err != nil {
			return Edit{Changes: notes.EmptyChanges(len(doc)), Sels: sels}
		}
		return Edit{Changes: cs, Sels: []Sel{caret(pos + len(text))}}
	})
}

func (e *NoteEditor) report(err error) {
	if e.OnError != nil {
		e.OnError(err)
	}
}

// Attach uploads files beside the note and inserts their embeds at the
// caret.
func (e *NoteEditor) Attach(files []File) {
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
	files := make([]File, 0, len(paths))
	for _, p := range paths {
		files = append(files, File{Name: CleanName(filepath.Base(p)), Path: p})
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
		if len(png) > MaxAttach {
			e.report(fmt.Errorf("the pasted image is over 10 MB"))
			return true
		}
		e.Attach([]File{{Name: PastedName(e.now(), "png"), Data: png}})
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
