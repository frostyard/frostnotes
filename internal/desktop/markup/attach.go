package markup

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

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

// InsertEmbeds is the edit that writes text at the last caret, the caret
// after it; at the start of a line that holds text, the text gets a line
// of its own.
func InsertEmbeds(doc string, sels []Sel, text string) Edit {
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
}
