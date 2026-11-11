package catalog

import (
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
)

func start(t *testing.T, files map[string]string) (*Catalog, *desktoptest.Notesd, *desktoptest.Loop) {
	t.Helper()
	n := desktoptest.StartNotesd(t)
	for p, c := range files {
		n.Create(p, c)
	}
	loop := desktoptest.NewLoop()
	c := New(loop, func() API {
		if a := n.AppNotes(); a != nil {
			return a
		}
		return nil
	}, n.Vault)
	c.Reload()
	loop.Until(t, "the files", func() bool { return len(c.Files()) == len(files) })
	return c, n, loop
}

func TestLinkText(t *testing.T) {
	c := &Catalog{LinkFormat: "shortest", files: []api.VaultFile{{Path: "A/Plan.md"}, {Path: "B/Plan.md"}, {Path: "B/Unique.md"}, {Path: "map.png"}}}
	cases := []struct{ format, p, from, want string }{
		{"shortest", "B/Unique.md", "A/x.md", "Unique"},
		{"shortest", "A/Plan.md", "B/x.md", "A/Plan"},
		{"shortest", "map.png", "A/x.md", "map.png"},
		{"absolute", "B/Unique.md", "A/x.md", "B/Unique"},
		{"relative", "B/Unique.md", "A/x.md", "../B/Unique"},
		{"relative", "A/Plan.md", "A/x.md", "Plan"},
	}
	for _, tc := range cases {
		c.LinkFormat = tc.format
		if got := c.LinkText(tc.p, tc.from); got != tc.want {
			t.Errorf("%s %s from %s: %q, want %q", tc.format, tc.p, tc.from, got, tc.want)
		}
	}
}

func TestTheCatalogFollowsTheVault(t *testing.T) {
	c, n, loop := start(t, map[string]string{"Plan.md": "---\naliases: [Roadmap]\ntags: [work]\n---\n# Goals\n\ntext ^blk\n", "Other.md": "x"})
	loop.Until(t, "the aliases", func() bool { return len(c.Aliases("Plan.md")) == 1 })
	src := c.Sources(func() string { return "Other.md" }, func(string) (string, bool) { return "", false })

	targets := src.LinkTargets("road", 10)
	if len(targets) != 1 || targets[0].Insert != "Plan|Roadmap" || targets[0].Label != "Roadmap" {
		t.Fatalf("%+v", targets)
	}
	loop.Until(t, "the tags", func() bool { return len(src.Tags()) == 1 })

	var got []editor.Target
	src.Subpaths("Plan", false, func(ts []editor.Target) { got = ts })
	loop.Until(t, "the headings", func() bool { return got != nil })
	if got[0].Label != "Goals" {
		t.Errorf("%+v", got)
	}
	got = nil
	src.Subpaths("Plan", true, func(ts []editor.Target) { got = ts })
	loop.Until(t, "the blocks", func() bool { return got != nil })
	if got[0].Label != "blk" || got[0].Detail != "text ^blk" {
		t.Errorf("%+v", got)
	}

	hash := n.Create("New.md", "new")
	c.Event(api.NoteChanged{VaultID: n.Vault, Path: "New.md", Hash: hash})
	c.Event(api.NoteChanged{VaultID: n.Vault, Path: "Another.md", Hash: hash}) // a burst reads once
	loop.Advance(ReloadDelay)
	loop.Until(t, "the new file", func() bool { return len(c.Files()) == 3 })
	updated := n.Write("Other.md", n.Read("Other.md").Hash, "changed")
	c.Event(api.NoteChanged{VaultID: n.Vault, Path: "Other.md", Hash: updated})
	for _, f := range c.Files() {
		if f.Path == "Other.md" && f.Hash != updated {
			t.Errorf("hash %s", f.Hash)
		}
	}
}

func TestUploadingAnAttachment(t *testing.T) {
	c, n, loop := start(t, map[string]string{"Notes/Plan.md": "x", "Other/a.png": "taken"})
	c.AttachmentFolder = "./media"
	src := c.Sources(func() string { return "Notes/Plan.md" }, func(string) (string, bool) { return "", false })
	var embeds string
	var err error
	done := false
	src.Upload([]editor.File{{Name: "a.png", Data: []byte("image")}, {Name: "a.png", Data: []byte("second")}}, func(e string, er error) { embeds, err, done = e, er, true })
	loop.Until(t, "the upload", func() bool { return done })
	if err != nil {
		t.Fatal(err)
	}
	// The name is taken elsewhere, so the first embed names its path; the
	// second file gets a free name.
	if embeds != "![[Notes/media/a.png]]\n![[a 1.png]]" {
		t.Fatalf("embeds %q", embeds)
	}
	files, err := n.Other.Note().Files(t.Context(), &api.NoteFilesParams{VaultID: n.Vault})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, f := range files.Files {
		if f.Path == "Notes/media/a.png" || f.Path == "Notes/media/a 1.png" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("%d uploaded", found)
	}
	big := make([]byte, editor.MaxAttach+1)
	done = false
	src.Upload([]editor.File{{Name: "big.bin", Data: big}}, func(_ string, er error) { err, done = er, true })
	loop.Until(t, "the refusal", func() bool { return done })
	if err == nil {
		t.Fatal("a file over 10 MB uploaded")
	}
}

// Run with the race detector: the setting is read on the UI thread.
func TestUploadReadsTheSettingBeforeItsWorker(t *testing.T) {
	c, _, loop := start(t, map[string]string{"Plan.md": "x"})
	src := c.Sources(func() string { return "Plan.md" }, func(string) (string, bool) { return "", false })
	var embeds string
	done := false
	src.Upload([]editor.File{{Name: "a.png", Data: []byte("x")}}, func(e string, _ error) { embeds, done = e, true })
	c.Wikilinks = false
	loop.Until(t, "the upload", func() bool { return done })
	if !strings.HasPrefix(embeds, "![[") {
		t.Fatalf("embeds %q", embeds)
	}
}
