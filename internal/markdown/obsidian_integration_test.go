//go:build integration

package markdown

import (
	"encoding/json/v2"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// TestIObsidianLinkDump checks Resolve against Obsidian's own answers for a
// real vault (plan 0003, Phase 2). FROSTNOTES_OBSIDIAN_LINKS names a JSON
// dump of {source, link, dest} records made in Obsidian's developer
// console, and FROSTNOTES_OBSIDIAN_VAULT the vault, which is only walked
// for file names. Neither ever enters the repository; run it with
// make obsidian-links.
func TestIObsidianLinkDump(t *testing.T) {
	dumpPath, root := os.Getenv("FROSTNOTES_OBSIDIAN_LINKS"), os.Getenv("FROSTNOTES_OBSIDIAN_VAULT")
	if dumpPath == "" || root == "" {
		t.Skip("set FROSTNOTES_OBSIDIAN_LINKS and FROSTNOTES_OBSIDIAN_VAULT")
	}
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	var dump []struct {
		Source string  `json:"source"`
		Link   string  `json:"link"`
		Dest   *string `json:"dest"`
	}
	if err := json.Unmarshal(data, &dump); err != nil {
		t.Fatalf("dump: %v", err)
	}
	if len(dump) == 0 {
		t.Fatal("the dump has no links")
	}

	byName := map[string][]string{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			name := strings.ToLower(path.Base(rel))
			byName[name] = append(byName[name], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	files := func(name string) []string { return byName[name] }

	resolved, wrong := 0, 0
	for _, e := range dump {
		target, _, _ := strings.Cut(e.Link, "#")
		want := ""
		if e.Dest != nil {
			want = *e.Dest
			resolved++
		}
		// The dump holds note and file names from the user's vault, so a
		// failure says how it differs, never which link it was.
		got := Resolve(e.Source, target, files)
		if got == want {
			continue
		}
		wrong++
		found := "nothing"
		switch {
		case got != "" && want != "":
			found = "a different file"
		case got != "":
			found = "a file where Obsidian found none"
		}
		t.Errorf("disagreement %d: Resolve found %s (folder path %v, relative %v)", wrong, found,
			strings.Contains(target, "/"), strings.HasPrefix(target, "./") || strings.HasPrefix(target, "../"))
	}
	t.Logf("%d links from Obsidian (%d resolved); %d disagree", len(dump), resolved, wrong)
}
