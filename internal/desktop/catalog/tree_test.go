package catalog

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func TestNaturalOrder(t *testing.T) {
	names := []string{"note 10", "Note 2", "note 1", "alpha", "note"}
	slices.SortFunc(names, compareNatural)
	if !slices.Equal(names, []string{"alpha", "note", "note 1", "Note 2", "note 10"}) {
		t.Errorf("%q", names)
	}
}

func TestBuildTree(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []api.VaultFile{
		{Path: "b.md", ModifiedAt: at}, {Path: "a.md", ModifiedAt: at.Add(time.Hour)}, {Path: "d/x.md", ModifiedAt: at},
		{Path: "c.bin", ModifiedAt: at}, {Path: "p.png", ModifiedAt: at},
	}
	tr := BuildTree(files, []string{"d", "empty"}, SortAlphabetical, false)
	if !slices.Equal(tr.Roots, []string{"d", "empty", "a.md", "b.md", "p.png"}) {
		t.Errorf("%q", tr.Roots)
	}
	tr = BuildTree(files, []string{"d"}, SortModified, true)
	if !slices.Equal(tr.Roots, []string{"d", "a.md", "b.md", "c.bin", "p.png"}) {
		t.Errorf("modified: %q", tr.Roots)
	}
	tr = BuildTree(files, nil, SortAlphabeticalReverse, false)
	if !slices.Equal(tr.Roots, []string{"d", "p.png", "b.md", "a.md"}) {
		t.Errorf("reversed: %q", tr.Roots)
	}
	if n := tr.Nodes["p.png"]; n.Name != "p" || n.Extension != "png" || n.Type != "image" {
		t.Errorf("%+v", n)
	}
}

func TestTagTree(t *testing.T) {
	roots := TagTree([]api.TagCount{{Tag: "garden/fruit", Count: 2}, {Tag: "Garden", Count: 1}, {Tag: "zoo", Count: 9}}, false)
	if len(roots) != 2 || roots[0].Tag != "garden" || roots[0].Count != 3 || roots[0].Children[0].Tag != "garden/fruit" {
		t.Fatalf("%+v", roots[0])
	}
	if byCount := TagTree([]api.TagCount{{Tag: "a", Count: 1}, {Tag: "b", Count: 5}}, true); byCount[0].Tag != "b" {
		t.Error("by count")
	}
}
