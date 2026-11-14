package store

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// sync.status's counts and refusals, read after every sync cycle without
// loading the rows they come from.

// push records p in vault and, unless status is "", answers it with status
// and reason.
func (dt *deviceTest) push(tx *Tx, vault int64, p LocalPush, status api.PushStatus, reason api.RefuseReason) LocalPush {
	dt.t.Helper()
	p.VaultID = vault
	stored, err := tx.AddPush(dt.t.Context(), p)
	if err != nil {
		dt.t.Fatal(err)
	}
	if status != "" {
		if err := tx.AnswerPush(dt.t.Context(), vault, stored.PushID, status, reason); err != nil {
			dt.t.Fatal(err)
		}
	}
	return stored
}

func (dt *deviceTest) conflictVersion(tx *Tx, f File, hash string) {
	dt.t.Helper()
	if _, err := tx.AddVersion(dt.t.Context(), Version{VaultID: dt.vault, FileID: f.ID, Path: f.Path, Hash: hash, Size: 1,
		Source: api.VersionSourceEditor, Conflict: true}); err != nil {
		dt.t.Fatal(err)
	}
}

func TestSyncCountsAreTheListsLengths(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	if c, err := dt.d.SyncCounts(ctx, dt.vault); err != nil || c != (SyncCounts{}) {
		t.Fatalf("SyncCounts with nothing = %+v, %v; want zeros", c, err)
	}
	dt.tx(func(tx *Tx) {
		same := dt.file(tx, "same.md", "h-same")
		edited := dt.file(tx, "edited.md", "h-new")
		dt.file(tx, "created.md", "h-created")
		refused := dt.file(tx, "refused.md", "h-refused")
		missing := dt.file(tx, "missing.md", "h-missing")
		flagged := dt.file(tx, "flagged.md", "h-flagged")
		dt.agree(tx, Agreed{HubFile: 1, FileID: same.ID, Seq: 1, Revision: 1, Path: "same.md", Hash: "h-same"})
		dt.agree(tx, Agreed{HubFile: 2, FileID: edited.ID, Seq: 2, Revision: 2, Path: "edited.md", Hash: "h-old"})
		dt.agree(tx, Agreed{HubFile: 3, FileID: 999, Seq: 3, Revision: 3, Path: "gone.md", Hash: "h-gone"})
		dt.agree(tx, Agreed{HubFile: 4, FileID: flagged.ID, Seq: 4, Revision: 4, Path: "flagged.md", Hash: "h-flagged",
			Conflicted: true})
		// Refused in this state: not a change until the file changes.
		dt.push(tx, dt.vault, LocalPush{FileID: refused.ID, Path: "refused.md", Hash: "h-refused"},
			api.PushStatusRefused, api.RefuseReasonCollision)
		// Refused for its missing content: still a change.
		dt.push(tx, dt.vault, LocalPush{FileID: missing.ID, Path: "missing.md", Hash: "h-missing"},
			api.PushStatusRefused, api.RefuseReasonMissing)
		// Two lost versions, one carried by an unanswered push, one by an
		// answered one, and one not carried yet.
		dt.conflictVersion(tx, same, "lost")
		dt.conflictVersion(tx, same, "sent")
		dt.conflictVersion(tx, same, "answered")
		dt.conflictVersion(tx, flagged, "lost-too")
		dt.push(tx, dt.vault, LocalPush{FileID: same.ID, HubFile: 1, Base: 1, Path: "same.md", Hash: "sent", Conflict: true}, "", "")
		dt.push(tx, dt.vault, LocalPush{FileID: same.ID, HubFile: 1, Base: 1, Path: "same.md", Hash: "answered",
			Conflict: true}, api.PushStatusAccepted, "")
		// An unanswered push of a change is no conflict push.
		dt.push(tx, dt.vault, LocalPush{FileID: edited.ID, HubFile: 2, Base: 2, Path: "edited.md", Hash: "h-new"}, "", "")
	})
	got, err := dt.d.SyncCounts(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := dt.d.Pending(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	lost, err := dt.d.PendingConflicts(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	unanswered, err := dt.d.Unanswered(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := dt.d.AgreedRows(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	want := SyncCounts{Changes: int64(len(changes)), Lost: int64(len(lost))}
	for _, p := range unanswered {
		if p.Conflict {
			want.UnansweredConflicts++
		}
	}
	for _, a := range rows {
		if a.Conflicted {
			want.Conflicted++
		}
	}
	if got != want {
		t.Fatalf("SyncCounts = %+v; want the lists' lengths %+v", got, want)
	}
	// edited, created, missing and gone; lost and lost-too; sent; flagged.
	if want != (SyncCounts{Changes: 4, Lost: 2, UnansweredConflicts: 1, Conflicted: 1}) || got.Pending() != 7 {
		t.Fatalf("SyncCounts = %+v (pending %d); want 4 changes, 2 lost, 1 unanswered conflict push, 1 conflicted, 7 pending",
			got, got.Pending())
	}
}

func TestRefusalsStandUntilTheFileChanges(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	if r, err := dt.d.Refusals(ctx, dt.vault); err != nil || r == nil || len(r) != 0 {
		t.Fatalf("Refusals with nothing = %#v, %v; want a non-nil empty slice", r, err)
	}
	refuse := func(tx *Tx, vault int64, f File, path, hash string) LocalPush {
		return dt.push(tx, vault, LocalPush{FileID: f.ID, Path: path, Hash: hash}, api.PushStatusRefused, api.RefuseReasonCollision)
	}
	var stands, deleted, answeredLater LocalPush
	dt.tx(func(tx *Tx) {
		other, err := tx.InsertVault(ctx, "Other", "/v/other")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.AddRemote(ctx, Remote{VaultID: other.ID, Hub: dt.hub.ID, SyncID: "s2", Name: "Other", ReplicaID: 10}); err != nil {
			t.Fatal(err)
		}
		// Refused, and the file is still as it was: listed.
		b := dt.file(tx, "b.md", "h-b")
		stands = refuse(tx, dt.vault, b, "b.md", "h-b")
		// Refused, then sent again and not answered yet: the newest answer
		// is still the refusal.
		a := dt.file(tx, "a.md", "h-a")
		answeredLater = refuse(tx, dt.vault, a, "a.md", "h-a")
		dt.push(tx, dt.vault, LocalPush{FileID: a.ID, Path: "a.md", Hash: "h-a", Conflict: true}, "", "")
		// A refused delete of a file that is gone: listed.
		deleted = dt.push(tx, dt.vault, LocalPush{FileID: 998, HubFile: 3, Base: 1, Path: "gone.md"},
			api.PushStatusRefused, api.RefuseReasonEdited)
		// Refused, then the file's content changed: not listed.
		edited := dt.file(tx, "edited.md", "h-1")
		refuse(tx, dt.vault, edited, "edited.md", "h-0")
		// Refused, then the file moved: not listed.
		moved := dt.file(tx, "z/moved.md", "h-m")
		refuse(tx, dt.vault, moved, "moved.md", "h-m")
		// Refused, then a newer push of the file was accepted: not listed.
		later := dt.file(tx, "later.md", "h-l")
		refuse(tx, dt.vault, later, "later.md", "h-l")
		dt.push(tx, dt.vault, LocalPush{FileID: later.ID, Path: "later.md", Hash: "h-l"}, api.PushStatusAccepted, "")
		// A refused delete of a file that is still there: not listed.
		kept := dt.file(tx, "kept.md", "h-k")
		dt.push(tx, dt.vault, LocalPush{FileID: kept.ID, HubFile: 4, Base: 1, Path: "kept.md"},
			api.PushStatusRefused, api.RefuseReasonEdited)
		// Accepted only: not listed.
		fine := dt.file(tx, "fine.md", "h-f")
		dt.push(tx, dt.vault, LocalPush{FileID: fine.ID, Path: "fine.md", Hash: "h-f"}, api.PushStatusAccepted, "")
		// Another vault's refusal: not this vault's.
		elsewhere, err := tx.PutFile(ctx, File{VaultID: other.ID, Path: "b.md", Size: 1, ModTime: time.Unix(1, 0), Hash: "h-o"})
		if err != nil {
			t.Fatal(err)
		}
		refuse(tx, other.ID, elsewhere, "b.md", "h-o")
	})
	got, err := dt.d.Refusals(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	want := []LocalPush{answeredLater, stands, deleted} // by path: a.md, b.md, gone.md
	for i, reason := range []api.RefuseReason{api.RefuseReasonCollision, api.RefuseReasonCollision, api.RefuseReasonEdited} {
		want[i].Status, want[i].Reason = api.PushStatusRefused, reason
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Refusals =\n%+v\nwant the refusals that still stand, by path:\n%+v", got, want)
	}
}

// The refusals are read through the partial index of refused pushes, not
// by a scan of every push the device made.
func TestRefusalsReadOnlyRefusedPushes(t *testing.T) {
	dt := newDeviceTest(t)
	rows, err := dt.d.db.QueryContext(t.Context(), `EXPLAIN QUERY PLAN `+refusalsQuery, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 || !strings.Contains(plan[0], "sync_pushes_refused") {
		t.Fatalf("Refusals' plan =\n%s\nwant its first step to use the index sync_pushes_refused", strings.Join(plan, "\n"))
	}
}
