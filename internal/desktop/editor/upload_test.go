package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/markup"
)

type fakeUploads struct{ got []markup.File }

func (u *fakeUploads) Upload(files []markup.File, done func(string, error)) {
	u.got = append(u.got, files...)
	done("![["+files[0].Name+"]]", nil)
}

type fakeClipboard struct {
	files []string
	png   []byte
}

func (c fakeClipboard) Files() []string { return c.files }
func (c fakeClipboard) PNG() []byte     { return c.png }

func TestPastingAnImage(t *testing.T) {
	f := openEditor(t, "a.md", "before\n")
	up := &fakeUploads{}
	f.ed.Uploads = up
	f.ed.Clipboard = fakeClipboard{png: []byte("png bytes")}
	f.ed.Now = func() time.Time { return time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC) }
	f.caret(6)
	f.tt.Key(ui.Cmd, ui.KeyV)
	if len(up.got) != 1 || up.got[0].Name != "Pasted image 20261110090000.png" {
		t.Fatalf("%+v", up.got)
	}
	if !strings.Contains(f.text(), "![[Pasted image 20261110090000.png]]") {
		t.Fatalf("the note holds %q", f.text())
	}
	// Plain text is the editor's to paste.
	f.ed.Clipboard = fakeClipboard{}
	if f.ed.paste() {
		t.Error("an empty clipboard took the paste")
	}
}
