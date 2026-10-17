package vault

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/store"
)

// hubVault is a served test vault with one paired device and its replica.
type hubVault struct {
	*testVault
	device, replica int64
	pushes          int64
}

func newHubVault(t *testing.T, files map[string]string) *hubVault {
	t.Helper()
	tv := newTestVault(t, files, nil)
	tv.scan()
	if _, err := tv.v.Serve(t.Context()); err != nil {
		t.Fatal(err)
	}
	hv := &hubVault{testVault: tv}
	if err := tv.db.Tx(t.Context(), func(tx *store.Tx) error {
		d, err := tx.AddDevice(t.Context(), "laptop", "token-hash")
		if err != nil {
			return err
		}
		r, err := tx.AddReplica(t.Context(), d.ID, tv.v.id)
		hv.device, hv.replica = d.ID, r.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return hv
}

// blob stores content in the vault's blob store, as an upload does.
func (tv *testVault) blob(content string) string {
	tv.t.Helper()
	id, err := tv.blobs.Put(tv.t.Context(), strings.NewReader(content))
	if err != nil {
		tv.t.Fatal(err)
	}
	return id
}

// head returns file id's state in the log.
func (hv *hubVault) head(id int64) api.LogFile {
	hv.t.Helper()
	f, err := hv.db.LogFile(hv.t.Context(), hv.v.id, id)
	if err != nil {
		hv.t.Fatal(err)
	}
	return f
}

// push sends one change as the device's next push.
func (hv *hubVault) push(c PushChange) PushOutcome {
	hv.t.Helper()
	hv.pushes++
	who := Author{Source: api.VersionSourceEditor, Name: "app@laptop",
		Push: &Push{DeviceID: hv.device, ReplicaID: hv.replica, PushID: hv.pushes, Base: c.Base}}
	out, err := hv.v.Push(hv.t.Context(), c, who)
	if err != nil {
		hv.t.Fatalf("Push(%+v): %v", c, err)
	}
	return out
}

// change makes a put of file f from its current head.
func (hv *hubVault) change(f api.LogFile, path, content string) PushChange {
	return PushChange{FileID: f.File, Base: f.Revision, BaseHash: f.Hash, Path: path, Hash: hv.blob(content)}
}

func (hv *hubVault) history(id int64) []api.Revision {
	hv.t.Helper()
	revs, err := hv.db.History(hv.t.Context(), hv.v.id, id, 0, 0)
	if err != nil {
		hv.t.Fatal(err)
	}
	return revs
}

func ops(revs []api.Revision) string {
	var out []string
	for i := len(revs) - 1; i >= 0; i-- {
		out = append(out, string(revs[i].Op))
	}
	return strings.Join(out, " ")
}

func TestServeSeedsTheLogAndEveryChangeAppends(t *testing.T) {
	hv := newHubVault(t, map[string]string{"a.md": "one\n", "b/c.png": "png"})
	ctx := t.Context()
	page, err := hv.db.Pull(ctx, hv.v.id, 0, 0)
	if err != nil || len(page.Files) != 2 || page.Files[0].Path != "a.md" || page.Files[1].Path != "b/c.png" {
		t.Fatalf("the seeded log = %+v, %v; want a create per file, by path", page, err)
	}
	a := hv.row("a.md")
	res, err := hv.v.Write(ctx, "a.md", a.Hash, []byte("two\n"), editor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hv.v.Move(ctx, "a.md", "d/a.md", false, editor); err != nil {
		t.Fatal(err)
	}
	hv.edit("d/a.md", "three\n")
	hv.scan()
	// Two writers on one base: the second's conflicting hunk is a conflict revision.
	if _, err := hv.v.Write(ctx, "d/a.md", res.Hash, []byte("four\n"), agent); err != nil {
		t.Fatal(err)
	}
	if err := hv.v.Delete(ctx, "b/c.png", hv.row("b/c.png").Hash, editor); err != nil {
		t.Fatal(err)
	}
	revs := hv.history(a.ID)
	if got := ops(revs); got != "create put move put conflict" {
		t.Fatalf("a.md's log = %s, want create put move put conflict", got)
	}
	if revs[1].Source != api.VersionSourceOutside || revs[0].Author != "an agent" || revs[0].LostTo == nil ||
		*revs[0].LostTo != revs[1].Seq || revs[1].Base != revs[2].Seq {
		t.Fatalf("revisions = %+v", revs)
	}
	if !hv.head(a.ID).Conflicted {
		t.Fatal("a conflict revision did not flag the file")
	}
	if got := ops(hv.history(hv.history(a.ID)[0].File + 1)); got != "create delete" {
		t.Fatalf("c.png's log = %s, want create delete", got)
	}
}

func TestAVaultThatIsNotServedAppendsNothing(t *testing.T) {
	tv := newTestVault(t, map[string]string{"a.md": "one\n"}, nil)
	tv.scan()
	if _, err := tv.v.Write(t.Context(), "a.md", tv.row("a.md").Hash, []byte("two\n"), editor); err != nil {
		t.Fatal(err)
	}
	if _, err := tv.db.HubFile(t.Context(), tv.v.id, tv.row("a.md").ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("an unserved vault's write reached the log: %v", err)
	}
}

func TestPushCreatesAndMergesCreates(t *testing.T) {
	hv := newHubVault(t, map[string]string{"2026-10-16.md": "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\n\n"})
	out := hv.push(PushChange{Path: "new/Plan.md", Hash: hv.blob("plan\n")})
	if out.Status != api.PushStatusAccepted || out.FileID == 0 || hv.disk("new/Plan.md") != "plan\n" {
		t.Fatalf("a create at a free path = %+v", out)
	}
	revs := hv.history(out.FileID)
	if len(revs) != 1 || revs[0].Op != api.RevisionOpCreate || revs[0].Device == nil || *revs[0].Device != hv.device ||
		revs[0].Author != "app@laptop" {
		t.Fatalf("its revision = %+v; want a create by the device", revs)
	}
	recs, _ := hv.db.PushRecords(t.Context(), hv.v.id, hv.replica, []int64{1})
	if len(recs[0].Revisions) != 1 {
		t.Fatalf("push 1's record = %+v", recs)
	}
	// Two daily notes made offline from one template merge.
	daily := hv.row("2026-10-16.md")
	out = hv.push(PushChange{Path: "2026-10-16.md", Hash: hv.blob("# 2026-10-16\n## Tasks\n## Notes\nmet Ana at lunch\n\n")})
	if out.Status != api.PushStatusMerged || out.FileID != daily.ID || !out.BasePruned {
		t.Fatalf("a create on an existing path = %+v; want merged into file %d, as creates", out, daily.ID)
	}
	if got := hv.disk("2026-10-16.md"); got != "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\nmet Ana at lunch\n\n" {
		t.Fatalf("merged daily note = %q", got)
	}
	if out := hv.push(PushChange{Path: "NEW/plan.md", Hash: hv.blob("x\n")}); out.Status != api.PushStatusRefused ||
		out.Reason != api.RefuseReasonCollision {
		t.Fatalf("a create differing only by case = %+v, want a collision", out)
	}
}

func TestPushMergesAndConflicts(t *testing.T) {
	hv := newHubVault(t, map[string]string{"a.md": "a\nb\nc\nd\ne\n"})
	id := hv.row("a.md").ID
	base := hv.head(id)
	if out := hv.push(hv.change(base, "a.md", "A\nb\nc\nd\ne\n")); out.Status != api.PushStatusAccepted {
		t.Fatalf("a put on the head = %+v", out)
	}
	// Another device, behind, changed another line.
	if out := hv.push(hv.change(base, "a.md", "a\nb\nc\nd\nE\n")); out.Status != api.PushStatusMerged || out.Conflicts != 0 {
		t.Fatalf("a put on a stale base = %+v, want merged", out)
	}
	if got := hv.disk("a.md"); got != "A\nb\nc\nd\nE\n" {
		t.Fatalf("merged = %q", got)
	}
	// And a third changed the first line differently: the head's side stands.
	out := hv.push(hv.change(base, "a.md", "X\nb\nc\nd\ne\n"))
	if out.Status != api.PushStatusConflicted || out.Conflicts != 1 || hv.disk("a.md") != "A\nb\nc\nd\nE\n" {
		t.Fatalf("a conflicting put = %+v, file %q", out, hv.disk("a.md"))
	}
	head := hv.head(id)
	revs := hv.history(id)
	if !head.Conflicted || revs[0].Op != api.RevisionOpConflict || revs[0].Hash != blobs.ID([]byte("X\nb\nc\nd\ne\n")) {
		t.Fatalf("after a conflict: head %+v, newest revision %+v", head, revs[0])
	}
	// A push the hub has already applied changes nothing.
	if out := hv.push(PushChange{FileID: id, Base: head.Revision, BaseHash: head.Hash, Path: "a.md", Hash: head.Hash}); out.Status != api.PushStatusAccepted {
		t.Fatalf("a put of the head's own content = %+v", out)
	}
	if len(hv.history(id)) != len(revs) {
		t.Fatal("a put of the head's own content appended a revision")
	}
}

func TestPushDeletesAndRestores(t *testing.T) {
	hv := newHubVault(t, map[string]string{"a.md": "one\n", "dir/b.md": "two\n"})
	a, b := hv.row("a.md").ID, hv.row("dir/b.md").ID
	old := hv.head(a)
	hv.push(hv.change(old, "a.md", "edited\n"))
	stale := PushChange{FileID: a, Base: old.Revision, BaseHash: old.Hash, Path: "a.md", Deleted: true}
	if out := hv.push(stale); out.Status != api.PushStatusRefused || out.Reason != api.RefuseReasonEdited || out.FileID != a ||
		!hv.exists("a.md") {
		t.Fatalf("a delete of an edited file = %+v; want refused, the edit standing", out)
	}
	bh := hv.head(b)
	del := PushChange{FileID: b, Base: bh.Revision, BaseHash: bh.Hash, Path: "dir/b.md", Deleted: true}
	if out := hv.push(del); out.Status != api.PushStatusAccepted || hv.exists("dir/b.md") || hv.exists("dir") {
		t.Fatalf("a delete on the head = %+v; want the file and its emptied folder gone", out)
	}
	if out := hv.push(del); out.Status != api.PushStatusAccepted || len(hv.history(b)) != 2 {
		t.Fatalf("deleting a deleted file again = %+v, %d revisions", out, len(hv.history(b)))
	}
	// An edit made before the delete brings the file back, with its ID.
	hv.edit("dir/b.md", "squatter\n") // another file has taken the path meanwhile
	hv.scan()
	out := hv.push(hv.change(bh, "dir/b.md", "two, edited\n"))
	if out.Status != api.PushStatusAccepted || out.FileID != b || hv.disk("dir/b 1.md") != "two, edited\n" {
		t.Fatalf("a put on a deleted file = %+v; want it back as dir/b 1.md", out)
	}
	if got := ops(hv.history(b)); got != "create delete put" {
		t.Fatalf("b's log = %s", got)
	}
}

func TestPushMoves(t *testing.T) {
	hv := newHubVault(t, map[string]string{"a.md": "one\n", "taken.md": "other\n"})
	a := hv.row("a.md").ID
	base := hv.head(a)
	if out := hv.push(PushChange{FileID: a, Base: base.Revision, BaseHash: base.Hash, Path: "x/a.md", Hash: base.Hash}); out.Status != api.PushStatusAccepted || !hv.exists("x/a.md") {
		t.Fatalf("a move = %+v", out)
	}
	// A device that has not seen the move edits the file at its old path:
	// the edit applies where the file now is.
	if out := hv.push(hv.change(base, "a.md", "one, edited\n")); out.Status != api.PushStatusAccepted || hv.disk("x/a.md") != "one, edited\n" {
		t.Fatalf("an edit at the old path = %+v", out)
	}
	// The latest move wins, and a taken path gets " 1".
	head := hv.head(a)
	if out := hv.push(PushChange{FileID: a, Base: head.Revision, BaseHash: head.Hash, Path: "taken.md", Hash: head.Hash}); out.Status != api.PushStatusAccepted || hv.disk("taken 1.md") != "one, edited\n" {
		t.Fatalf("a move onto a taken path = %+v", out)
	}
	if hv.exists("x") {
		t.Fatal("a move left its emptied folder")
	}
}

func TestPushRefusals(t *testing.T) {
	hv := newHubVault(t, map[string]string{"a.md": "one\n", "b.md": "two\n"})
	a, b := hv.head(hv.row("a.md").ID), hv.head(hv.row("b.md").ID)
	good := hv.blob("content\n")
	for name, tc := range map[string]struct {
		c    PushChange
		want api.RefuseReason
	}{
		"a path out of the vault":      {PushChange{Path: "../x.md", Hash: good}, api.RefuseReasonInvalid},
		"a path in the settings":       {PushChange{Path: ".frostnotes/vault.yaml", Hash: good}, api.RefuseReasonInvalid},
		"a hidden path":                {PushChange{Path: ".obsidian/app.json", Hash: good}, api.RefuseReasonInvalid},
		"a malformed hash":             {PushChange{Path: "c.md", Hash: "abc"}, api.RefuseReasonInvalid},
		"a create that deletes":        {PushChange{Path: "c.md", Deleted: true}, api.RefuseReasonInvalid},
		"content the hub lacks":        {PushChange{Path: "c.md", Hash: blobs.ID([]byte("never uploaded"))}, api.RefuseReasonMissing},
		"a file the vault never had":   {PushChange{FileID: 999, Base: 1, BaseHash: a.Hash, Path: "a.md", Hash: good}, api.RefuseReasonUnknown},
		"a base of another file":       {PushChange{FileID: a.File, Base: b.Revision, BaseHash: b.Hash, Path: "a.md", Hash: good}, api.RefuseReasonBase},
		"a base whose hash is not its": {PushChange{FileID: a.File, Base: a.Revision, BaseHash: b.Hash, Path: "a.md", Hash: good}, api.RefuseReasonBase},
	} {
		if out := hv.push(tc.c); out.Status != api.PushStatusRefused || out.Reason != tc.want {
			t.Errorf("%s: %+v, want refused %s", name, out, tc.want)
		}
	}
	if hv.disk("a.md") != "one\n" || hv.exists("c.md") {
		t.Fatal("a refused push changed the vault")
	}
}

func TestPushOfAttachmentsDoesNotMerge(t *testing.T) {
	hv := newHubVault(t, map[string]string{"img.png": "\x89PNG one"})
	id := hv.row("img.png").ID
	base := hv.head(id)
	hv.push(hv.change(base, "img.png", "\x89PNG two"))
	out := hv.push(hv.change(base, "img.png", "\x89PNG three"))
	if out.Status != api.PushStatusConflicted || hv.disk("img.png") != "\x89PNG three" {
		t.Fatalf("a stale push of an attachment = %+v, file %q; want the later push standing, flagged", out, hv.disk("img.png"))
	}
	revs := hv.history(id)
	if revs[0].Op != api.RevisionOpConflict || revs[0].Hash != blobs.ID([]byte("\x89PNG two")) {
		t.Fatalf("the replaced content = %+v; want it kept as a conflict revision", revs[0])
	}
}

// applyRecord collects the files Apply's record saw.
type applyRecord struct{ files []store.File }

func (a *applyRecord) record(_ context.Context, _ *store.Tx, f store.File) error {
	a.files = append(a.files, f)
	return nil
}

var fromHub = Author{Source: api.VersionSourceSync, Name: "app@laptop"}

func TestApply(t *testing.T) {
	tv := newTestVault(t, map[string]string{"a.md": "one\n", "dir/b.md": "two\n"}, nil)
	tv.scan()
	ctx := t.Context()
	var rec applyRecord
	created, err := tv.v.Apply(ctx, Expect{Path: "new/c.md"}, Head{Path: "new/c.md", Hash: tv.blob("three\n")}, fromHub, rec.record)
	if err != nil || tv.disk("new/c.md") != "three\n" || created.ID == 0 {
		t.Fatalf("Apply of a create = %+v, %v", created, err)
	}
	a := tv.row("a.md")
	if _, err := tv.v.Apply(ctx, Expect{Path: "a.md", Hash: a.Hash}, Head{Path: "x/a.md", Hash: tv.blob("one, edited\n")}, fromHub, rec.record); err != nil {
		t.Fatal(err)
	}
	if tv.exists("a.md") || tv.disk("x/a.md") != "one, edited\n" || tv.row("x/a.md").ID != a.ID {
		t.Fatal("Apply of a move and an edit did not move the file, keeping its ID, and write it")
	}
	b := tv.row("dir/b.md")
	if _, err := tv.v.Apply(ctx, Expect{Path: "dir/b.md", Hash: b.Hash}, Head{Path: "dir/b.md", Deleted: true}, fromHub, rec.record); err != nil {
		t.Fatal(err)
	}
	if tv.exists("dir") {
		t.Fatal("Apply of a delete left its emptied folder")
	}
	if len(rec.files) != 3 || rec.files[0].Path != "new/c.md" || rec.files[1].Path != "x/a.md" || rec.files[2].ID != b.ID {
		t.Fatalf("record saw %+v; want one call per change, in its transaction", rec.files)
	}
	vs := tv.versions(a.ID)
	if vs[0].Source != api.VersionSourceSync || vs[0].Author != "app@laptop" {
		t.Fatalf("an applied version = %+v; want source sync and the revision's author", vs[0])
	}
}

func TestApplyRefusesWhatChanged(t *testing.T) {
	tv := newTestVault(t, map[string]string{"a.md": "one\n", "b.md": "two\n"}, nil)
	tv.scan()
	ctx := t.Context()
	next := tv.blob("next\n")
	a := tv.row("a.md")
	tv.edit("a.md", "typed meanwhile\n") // no scan has seen it
	if _, err := tv.v.Apply(ctx, Expect{Path: "a.md", Hash: a.Hash}, Head{Path: "a.md", Hash: next}, fromHub, nil); !errors.Is(err, ErrChanged) {
		t.Fatalf("Apply over an outside edit = %v, want ErrChanged", err)
	}
	if tv.disk("a.md") != "typed meanwhile\n" {
		t.Fatal("Apply overwrote an outside edit")
	}
	if _, err := tv.v.Apply(ctx, Expect{Path: "b.md"}, Head{Path: "b.md", Hash: next}, fromHub, nil); !errors.Is(err, ErrChanged) {
		t.Fatalf("Apply of a create onto a file = %v, want ErrChanged", err)
	}
	b := tv.row("b.md")
	if _, err := tv.v.Apply(ctx, Expect{Path: "b.md", Hash: b.Hash}, Head{Path: "a.md", Hash: b.Hash}, fromHub, nil); !errors.Is(err, ErrChanged) {
		t.Fatalf("Apply of a move onto a file = %v, want ErrChanged", err)
	}
	// An edit between staging and the rename.
	tv.v.staged = func(p string) { tv.edit(p, "typed while staged\n") }
	b = tv.row("b.md")
	if _, err := tv.v.Apply(ctx, Expect{Path: "b.md", Hash: b.Hash}, Head{Path: "b.md", Hash: next}, fromHub, nil); !errors.Is(err, ErrChanged) {
		t.Fatalf("Apply over an edit made while staged = %v, want ErrChanged", err)
	}
	if tv.disk("b.md") != "typed while staged\n" {
		t.Fatal("Apply overwrote an edit made while it staged")
	}
	for _, p := range []string{"../x.md", ".frostnotes/vault.yaml", "a/../../x.md"} {
		if _, err := tv.v.Apply(ctx, Expect{Path: p}, Head{Path: p, Hash: next}, fromHub, nil); err == nil || errors.Is(err, ErrChanged) {
			t.Errorf("Apply to %s = %v; want the path guard's refusal", p, err)
		}
	}
}

func TestApplyStreamsFilesTooBigForTheAPI(t *testing.T) {
	tv := newTestVault(t, nil, nil)
	ctx := t.Context()
	big := bytes.Repeat([]byte("0123456789abcdef"), (bigFile+1024)/16)
	hash, err := tv.blobs.Put(ctx, bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	f, err := tv.v.Apply(ctx, Expect{Path: "huge.md"}, Head{Path: "huge.md", Hash: hash}, fromHub, nil)
	if err != nil || f.Size != int64(len(big)) || f.Hash != hash {
		t.Fatalf("Apply of a %d-byte note = %+v, %v", len(big), f, err)
	}
	if got := blobs.ID([]byte(tv.disk("huge.md"))); got != hash {
		t.Fatalf("the file on disk hashes to %s, want %s", got, hash)
	}
	if info, err := tv.db.NoteInfo(ctx, f.ID); err != nil || info.Title != "huge" {
		t.Fatalf("a big note's index = %+v, %v; want it indexed by name", info, err)
	}
}
