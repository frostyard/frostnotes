package render

import (
	"bytes"
	"strings"
	"testing"
)

// budgetVault is a vault of notes by path for Options: every note exists.
func budgetVault(notes map[string]string) Options {
	return Options{
		Path:    "A.md",
		Resolve: func(_, target string) string { return target + ".md" },
		File: func(p string) (string, bool) {
			_, ok := notes[p]
			return "h", ok
		},
		Note: func(p string) ([]byte, bool) {
			n, ok := notes[p]
			if !ok || len(n) > MaxEmbedBytes {
				return nil, false
			}
			return []byte(n), true
		},
	}
}

// fanOut is n block embeds of target.
func fanOut(target string, n int) string {
	return strings.Repeat("![["+target+"]]\n\n", n)
}

// A few notes that embed each other many times over draw at most
// MaxEmbeds embeds, the rest as links (review finding 2, plan 0005 Phase 7).
func TestRenderBoundsNoteEmbeds(t *testing.T) {
	opts := budgetVault(map[string]string{
		"A.md": fanOut("B", 20), "B.md": fanOut("C", 20), "C.md": fanOut("D", 20), "D.md": "Deep.\n",
	})
	got := HTML([]byte(fanOut("B", 20)), opts)
	if n := strings.Count(got, `<div class="embed"`); n != MaxEmbeds {
		t.Errorf("%d note embeds drawn, want %d", n, MaxEmbeds)
	}
	if !strings.Contains(got, `<p><a class="internal-link" data-href="D" data-path="D.md">D</a></p>`) {
		t.Error("an embed past the budget is not a link")
	}
	if len(got) > 1<<20 {
		t.Errorf("%d bytes of HTML", len(got))
	}
}

func TestRenderBoundsEmbeddedBytes(t *testing.T) {
	big := string(bytes.Repeat([]byte("word "), (1<<20)/5)) + "\n"
	opts := budgetVault(map[string]string{"D.md": big, "Huge.md": big + big + big + big + big})
	got := HTML([]byte(fanOut("D", 10)+"![[Huge]]\n"), opts)
	if n := strings.Count(got, `<div class="embed"`); n != MaxEmbedBytes/len(big) {
		t.Errorf("%d note embeds of 1 MiB drawn, want %d", n, MaxEmbedBytes/len(big))
	}
	if !strings.Contains(got, `<p><a class="internal-link" data-href="Huge" data-path="Huge.md">Huge</a></p>`) {
		t.Error("a note larger than the budget is not a link")
	}
	if inline := HTML([]byte("Inline ![[Huge]].\n"), opts); !strings.Contains(inline, `<a class="internal-link" data-href="Huge" data-path="Huge.md">Huge</a>`) {
		t.Errorf("an inline embed of a large note = %q", inline)
	}
}
