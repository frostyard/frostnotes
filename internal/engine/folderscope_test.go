package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// porterScope is Porter's scope (ADR-0028) for vault id.
func porterScope(t *testing.T, id int64, rest string) *api.Scope {
	t.Helper()
	sc, err := api.ParseScope(fmt.Sprintf("vault=%d%s", id, rest))
	if err != nil {
		t.Fatal(err)
	}
	return &sc
}

// porterVault is a vault laid out as Porter's: its folder, its routines,
// and notes around them that link in.
func porterVault(t *testing.T) *apiVault {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"Plan.md":                  "plan, see [[Porter/Briefs/B]]\n",
		"Porterhouse.md":           "steak\n",
		"Porter/Memory.md":         "memory, see [[Porter/Drafts/D]]\n",
		"Porter/Briefs/B.md":       "brief\n",
		"Porter/Drafts/D.md":       "draft\n",
		"Porter/Routines/Brief.md": "routine\n",
	})
	return openVault(t, dir)
}

// outside asserts that err is the refusal of a write outside the scope:
// invalidParams, naming what the token may write.
func outside(t *testing.T, what string, err error, may string) {
	t.Helper()
	if code(err) != api.CodeInvalidParams || !strings.Contains(err.Error(), "outside this token's writes") ||
		!strings.Contains(err.Error(), may) {
		t.Errorf("%s = %v, want invalidParams outside the token's writes (%s)", what, err, may)
	}
}

// files lists the vault's files on disk, so a refusal can be shown to
// have changed nothing.
func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameFiles(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	for p, c := range before {
		if after[p] != c {
			t.Errorf("%s changed %s", what, p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s made %s", what, p)
		}
	}
}

// Every method marked writes judges its paths against a folder scope:
// noteParams name paths outside Porter/, so each answers invalidParams
// naming the writable folders. TestAgentScopeRefusesWritesUnderARead
// holds noteWrites to the router's table, so a new writing method that
// skips the check fails here.
func TestAgentFolderScopeJudgesEveryWrite(t *testing.T) {
	av := porterVault(t)
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "porter",
		Scope: porterScope(t, av.id, ";write=Porter/;except=Porter/Routines/")})
	before := files(t, av.dir)
	params := noteParams(av.id)
	for _, m := range noteWrites {
		outside(t, m, agent.Call(t.Context(), m, params[m], nil), "under Porter/ except Porter/Routines/")
	}
	sameFiles(t, "refused writes", before, files(t, av.dir))
}

// Files: writable in Porter/, and in no spelling of Porter/Routines/.
func TestAgentFolderScopeFiles(t *testing.T) {
	av := porterVault(t)
	ctx := t.Context()
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "porter",
		Scope: porterScope(t, av.id, ";write=Porter/;except=Porter/Routines/;daily")})
	may := "this token may write under Porter/ except Porter/Routines/, and append to today's daily note"
	zero := strings.Repeat("0", 64)
	hash := func(p string) string {
		t.Helper()
		n, err := av.c.Note().Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: p})
		if err != nil {
			t.Fatal(err)
		}
		return n.Hash
	}
	before := files(t, av.dir)
	for _, p := range []string{
		"Plan2.md", "Porterhouse2.md", "Porter.md", "Notes/Porter/x.md",
		"Porter/Routines/New.md", "porter/routines/New.md", "PORTER/ROUTINES/New.md", "Porter/routines/New.md",
		"Porter/Routineſ/New.md", "Porter/Routines/Sub/New.md",
	} {
		_, err := agent.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: p, Content: "x\n"})
		outside(t, "create "+p, err, may)
		_, err = agent.Note().Upload(ctx, &api.NoteUploadParams{VaultID: av.id, Path: strings.TrimSuffix(p, ".md") + ".png", Data: "AA=="})
		outside(t, "upload "+p, err, may)
	}
	// The path guard comes first: these are refused as the vault would
	// refuse them, before any prefix is compared.
	for _, p := range []string{"Porter/../Plan.md", "Porter/./x.md", "Porter//x.md", "/Porter/x.md", ".obsidian/app.json", "Porter/.hidden.md", ""} {
		_, err := agent.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: p, Content: "x\n"})
		if code(err) != api.CodeInvalidParams || strings.Contains(err.Error(), "outside this token's writes") {
			t.Errorf("create %q = %v, want the path guard's invalidParams", p, err)
		}
	}
	for _, p := range []string{"Plan.md", "Porter/Routines/Brief.md", "porter/routines/brief.md", "Porterhouse.md"} {
		_, err := agent.Note().Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: p, Base: zero, Content: "x\n"})
		outside(t, "write "+p, err, may)
		outside(t, "delete "+p, agent.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: av.id, Path: p, Base: zero}), may)
		_, err = agent.Note().Revert(ctx, &api.NoteRevertParams{VaultID: av.id, Path: p, Version: 1, Base: zero})
		outside(t, "revert "+p, err, may)
	}
	sameFiles(t, "refused writes", before, files(t, av.dir))

	// Inside the scope, every kind of file.
	if _, err := agent.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: "Porter/Briefs/2026-10-19.md", Content: "brief\n"}); err != nil {
		t.Errorf("create in Porter/Briefs: %v", err)
	}
	if _, err := agent.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: "porter/Reviews/W42.md", Content: "review\n"}); err != nil {
		t.Errorf("create in porter/ (case folded into Porter/): %v", err)
	}
	if _, err := agent.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: "Porter/Routines.md", Content: "beside the routines\n"}); err != nil {
		t.Errorf("create Porter/Routines.md: %v", err)
	}
	if _, err := agent.Note().Upload(ctx, &api.NoteUploadParams{VaultID: av.id, Path: "Porter/chart.png", Data: "AA=="}); err != nil {
		t.Errorf("upload in Porter/: %v", err)
	}
	w, err := agent.Note().Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Porter/Memory.md", Base: hash("Porter/Memory.md"), Content: "more memory\n"})
	if err != nil {
		t.Fatalf("write Porter/Memory.md: %v", err)
	}
	vs, err := agent.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "Porter/Memory.md"})
	if err != nil || len(vs) < 2 {
		t.Fatalf("versions = %+v, %v", vs, err)
	}
	if _, err := agent.Note().Revert(ctx, &api.NoteRevertParams{VaultID: av.id, Path: "Porter/Memory.md", Version: vs[len(vs)-1].ID, Base: w.Hash}); err != nil {
		t.Errorf("revert Porter/Memory.md: %v", err)
	}
	if err := agent.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: av.id, Path: "Porter/Briefs/B.md", Base: hash("Porter/Briefs/B.md")}); err != nil {
		t.Errorf("delete Porter/Briefs/B.md: %v", err)
	}
	if got := files(t, av.dir)["Porter/Routines/Brief.md"]; got != "routine\n" {
		t.Errorf("the routine reads %q", got)
	}
}

// A move needs both ends in the scope, in both directions, and a move
// whose link updates would rewrite a note outside it is refused whole.
func TestAgentFolderScopeMoves(t *testing.T) {
	av := porterVault(t)
	ctx := t.Context()
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "porter",
		Scope: porterScope(t, av.id, ";write=Porter/;except=Porter/Routines/")})
	may := "under Porter/ except Porter/Routines/"
	yes := true
	before := files(t, av.dir)
	for _, mv := range [][2]string{
		{"Porter/Memory.md", "Porter/Routines/Memory.md"}, // into the routines
		{"Porter/Memory.md", "porter/ROUTINES/Memory.md"},
		{"Porter/Routines/Brief.md", "Porter/Brief.md"}, // out of them
		{"porter/routines/Brief.md", "Porter/Brief.md"},
		{"Plan.md", "Porter/Plan.md"},        // in from outside
		{"Porter/Drafts/D.md", "Notes/D.md"}, // out to outside
		{"Porter/Drafts/D.md", "Porterhouse/D.md"},
	} {
		_, err := agent.Note().Move(ctx, &api.NoteMoveParams{VaultID: av.id, Path: mv[0], To: mv[1]})
		outside(t, "move "+mv[0]+" to "+mv[1], err, may)
	}
	// Plan.md links to Porter/Briefs/B: rewriting it is a write outside.
	_, err := agent.Note().Move(ctx, &api.NoteMoveParams{VaultID: av.id, Path: "Porter/Briefs/B.md", To: "Porter/Archive/B.md", UpdateLinks: &yes})
	outside(t, "a move whose link updates reach Plan.md", err, may)
	sameFiles(t, "refused moves", before, files(t, av.dir))

	// Without link updates, the same move is in the scope.
	if _, err := agent.Note().Move(ctx, &api.NoteMoveParams{VaultID: av.id, Path: "Porter/Briefs/B.md", To: "Porter/Archive/B.md"}); err != nil {
		t.Errorf("move within Porter/: %v", err)
	}
	// Only Porter/Memory.md links to Porter/Drafts/D: the update is inside.
	if _, err := agent.Note().Move(ctx, &api.NoteMoveParams{VaultID: av.id, Path: "Porter/Drafts/D.md", To: "Porter/Drafts/E.md", UpdateLinks: &yes}); err != nil {
		t.Errorf("move with link updates inside Porter/: %v", err)
	}
	if got := files(t, av.dir)["Porter/Memory.md"]; got != "memory, see [[E]]\n" {
		t.Errorf("Porter/Memory.md after the move reads %q", got)
	}
}

// A folder is made where the scope may write; it is moved or deleted only
// when it is writable whole, with no except at or under either end.
func TestAgentFolderScopeFolders(t *testing.T) {
	av := porterVault(t)
	ctx := t.Context()
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "porter",
		Scope: porterScope(t, av.id, ";write=Porter/;except=Porter/Routines/")})
	may := "under Porter/ except Porter/Routines/"
	yes := true
	for _, p := range []string{"Other", "Porterhouse", "Porter/Routines", "porter/routines/Sub", "Porter/Routines/Sub"} {
		outside(t, "mkdir "+p, agent.Note().Mkdir(ctx, &api.NoteMkdirParams{VaultID: av.id, Path: p}), may)
	}
	for _, p := range []string{"Porter", "Porter/Archive", "porter/Old"} {
		if err := agent.Note().Mkdir(ctx, &api.NoteMkdirParams{VaultID: av.id, Path: p}); err != nil {
			t.Errorf("mkdir %s: %v", p, err)
		}
	}
	before := files(t, av.dir)
	for _, mv := range [][2]string{
		{"Porter", "Porter2"},                   // holds the routines
		{"Porter/Routines", "Porter/Routines2"}, // is them
		{"PORTER/routines", "Porter/Routines2"},
		{"Porter/Drafts", "Porter/Routines/Drafts"}, // into them
		{"Porter/Drafts", "Notes/Drafts"},           // out of the scope
		{"Porter/Drafts", "Porterhouse"},
	} {
		_, err := agent.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: mv[0], To: mv[1]})
		outside(t, "moveFolder "+mv[0]+" to "+mv[1], err, "the folder")
	}
	_, err := agent.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: "Porter/Briefs", To: "Porter/Old/Briefs", UpdateLinks: &yes})
	outside(t, "a folder move whose link updates reach Plan.md", err, may)
	for _, p := range []string{"Porter", "porter", "Porter/Routines", "Porter/routines", "Other"} {
		_, err := agent.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: p, Files: []api.FileBase{}})
		outside(t, "deleteFolder "+p, err, "the folder")
	}
	sameFiles(t, "refused folder writes", before, files(t, av.dir))

	if _, err := agent.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: "Porter/Drafts", To: "Porter/Old/Drafts", UpdateLinks: &yes}); err != nil {
		t.Errorf("moveFolder within Porter/: %v", err)
	}
	if got := files(t, av.dir)["Porter/Memory.md"]; got != "memory, see [[D]]\n" {
		t.Errorf("Porter/Memory.md after the folder's move reads %q", got)
	}
	ls, err := agent.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	var bases []api.FileBase
	for _, f := range ls.Files {
		if strings.HasPrefix(f.Path, "Porter/Old/Drafts/") {
			bases = append(bases, api.FileBase{Path: f.Path, Base: f.Hash})
		}
	}
	if _, err := agent.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: "Porter/Old/Drafts", Files: bases}); err != nil {
		t.Errorf("deleteFolder within Porter/: %v", err)
	}
}

// daily writes today's daily note, under the daily-note settings, and no
// other day's; except wins over it, and a scope with only daily writes
// nothing else.
func TestAgentDailyScope(t *testing.T) {
	av := porterVault(t)
	ctx := t.Context()
	day := func(d int) *string {
		s := time.Now().AddDate(0, 0, d).Format(time.DateOnly)
		return &s
	}
	porter := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "porter",
		Scope: porterScope(t, av.id, ";write=Porter/;except=Porter/Routines/;daily")})
	only := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "capture", Scope: porterScope(t, av.id, ";daily")})
	noDaily := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "briefs", Scope: porterScope(t, av.id, ";write=Porter/")})

	before := files(t, av.dir)
	for _, d := range []int{-1, -7, 2, 30} {
		for name, c := range map[string]*api.Client{"porter": porter, "daily": only} {
			_, err := c.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id, Date: day(d)})
			if code(err) != api.CodeInvalidParams || !strings.Contains(err.Error(), "today's daily note only") {
				t.Errorf("%s: the daily note %d days from today = %v, want invalidParams for today only", name, d, err)
			}
		}
	}
	_, err := noDaily.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id})
	outside(t, "today's daily note without daily", err, "this token may write under Porter/")
	if err := only.Call(ctx, "note.create", map[string]any{"vaultId": av.id, "path": "Porter/x.md", "content": "x\n"}, nil); code(err) != api.CodeMethodNotFound {
		t.Errorf("note.create under only daily = %v, want methodNotFound", err)
	}
	sameFiles(t, "refused daily notes", before, files(t, av.dir))

	got, err := only.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id})
	if err != nil || !got.Created {
		t.Fatalf("today's daily note under daily = %+v, %v", got, err)
	}
	if again, err := porter.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id, Date: day(0)}); err != nil || again.Note.Path != got.Note.Path {
		t.Errorf("today's daily note again = %+v, %v", again, err)
	}

	// The settings put daily notes in the routines: except wins, for
	// today too, however the settings spell the folder.
	for _, folder := range []string{"Porter/Routines", "porter/routines", "Porter/Routines/Daily"} {
		writeFiles(t, av.dir, map[string]string{".obsidian/daily-notes.json": `{"folder": "` + folder + `"}`})
		before := files(t, av.dir)
		_, err := porter.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id})
		outside(t, "today's daily note in "+folder, err, "except Porter/Routines/")
		sameFiles(t, "daily notes in the routines", before, files(t, av.dir))
	}
	// A scope that excepts the daily folder itself takes daily back.
	writeFiles(t, av.dir, map[string]string{".obsidian/daily-notes.json": `{"folder": "Journal"}`})
	journal := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "j", Scope: porterScope(t, av.id, ";except=Journal/;daily")})
	_, err = journal.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id})
	outside(t, "today's daily note in an excepted folder", err, "this token may append to today's daily note outside Journal/")
	if slices.ContainsFunc(keys(files(t, av.dir)), func(p string) bool { return strings.HasPrefix(p, "Journal/") }) {
		t.Error("a daily note was made in Journal/")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A session, a local caller and a bare write write anywhere: the folder
// rules bind only a scope that has them.
func TestUnscopedWritesStayWhole(t *testing.T) {
	av := porterVault(t)
	ctx := t.Context()
	for name, c := range map[string]*api.Client{
		"local":   av.c,
		"session": av.srv.DialAs(t, api.Identity{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"}),
		"write":   av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude", Scope: porterScope(t, av.id, ";write")}),
	} {
		if _, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: "Porter/Routines/" + name + ".md", Content: "x\n"}); err != nil {
			t.Errorf("%s: create in Porter/Routines: %v", name, err)
		}
		if _, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: name + ".md", Content: "x\n"}); err != nil {
			t.Errorf("%s: create at the root: %v", name, err)
		}
	}
}
