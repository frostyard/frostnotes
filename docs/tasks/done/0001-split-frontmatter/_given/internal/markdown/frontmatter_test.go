package markdown

// CONTRACT TEST for task card T-0001 (docs/tasks). Do not edit.

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		has        bool
		front      string
		bodyStart  int
	}{
		{"no frontmatter", "# Title\ntext\n", false, "", 0},
		{"empty note", "", false, "", 0},
		{"simple", "---\ntitle: A\n---\nbody\n", true, "title: A\n", 17},
		{"empty frontmatter", "---\n---\nbody\n", true, "", 8},
		{"dots close it", "---\na: 1\n...\nbody\n", true, "a: 1\n", 13},
		{"closing line ends the file", "---\na: 1\n---", true, "a: 1\n", 12},
		{"nothing after the closing line", "---\na: 1\n---\n", true, "a: 1\n", 13},
		{"CRLF", "---\r\na: 1\r\n---\r\nbody\r\n", true, "a: 1\r\n", 16},
		{"never closed", "---\na: 1\nbody\n", false, "", 0},
		{"not at the first byte", "\n---\na: 1\n---\n", false, "", 0},
		{"a rule later is body", "text\n---\nmore\n", false, "", 0},
		{"four dashes do not open", "----\na: 1\n----\n", false, "", 0},
		{"trailing text on the opener", "--- x\na: 1\n---\n", false, "", 0},
		{"a dashed line inside a value is not a closer", "---\na: |\n  ---x\n---\nb\n", true, "a: |\n  ---x\n", 20},
		{"indented dashes do not close", "---\na: 1\n ---\n---\nb\n", true, "a: 1\n ---\n", 18},
		{"byte order mark", "\ufeff---\na: 1\n---\nb\n", true, "a: 1\n", 16},
		{"only an opener", "---\n", false, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := SplitFrontmatter([]byte(tc.note))
			if s.HasFront != tc.has || string(s.Front) != tc.front || s.BodyStart != tc.bodyStart {
				t.Fatalf("SplitFrontmatter(%q) = {HasFront %v, Front %q, BodyStart %d}; want {%v, %q, %d}",
					tc.note, s.HasFront, s.Front, s.BodyStart, tc.has, tc.front, tc.bodyStart)
			}
			if string(s.Body) != tc.note[tc.bodyStart:] {
				t.Fatalf("Body = %q, want %q", s.Body, tc.note[tc.bodyStart:])
			}
		})
	}
}

// Splitting never copies: Front and Body alias the note, so writing one
// part back cannot change the other's bytes.
func TestSplitFrontmatterAliasesTheNote(t *testing.T) {
	note := []byte("---\na: 1\n---\nbody\n")
	s := SplitFrontmatter(note)
	if &s.Front[0] != &note[4] || &s.Body[0] != &note[13] {
		t.Fatal("Front and Body must be subslices of the note")
	}
}
