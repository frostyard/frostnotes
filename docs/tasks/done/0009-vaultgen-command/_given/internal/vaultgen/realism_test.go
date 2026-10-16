package vaultgen

// CONTRACT TEST for task card T-0009 (docs/tasks). Do not edit.

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// realism measures what the user's vault looks like at scale (plan 0003):
// 666 notes in 97 folders, about 3.3 KB a note, prose that rarely repeats,
// dates in frontmatter written unquoted.
type realism struct {
	folders           int     // distinct folders holding a file
	meanBytes, maxLen int     // note sizes
	attachments       int     // files that are not notes
	proseLines        float64 // per topic note
	repeated          int     // topic notes holding the most common prose line
	topic             int     // topic notes
	created, quoted   int     // created: lines, and those not an unquoted YYYY-MM-DD
}

var unquotedDate = regexp.MustCompile(`^created: \d{4}-\d{2}-\d{2}$`)

// topicNote is a note other than a daily note, a member's meeting, the
// Inbox, an Overview or a Plan.
func topicNote(p string) bool {
	base := path.Base(p)
	return strings.Contains(p, "/") && !strings.HasPrefix(p, "Members/") && base != "Overview.md" && base != "Plan.md"
}

// proseLine is a line of running text: 40 bytes or more, starting with a
// letter, not a heading, list item, quote, link or code.
func proseLine(line string) bool {
	return len(line) >= 40 && strings.IndexFunc(line[:1], func(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }) == 0
}

func measure(m Manifest, files map[string]string) realism {
	var r realism
	dirs := map[string]bool{}
	for _, p := range append(slices.Clone(m.Notes), m.Attachments...) {
		if d := path.Dir(p); d != "." {
			dirs[d] = true
		}
	}
	r.folders, r.attachments = len(dirs), len(m.Attachments)
	total := 0
	count := map[string]int{}
	lines := 0
	for _, p := range m.Notes {
		note := files[p]
		total += len(note)
		r.maxLen = max(r.maxLen, len(note))
		if front, _, ok := strings.Cut(strings.TrimPrefix(note, "---\n"), "\n---\n"); ok && strings.HasPrefix(note, "---\n") {
			for line := range strings.Lines(front) {
				if line = strings.TrimSuffix(line, "\n"); strings.HasPrefix(line, "created:") {
					r.created++
					if !unquotedDate.MatchString(line) {
						r.quoted++
					}
				}
			}
		}
		if !topicNote(p) {
			continue
		}
		r.topic++
		seen := map[string]bool{}
		for line := range strings.Lines(note) {
			line = strings.TrimSuffix(line, "\n")
			if proseLine(line) {
				lines++
				if !seen[line] {
					seen[line] = true
					count[line]++
				}
			}
		}
	}
	r.meanBytes = total / len(m.Notes)
	for _, n := range count {
		r.repeated = max(r.repeated, n)
	}
	r.proseLines = float64(lines) / float64(r.topic)
	return r
}

func TestVaultIsRealistic(t *testing.T) {
	_, m, files := generate(t, 11, 666)
	r := measure(m, files)
	if r.folders < 80 || r.folders > 120 {
		t.Errorf("%d folders, want 80 to 120 (the user's vault has 97 for 666 notes)", r.folders)
	}
	if r.meanBytes < 2500 || r.meanBytes > 4500 {
		t.Errorf("notes average %d bytes, want 2,500 to 4,500 (the user's: about 3,300)", r.meanBytes)
	}
	if r.maxLen < 8000 || r.maxLen > 65536 {
		t.Errorf("the largest note has %d bytes, want 8,000 to 65,536", r.maxLen)
	}
	if r.attachments < 6 {
		t.Errorf("%d attachments, want at least 6", r.attachments)
	}
	if r.proseLines < 4 {
		t.Errorf("topic notes have %.1f prose lines each, want at least 4", r.proseLines)
	}
	if r.repeated*50 > r.topic {
		t.Errorf("a prose line appears in %d of %d topic notes, want at most 2%%", r.repeated, r.topic)
	}
	if r.created < 500 || r.quoted > 0 {
		t.Errorf("%d created: lines, %d of them not an unquoted date like created: 2026-10-16; want 500 or more, none quoted", r.created, r.quoted)
	}
}

func TestVaultScales(t *testing.T) {
	for _, notes := range []int{20, 100, 3000} {
		_, m, files := generate(t, 5, notes)
		r := measure(m, files)
		if lo, hi := max(5, notes/10), max(25, notes/5); r.folders < lo || r.folders > hi {
			t.Errorf("%d notes: %d folders, want %d to %d", notes, r.folders, lo, hi)
		}
		if notes >= 100 && (r.meanBytes < 2000 || r.meanBytes > 5000) {
			t.Errorf("%d notes: %d bytes on average, want 2,000 to 5,000", notes, r.meanBytes)
		}
		if want := 2 + notes/150; r.attachments < want {
			t.Errorf("%d notes: %d attachments, want at least %d: they grow with the topic folders", notes, r.attachments, want)
		}
		if len(m.Notes) != notes {
			t.Errorf("%d notes asked, %d written", notes, len(m.Notes))
		}
	}
}
