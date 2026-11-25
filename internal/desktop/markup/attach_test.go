package markup

import (
	"testing"
	"time"
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
