package vaultgen

// CONTRACT TEST for task card T-0008 (docs/tasks). Do not edit.

import (
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/markdown"
	"go.yaml.in/yaml/v3"
)

// readVault returns every file under dir by slash-separated vault path.
func readVault(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func generate(t *testing.T, seed uint64, notes int) (string, Manifest, map[string]string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	m, err := Generate(dir, Options{Seed: seed, Notes: notes})
	if err != nil {
		t.Fatal(err)
	}
	return dir, m, readVault(t, dir)
}

func TestGenerateIsDeterministic(t *testing.T) {
	_, m1, f1 := generate(t, 7, 120)
	_, m2, f2 := generate(t, 7, 120)
	if !maps.Equal(f1, f2) {
		t.Fatal("two vaults from one seed differ")
	}
	if !reflect.DeepEqual(m1, m2) {
		t.Fatal("two manifests from one seed differ")
	}
	_, _, f3 := generate(t, 8, 120)
	if maps.Equal(f1, f3) {
		t.Fatal("seeds 7 and 8 gave the same vault")
	}
}

func TestGenerateRefuses(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "real note.md")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir, Options{Seed: 1, Notes: 50}); err == nil {
		t.Fatal("Generate wrote into a directory that was not empty")
	}
	if got := readVault(t, dir); len(got) != 1 || got["real note.md"] != "mine" {
		t.Fatalf("Generate changed a directory it refused: %v", slices.Sorted(maps.Keys(got)))
	}
	if _, err := Generate(filepath.Join(t.TempDir(), "v"), Options{Seed: 1, Notes: 19}); err == nil {
		t.Fatal("Generate accepted 19 notes")
	}
	empty := t.TempDir()
	if _, err := Generate(empty, Options{Seed: 1, Notes: 20}); err != nil {
		t.Fatalf("Generate into an existing empty directory: %v", err)
	}
}

// lookup answers markdown.Resolve's files(name) over the manifest.
func lookup(m Manifest) func(string) []string {
	return func(name string) []string {
		var out []string
		for _, p := range append(slices.Clone(m.Notes), m.Attachments...) {
			if strings.ToLower(path.Base(p)) == name {
				out = append(out, p)
			}
		}
		return out
	}
}

// wikiTarget is what a parser reports as a wikilink's Target.
func wikiTarget(text string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "!"), "[["), "]]")
	inner, _, _ = strings.Cut(inner, "|")
	inner, _, _ = strings.Cut(inner, "#")
	return strings.TrimSpace(inner)
}

var markdownLink = regexp.MustCompile(`^!?\[[^\]]*\]\(([^)\s]+)\)$`)

// inCode reports whether offset start of note lies inside a fenced code
// block or an inline code span on its line.
func inCode(note string, start int) bool {
	fences := 0
	for _, line := range strings.Split(note[:start], "\n") {
		if strings.HasPrefix(line, "```") {
			fences++
		}
	}
	lineStart := strings.LastIndexByte(note[:start], '\n') + 1
	return fences%2 == 1 || strings.Count(note[lineStart:start], "`")%2 == 1
}

func TestManifestMatchesTheVault(t *testing.T) {
	for _, seed := range []uint64{1, 2} {
		_, m, files := generate(t, seed, 200)
		var notes, attachments []string
		for p := range files {
			if strings.HasSuffix(p, ".md") {
				notes = append(notes, p)
			} else {
				attachments = append(attachments, p)
			}
		}
		slices.Sort(notes)
		slices.Sort(attachments)
		if m.Seed != seed || len(notes) != 200 || !slices.Equal(m.Notes, notes) || !slices.Equal(m.Attachments, attachments) {
			t.Fatalf("seed %d: manifest lists %d notes and %d attachments; the vault has %d and %d",
				seed, len(m.Notes), len(m.Attachments), len(notes), len(attachments))
		}
		if len(m.Links) < 200 || len(m.Decoys) < 2 {
			t.Fatalf("seed %d: %d links and %d decoys; want at least 200 and 2", seed, len(m.Links), len(m.Decoys))
		}
		if !slices.IsSortedFunc(m.Links, func(a, b Link) int {
			if c := strings.Compare(a.Source, b.Source); c != 0 {
				return c
			}
			return a.Start - b.Start
		}) {
			t.Fatalf("seed %d: links are not in (Source, Start) order", seed)
		}
		for _, l := range append(slices.Clone(m.Links), m.Decoys...) {
			note, ok := files[l.Source]
			if !ok || l.Start < 0 || l.Start+len(l.Text) > len(note) || note[l.Start:l.Start+len(l.Text)] != l.Text {
				t.Fatalf("seed %d: %+v is not at its offset in %s", seed, l, l.Source)
			}
		}
		for _, l := range m.Decoys {
			if !inCode(files[l.Source], l.Start) || l.Kind != "wiki" || wikiTarget(l.Text) != l.Target || l.Want != "" {
				t.Fatalf("seed %d: decoy %+v is not a wikilink inside code", seed, l)
			}
		}
		for _, l := range m.Links {
			if inCode(files[l.Source], l.Start) {
				t.Fatalf("seed %d: link %+v is inside code", seed, l)
			}
			switch l.Kind {
			case "wiki":
				if wikiTarget(l.Text) != l.Target {
					t.Fatalf("seed %d: %+v: a parser reads target %q", seed, l, wikiTarget(l.Text))
				}
			case "markdown":
				sub := markdownLink.FindStringSubmatch(l.Text)
				if sub == nil {
					t.Fatalf("seed %d: %+v is not a markdown link", seed, l)
				}
				if dest, err := url.PathUnescape(sub[1]); err != nil || dest != l.Target {
					t.Fatalf("seed %d: %+v: its destination decodes to %q, %v", seed, l, dest, err)
				}
			default:
				t.Fatalf("seed %d: %+v has kind %q", seed, l, l.Kind)
			}
			if l.Want != "" && !slices.Contains(m.Notes, l.Want) && !slices.Contains(m.Attachments, l.Want) {
				t.Fatalf("seed %d: %+v wants a file that does not exist", seed, l)
			}
			if got := markdown.Resolve(l.Source, l.Target, lookup(m)); got != l.Want {
				t.Fatalf("seed %d: %+v resolves to %q", seed, l, got)
			}
		}
	}
}

var datedNote = regexp.MustCompile(`^(?:.*/)?(\d{4}-\d{2}-\d{2})\.md$`)

func TestVaultPatterns(t *testing.T) {
	_, m, files := generate(t, 1, 200)

	// Daily notes: the vault root, one per day ending 2026-10-16, all from
	// one template.
	var daily []string
	skeletons := map[string]bool{}
	for _, p := range m.Notes {
		if sub := datedNote.FindStringSubmatch(p); sub != nil && !strings.Contains(p, "/") {
			daily = append(daily, p)
			head, _, ok := strings.Cut(files[p], "## Log")
			if !ok {
				t.Fatalf("daily note %s has no \"## Log\"", p)
			}
			skeletons[strings.ReplaceAll(head, sub[1], "DATE")] = true
		}
	}
	if len(daily) < 20 || !slices.Contains(daily, "2026-10-16.md") || len(skeletons) != 1 {
		t.Fatalf("%d daily notes (want at least 20, ending 2026-10-16.md) from %d templates (want 1)", len(daily), len(skeletons))
	}

	// Names: a date in at least three member folders, another name in at
	// least three folders, names with spaces and non-ASCII letters, and
	// notes three folders deep. Root names are unique.
	names := map[string][]string{}
	deep, spaced, accented := false, false, false
	for _, p := range m.Notes {
		name := strings.ToLower(path.Base(p))
		names[name] = append(names[name], p)
		deep = deep || strings.Count(p, "/") >= 3
		spaced = spaced || strings.Contains(path.Base(p), " ")
		accented = accented || strings.ContainsFunc(p, func(r rune) bool { return r > 0x7f })
	}
	sharedDate, sharedName := false, false
	for name, paths := range names {
		if len(paths) >= 3 && datedNote.MatchString(name) {
			sharedDate = sharedDate || strings.HasPrefix(paths[0], "Members/")
		} else if len(paths) >= 3 {
			sharedName = true
		}
		for _, p := range paths {
			if !strings.Contains(p, "/") && len(paths) > 1 {
				t.Fatalf("root note %s shares its name with %v", p, paths)
			}
		}
	}
	if !sharedDate || !sharedName || !deep || !spaced || !accented {
		t.Fatalf("shared date %v, shared name %v, three deep %v, spaces %v, non-ASCII %v; want all",
			sharedDate, sharedName, deep, spaced, accented)
	}

	// Bytes: some notes end without a newline; frontmatter parses.
	withFront, final, noFinal := 0, 0, 0
	for _, p := range m.Notes {
		note := files[p]
		if strings.HasSuffix(note, "\n") {
			final++
		} else {
			noFinal++
		}
		if !strings.HasPrefix(note, "---\n") {
			continue
		}
		withFront++
		front, _, ok := strings.Cut(note[4:], "\n---\n")
		var props map[string]any
		if err := yaml.Unmarshal([]byte(front), &props); !ok || err != nil || props == nil {
			t.Fatalf("%s: frontmatter does not parse as a mapping: %v", p, err)
		}
	}
	if final == 0 || noFinal == 0 || withFront < 140 || withFront > 196 {
		t.Fatalf("%d notes end with a newline, %d do not, %d of 200 have frontmatter (want 140 to 196)", final, noFinal, withFront)
	}
	callout := false
	for _, note := range files {
		callout = callout || strings.Contains(note, "\n> [!important] ")
	}
	if !callout {
		t.Fatal("no callout")
	}

	// Every form of link and decoy appears.
	forms := map[string]bool{}
	for _, l := range m.Links {
		forms["bare"] = forms["bare"] || l.Kind == "wiki" && !strings.Contains(l.Target, "/") && l.Want != "" && strings.HasSuffix(l.Want, ".md")
		forms["same-folder duplicate"] = forms["same-folder duplicate"] || l.Kind == "wiki" && !strings.Contains(l.Target, "/") &&
			len(names[strings.ToLower(path.Base(l.Want))]) > 1 && path.Dir(l.Want) == path.Dir(l.Source)
		forms["folder path"] = forms["folder path"] || l.Kind == "wiki" && strings.Contains(l.Target, "/")
		forms["heading"] = forms["heading"] || strings.Contains(l.Text, "#")
		forms["alias"] = forms["alias"] || strings.Contains(l.Text, "|")
		forms["unresolved"] = forms["unresolved"] || l.Want == ""
		forms["markdown note"] = forms["markdown note"] || l.Kind == "markdown" && strings.Contains(l.Text, "%20") && strings.HasSuffix(l.Want, ".md")
		forms["relative image"] = forms["relative image"] || l.Kind == "markdown" && strings.HasPrefix(l.Text, "![") && strings.HasPrefix(l.Target, "./")
		forms["wiki embed"] = forms["wiki embed"] || l.Kind == "wiki" && strings.HasPrefix(l.Text, "![[") && !strings.HasSuffix(l.Want, ".md")
	}
	for _, l := range m.Decoys {
		before := files[l.Source][:l.Start]
		forms["fenced decoy"] = forms["fenced decoy"] || strings.HasSuffix(before, "```\n")
		forms["inline decoy"] = forms["inline decoy"] || strings.HasSuffix(before, "`") && !strings.HasSuffix(before, "```\n")
	}
	for _, f := range []string{"bare", "same-folder duplicate", "folder path", "heading", "alias", "unresolved",
		"markdown note", "relative image", "wiki embed", "fenced decoy", "inline decoy"} {
		if !forms[f] {
			t.Errorf("no %s link", f)
		}
	}
}
