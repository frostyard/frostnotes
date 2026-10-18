package vault

import (
	"fmt"
	"strings"
	"testing"
)

func TestBacklinkContext(t *testing.T) {
	note := "# Plan\n  - see [[Quince|the quince]] soon\r\nnext line\n"
	start := strings.Index(note, "[[")
	stop := strings.Index(note, "]]") + 2
	if got := fmt.Sprint(backlinkContext([]byte(note), start, stop)); got != "[{- see  false} {[[Quince|the quince]] true} { soon false}]" {
		t.Errorf("a link mid-line = %s", got)
	}

	// A link alone on its line, and one that runs past its line's end.
	alone := "[[A]]\nrest"
	if got := fmt.Sprint(backlinkContext([]byte(alone), 0, 5)); got != "[{[[A]] true}]" {
		t.Errorf("a link alone = %s", got)
	}
	split := "see [text\nmore](B.md) after"
	if got := fmt.Sprint(backlinkContext([]byte(split), 4, len("see [text\nmore](B.md)"))); got != "[{see  false} {[text true}]" {
		t.Errorf("a link over two lines = %s", got)
	}

	// Long sides are cut 120 characters from the link.
	long := strings.Repeat("é", 200) + "[[L]]" + strings.Repeat("z", 200)
	at := strings.Index(long, "[[")
	parts := backlinkContext([]byte(long), at, at+5)
	if len(parts) != 3 || parts[0].Text != "…"+strings.Repeat("é", 120) || parts[2].Text != strings.Repeat("z", 120)+"…" {
		t.Errorf("long sides = %+v", parts)
	}

	// A span that is not in the data shows nothing.
	for _, span := range [][2]int{{-1, 2}, {3, 3}, {0, 999}} {
		if got := backlinkContext([]byte("short"), span[0], span[1]); len(got) != 0 {
			t.Errorf("span %v = %v, want none", span, got)
		}
	}
}
