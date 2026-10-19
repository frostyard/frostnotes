package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// dailyVault is a vault whose daily notes go in Daily/ from a template.
func dailyVault(t *testing.T) *apiVault {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".obsidian/daily-notes.json": `{"folder": "Daily", "template": "Templates/Daily"}`,
		"Templates/Daily.md":         "# {{title}}\n",
		"Porter/Memory.md":           "memory\n",
		"Porter/Routines/Brief.md":   "routine\n",
	})
	return openVault(t, dir)
}

func today() string { return time.Now().Format(time.DateOnly) }

// An append goes after what the note holds, which it leaves as it was,
// an edit made on disk meanwhile included; a missing note is made from
// the template first.
func TestAppendDaily(t *testing.T) {
	av := dailyVault(t)
	ctx := t.Context()
	p := "Daily/" + today() + ".md"
	read := func() string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(av.dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	first, err := av.c.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "- first"})
	if err != nil || !first.Created || first.Write.Path != p {
		t.Fatalf("the first append = %+v, %v; want %s created", first, err, p)
	}
	if got, want := read(), "# "+today()+"\n- first\n"; got != want {
		t.Errorf("after the first append the note reads %q, want %q", got, want)
	}
	second, err := av.c.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "- second\n"})
	if err != nil || second.Created || second.Write.Hash == first.Write.Hash {
		t.Fatalf("the second append = %+v, %v", second, err)
	}
	// The user edits the note by hand, leaving no final line break; the
	// next append keeps the edit, and changes none of the text before it.
	edited := "# " + today() + "\n- first, by hand\n- second"
	writeFiles(t, av.dir, map[string]string{p: edited})
	third, err := av.c.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "- third"})
	if err != nil || third.Write.Merged {
		t.Fatalf("the append after an edit = %+v, %v", third, err)
	}
	if got, want := read(), edited+"\n- third\n"; got != want {
		t.Errorf("after the edit and an append the note reads %q, want %q", got, want)
	}
	vs, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: p})
	if err != nil || len(vs) != 5 {
		t.Errorf("versions = %+v, %v; want the creation, three appends and the outside edit", vs, err)
	}
	// Another day's note, by date.
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	if got, err := av.c.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "late", Date: &yesterday}); err != nil ||
		got.Write.Path != "Daily/"+yesterday+".md" || !got.Created {
		t.Errorf("an append to yesterday's note = %+v, %v", got, err)
	}
	for _, bad := range []api.NoteAppendDailyParams{{VaultID: av.id}, {VaultID: av.id, Text: "x", Date: new("19-10-2026")}} {
		if _, err := av.c.Note().AppendDaily(ctx, &bad); code(err) != api.CodeInvalidParams {
			t.Errorf("AppendDaily(%+v) = %v, want invalidParams", bad, err)
		}
	}
}

// Under daily, an agent appends to today's note, as itself, and to no
// other day's; except wins over it, and a write scope without daily
// appends only where it may write.
func TestAgentAppendsToTodayUnderDaily(t *testing.T) {
	av := dailyVault(t)
	ctx := t.Context()
	scope := func(rest string) *api.Scope {
		sc, err := api.ParseScope(fmt.Sprintf("vault=%d%s", av.id, rest))
		if err != nil {
			t.Fatal(err)
		}
		return &sc
	}
	dial := func(token, rest string) *api.Client {
		return av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: token, Scope: scope(rest)})
	}
	porter := dial("porter", ";write=Porter/;except=Porter/Routines/;daily")
	capture := dial("capture", ";daily")
	briefs := dial("briefs", ";write=Porter/")
	journal := dial("journal", ";write=Daily/")

	for name, c := range map[string]*api.Client{"porter": porter, "capture": capture} {
		for _, d := range []int{-1, 2} {
			date := time.Now().AddDate(0, 0, d).Format(time.DateOnly)
			_, err := c.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "x", Date: &date})
			if code(err) != api.CodeInvalidParams || !strings.Contains(err.Error(), "today's daily note only") {
				t.Errorf("%s: an append %d days from today = %v, want today's only", name, d, err)
			}
		}
	}
	_, err := briefs.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "x"})
	outside(t, "an append without daily", err, "this token may write under Porter/")
	if _, err := os.Stat(filepath.Join(av.dir, "Daily")); !os.IsNotExist(err) {
		t.Fatalf("a refused append made Daily/: %v", err)
	}

	got, err := capture.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "- from capture"})
	if err != nil || !got.Created {
		t.Fatalf("an append under daily = %+v, %v", got, err)
	}
	if _, err := porter.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "- from porter", Date: new(today())}); err != nil {
		t.Fatalf("an append under Porter's scope, naming today: %v", err)
	}
	// A write scope covering the daily folder appends to any day's.
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	if _, err := journal.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "late", Date: &yesterday}); err != nil {
		t.Errorf("an append under write=Daily/ to yesterday's note: %v", err)
	}
	vs, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: got.Write.Path})
	if err != nil || len(vs) != 3 {
		t.Fatalf("versions = %+v, %v", vs, err)
	}
	for i, want := range []string{"porter", "capture", "capture"} {
		if vs[i].Source != api.VersionSourceAgent || vs[i].Author != want {
			t.Errorf("version %d by %s/%q, want agent/%s", i, vs[i].Source, vs[i].Author, want)
		}
	}

	// The settings move daily notes into the routines: except wins.
	writeFiles(t, av.dir, map[string]string{".obsidian/daily-notes.json": `{"folder": "porter/routines"}`})
	_, err = porter.Note().AppendDaily(ctx, &api.NoteAppendDailyParams{VaultID: av.id, Text: "x"})
	outside(t, "an append in the routines", err, "except Porter/Routines/")
	entries, err := os.ReadDir(filepath.Join(av.dir, "Porter/Routines"))
	if err != nil || len(entries) != 1 {
		t.Errorf("the routines hold %v, %v; want only Brief.md", entries, err)
	}
}
