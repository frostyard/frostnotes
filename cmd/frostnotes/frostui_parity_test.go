package main

import (
	"strings"
	"testing"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/system"
)

// system and catalog carry their own copies of frostui's mailto rule and
// fuzzy match, so that they import no UI toolkit. While this client has
// frostui, these targets hold each copy to frostui's results: on their
// seeds in every unit run, and on whatever `go test -fuzz` finds.

// mailtoSeeds are mailto links and other links made of the parts the rule
// reads: the scheme's case, an address or none, kept and dropped fields
// (encoded, without "=", Unicode-folded), "?" with nothing after it,
// fragments, spaces and control characters.
func mailtoSeeds() []string {
	heads := []string{"mailto:", "MAILTO:", "Mailto:a@e.test", "mailto:a@e.test", "https://e.test/", "file:///etc/passwd", "a@e.test", ""}
	fields := []string{
		"", "to=b@e.test", "TO=c@e.test", "cc=", "bcc=d@e.test", "Subject=Hi%20there", "body=a+b", "body=100%",
		"attach=/etc/passwd", "ATTACH=y", "Attachment=x", "%61ttach=x", "%74o=b@e.test", "ſubject=x", "subject",
		"%=y", "file=/etc/passwd", "body=x%26attach%3D/etc/passwd", "subject=a b", "body=x\nattach=y",
	}
	tails := []string{"", "#", "#&attach=/etc/passwd", "#attach=x", "#%zz", "\x7f", "\t"}
	var out []string
	for _, h := range heads {
		for _, t := range tails {
			out = append(out, h+t, h+"?"+t)
			for i, a := range fields {
				out = append(out, h+"?"+a+t)
				for _, b := range fields[i:] {
					out = append(out, h+"?"+a+"&"+b+t, h+"?"+b+"&"+a+"&"+t)
				}
			}
		}
	}
	return out
}

func FuzzSanitizeMailtoIsFrostuis(f *testing.F) {
	for _, s := range mailtoSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		want, wantErr := frostui.SanitizeMailto(raw)
		got, err := system.SanitizeMailto(raw)
		if got != want || (err == nil) != (wantErr == nil) || err != nil && err.Error() != wantErr.Error() {
			t.Fatalf("SanitizeMailto(%q): system gives %q, %v; frostui gives %q, %v", raw, got, err, want, wantErr)
		}
	})
}

// scoreTexts are file names and aliases as the quick switcher and [[ rank
// them, and frostui's own cases.
var scoreTexts = []string{
	"", "a b", "ab", "x ab", "axb", "xab", "zz", "Update everything", "Stage the system update",
	"Projects/Plan", "Plan", "garden/Note 10", "garden/Note 2", "Daily/2026-11-25", "Ćafé notes", "ĆAFÉ",
	"garden/fruit/apple tree in the far orchard of the old farm", "x-y_z/1.2 (draft)", "İstanbul", "ǅemal",
	"Notes/日本語のノート", "  padded  ",
}

var scoreQueries = []string{
	"", " ", "a", "ab", "AB", "upev", "upd", "xyz", "plan", " plan ", "pp", "note 1", "note 10", "2026",
	"ćaf", "cafe", "gf", "tree", "x y", "1.2", "i̇st", "ǆ", "日本", "/", "  padded",
}

func FuzzScoreIsFrostuisMatch(f *testing.F) {
	for _, q := range scoreQueries {
		for _, text := range scoreTexts {
			f.Add(q, text)
		}
	}
	f.Fuzz(func(t *testing.T, query, text string) {
		want, wantOK := frostui.Match(query, text)
		got, ok := catalog.Score(query, text)
		if got != want || ok != wantOK {
			t.Fatalf("Score(%q, %q) = %d, %v; frostui.Match gives %d, %v", query, text, got, ok, want, wantOK)
		}
	})
}

func TestTheSeedsReachEveryRule(t *testing.T) {
	kept, refused := 0, 0
	for _, s := range mailtoSeeds() {
		if got, err := system.SanitizeMailto(s); err != nil {
			refused++
		} else if strings.Contains(got, "?") {
			kept++
		}
	}
	if kept == 0 || refused == 0 {
		t.Fatalf("the seeds keep a field %d times and are refused %d times", kept, refused)
	}
}
