package vault

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// share is a file system that answers to more names than its folders
// list, as an SMB share can: it upper-cases names to compare them (so ı
// is I), trims a trailing dot or space, and knows 8.3 short names.
type share map[string][]string // folder -> the names it lists

func (s share) resolve(p string) (string, bool) {
	dir := "."
	for seg := range strings.SplitSeq(p, "/") {
		key := strings.ToUpper(strings.TrimRight(seg, ". "))
		found := false
		for _, n := range s[dir] {
			if strings.ToUpper(n) == key || shortName(n) == key {
				dir, found = path.Join(dir, n), true
				break
			}
		}
		if !found {
			return "", false
		}
	}
	return dir, true
}

// shortName is an 8.3 alias of a long name, as Windows makes them.
func shortName(n string) string {
	n = strings.ReplaceAll(n, " ", "")
	if len(n) <= 8 {
		return ""
	}
	return strings.ToUpper(n[:6]) + "~1"
}

func (s share) lstat(p string) error {
	if _, ok := s.resolve(p); !ok {
		return fs.ErrNotExist
	}
	return nil
}

func (s share) list(dir string) ([]string, error) {
	real, ok := s.resolve(dir)
	if dir == "." {
		real, ok = ".", true
	}
	if !ok {
		return nil, fs.ErrNotExist
	}
	return s[real], nil
}

// A path reaches existing files only by the names their folders list, up
// to case and normalization; a share's other names for them are refused.
func TestCheckSpelling(t *testing.T) {
	s := share{
		".":                    {"Porter", "Plan.md", "Résumés"},
		"Porter":               {"Memory.md", "Routines", "Long Routines"},
		"Porter/Routines":      {"Brief.md"},
		"Porter/Long Routines": {"Weekly.md"},
		"Résumés":              {"Me.md"},
	}
	for p, ok := range map[string]bool{
		"Plan.md":                        true,
		"Porter/Memory.md":               true,
		"Porter/Routines/Brief.md":       true,
		"porter/routines/brief.md":       true, // a fold: the scope judges it
		"PORTER/ROUTINES/New.md":         true,
		"Porter/Routines/Sub/New.md":     true, // new below an existing folder
		"New/Folder/x.md":                true,
		"Résumés/Me.md":                true,
		"Porter/Long Routines/Weekly.md": true,
		"Porter/LONGRO~1/Weekly.md":      false,
		"Porter/LONGRO~1/New.md":         false,
		"Porter/Routınes/New.md":         false, // dotless ı, which the share upper-cases to I
		"Porter/Routines./New.md":        false,
		"Porter/Routines /New.md":        false,
		"Porter./Routines/New.md":        false,
		"Plan.md.":                       false,
	} {
		err := checkSpelling(p, s.lstat, s.list)
		if (err == nil) != ok {
			t.Errorf("checkSpelling(%q) = %v, want ok %t", p, err, ok)
		}
	}
}
