package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAttachmentNaming(t *testing.T) {
	cases := []struct{ setting, note, want string }{
		{"", "A/n.md", ""}, {"/", "A/n.md", ""}, {"./", "A/n.md", "A"}, {".", "n.md", ""},
		{"./media/", "A/n.md", "A/media"}, {"./media", "n.md", "media"}, {"/Files/", "A/n.md", "Files"},
	}
	for _, c := range cases {
		if got := AttachmentFolder(c.setting, c.note); got != c.want {
			t.Errorf("%q %q: %q, want %q", c.setting, c.note, got, c.want)
		}
	}
	taken := func(p string) bool { return p == "F/a.png" || p == "F/a 1.png" }
	if got := FreePath("F", "a.png", taken); got != "F/a 2.png" {
		t.Errorf("%q", got)
	}
	if got := FreePath("", "b.png", taken); got != "b.png" {
		t.Errorf("%q", got)
	}
	at := time.Date(2026, 11, 10, 9, 5, 3, 0, time.UTC)
	if got := PastedName(at, "png"); got != "Pasted image 20261110090503.png" {
		t.Errorf("%q", got)
	}
	if EmbedFor("F/a b.png", true, true) != "![[a b.png]]" || EmbedFor("F/a.png", true, false) != "![[F/a.png]]" || EmbedFor("F/a b.png", false, true) != "![](F/a%20b.png)" {
		t.Error("embeds")
	}
}

type fakeUploads struct{ got []File }

func (u *fakeUploads) Upload(files []File, done func(string, error)) {
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
