package vault

// CONTRACT TEST for task card T-0006 (docs/tasks). Do not edit.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeVault writes files (path → content) under a new temporary vault.
func makeVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, content := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// summary renders a listing as one line per entry, so a failure shows the
// whole difference.
func summary(l Listing) string {
	var b strings.Builder
	for _, e := range l.Files {
		fmt.Fprintf(&b, "%s %d %d\n", e.Path, e.Kind, e.Size)
	}
	for _, s := range l.Skipped {
		fmt.Fprintf(&b, "skip %s: %s\n", s.Path, s.Reason)
	}
	return b.String()
}

var sample = map[string]string{
	"Inbox.md":                               "inbox\n",
	"Plan.md":                                "plan",
	"a-b.md":                                 "",
	"a/b.md":                                 "b",
	"Orchard/North Block/Plan.md":            "north plan\n",
	"Orchard/North Block/media/soil-map.png": "\x89PNG",
	"Orchard/Café ☕.md":                      "café",
	"Archive/2025/old.md":                    "old",
	"README":                                 "readme",
	"Notes.MD":                               "upper",
	".frostnotes/vault.yaml":                 "daily: {}\n",
	".frostnotes/templates/daily.yaml":       "x: 1\n",
	".frostnotes/.vault.yaml.tmp":            "partial",
	".obsidian/app.json":                     "{}",
	".git/HEAD":                              "ref",
	".trash/deleted.md":                      "gone",
	".DS_Store":                              "junk",
	"Orchard/.hidden/secret.md":              "hidden",
	"Orchard/.draft.md":                      "dot file",
	"Orchard/.frostnotes/vault.yaml":         "not at the root",
}

func TestWalk(t *testing.T) {
	root := makeVault(t, sample)
	symlink(t, "Plan.md", filepath.Join(root, "link-to-plan.md"))
	symlink(t, "Orchard", filepath.Join(root, "linked-dir"))
	symlink(t, t.TempDir(), filepath.Join(root, "Orchard", "outside"))
	l, err := Walk(t.Context(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `.frostnotes/templates/daily.yaml 3 5
.frostnotes/vault.yaml 3 10
Archive/2025/old.md 1 3
Inbox.md 1 6
Notes.MD 2 5
Orchard/Café ☕.md 1 5
Orchard/North Block/Plan.md 1 11
Orchard/North Block/media/soil-map.png 2 4
Plan.md 1 4
README 2 6
a-b.md 1 0
a/b.md 1 1
skip Orchard/outside: symlink
skip link-to-plan.md: symlink
skip linked-dir: symlink
`
	if got := summary(l); got != want {
		t.Fatalf("Walk =\n%s\nwant\n%s", got, want)
	}
	fi, err := os.Stat(filepath.Join(root, "Inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range l.Files {
		if e.Path == "Inbox.md" && !e.ModTime.Equal(fi.ModTime()) {
			t.Fatalf("ModTime = %v, want %v", e.ModTime, fi.ModTime())
		}
	}
}

func TestWalkEmptyVault(t *testing.T) {
	l, err := Walk(t.Context(), t.TempDir(), nil)
	if err != nil || l.Files == nil || l.Skipped == nil || len(l.Files)+len(l.Skipped) != 0 {
		t.Fatalf("Walk of an empty vault = %#v, %v; want empty, non-nil slices", l, err)
	}
}

func TestWalkIgnore(t *testing.T) {
	root := makeVault(t, sample)
	ig, err := NewIgnore([]string{"Archive/", "*.png", "/Plan.md", "*.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	l, err := Walk(t.Context(), root, ig)
	if err != nil {
		t.Fatal(err)
	}
	// Patterns never hide settings.
	want := `.frostnotes/templates/daily.yaml 3 5
.frostnotes/vault.yaml 3 10
Inbox.md 1 6
Notes.MD 2 5
Orchard/Café ☕.md 1 5
Orchard/North Block/Plan.md 1 11
README 2 6
a-b.md 1 0
a/b.md 1 1
`
	if got := summary(l); got != want {
		t.Fatalf("Walk with ignore =\n%s\nwant\n%s", got, want)
	}
}

func TestWalkSettingsMustBeAFolder(t *testing.T) {
	root := makeVault(t, map[string]string{".frostnotes": "a file, not a folder", "Inbox.md": "x"})
	l, err := Walk(t.Context(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := summary(l); got != "Inbox.md 1 1\n" {
		t.Fatalf("Walk = %q", got)
	}
}

func TestWalkErrors(t *testing.T) {
	root := makeVault(t, map[string]string{"Inbox.md": "x"})
	if _, err := Walk(t.Context(), filepath.Join(root, "missing"), nil); err == nil {
		t.Error("Walk of a missing root succeeded")
	}
	if _, err := Walk(t.Context(), filepath.Join(root, "Inbox.md"), nil); err == nil {
		t.Error("Walk of a file succeeded")
	}
	link := filepath.Join(t.TempDir(), "vault-link")
	symlink(t, root, link)
	if _, err := Walk(t.Context(), link, nil); err == nil {
		t.Error("Walk of a symlinked root succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Walk(ctx, root, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Walk with a canceled context = %v, want context.Canceled", err)
	}
}

// A folder Walk cannot read must fail the walk: a scan that took a partial
// listing for the whole vault would think the folder's notes were deleted.
func TestWalkFailsOnAnUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any folder")
	}
	root := makeVault(t, map[string]string{"Inbox.md": "x", "Locked/note.md": "y"})
	locked := filepath.Join(root, "Locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if l, err := Walk(t.Context(), root, nil); err == nil {
		t.Fatalf("Walk = %q, want an error", summary(l))
	}
}

func TestMatchIgnore(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		dir, want     bool
	}{
		{"Archive/", "Archive", true, true},
		{"Archive/", "Archive", false, false},
		{"Archive/", "Archive/old.md", false, true},
		{"Archive/", "Sub/Archive/x.md", false, true},
		{"Archive/", "Archived/x.md", false, false},
		{"archive/", "Archive/x.md", false, false},
		{"*.tmp", "a.tmp", false, true},
		{"*.tmp", "dir/b.tmp", false, true},
		{"*.tmp", "a.tmpx", false, false},
		{"/Plan.md", "Plan.md", false, true},
		{"/Plan.md", "Sub/Plan.md", false, false},
		{"Projects/*/draft.md", "Projects/A/draft.md", false, true},
		{"Projects/*/draft.md", "Projects/A/B/draft.md", false, false},
		{"Projects/*/draft.md", "X/Projects/A/draft.md", false, false},
		{"Projects/**/draft.md", "Projects/draft.md", false, true},
		{"Projects/**/draft.md", "Projects/A/B/draft.md", false, true},
		{"**/cache", "cache", false, true},
		{"**/cache", "a/b/cache/x.md", false, true},
		{"Daily/2026-*", "Daily/2026-10-16.md", false, true},
		{"Daily/2026-*", "Daily/2025-01-01.md", false, false},
		{"?.md", "a.md", false, true},
		{"?.md", "ab.md", false, false},
		{"[ab].md", "b.md", false, true},
		{"[ab].md", "c.md", false, false},
		{"Big Plan.md", "Orchard/Big Plan.md", false, true},
	} {
		ig, err := NewIgnore([]string{tc.pattern})
		if err != nil {
			t.Fatalf("NewIgnore(%q): %v", tc.pattern, err)
		}
		if got := ig.Match(tc.path, tc.dir); got != tc.want {
			t.Errorf("pattern %q: Match(%q, dir=%v) = %v, want %v", tc.pattern, tc.path, tc.dir, got, tc.want)
		}
	}
	var none *Ignore
	if none.Match("anything.md", false) {
		t.Error("a nil *Ignore matched")
	}
	blank, err := NewIgnore([]string{"", "   "})
	if err != nil || blank.Match("x.md", false) {
		t.Errorf("blank patterns: %v, or they matched", err)
	}
}

func TestNewIgnoreErrors(t *testing.T) {
	for _, p := range []string{"!keep.md", "[unclosed", "a//b", "/", `Daily/x\`} {
		if _, err := NewIgnore([]string{"ok.md", p}); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", p)) {
			t.Errorf("NewIgnore(%q) = %v, want an error quoting the pattern", p, err)
		}
	}
}
