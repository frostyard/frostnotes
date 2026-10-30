package store

import (
	"errors"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// DropAgreed drops a file's agreed row at once, as DeleteAgreed does, and
// first carries the file's conflict versions that no push has carried as
// unanswered conflict pushes from the row, with its base hash; unanswered
// conflict pushes of the file take the row as their base.
func TestDropAgreedCarriesConflictVersions(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	var kept, plain File
	conflict := func(tx *Tx, f File, hash string) {
		t.Helper()
		if _, err := tx.AddVersion(ctx, Version{VaultID: dt.vault, FileID: f.ID, Path: f.Path, Hash: hash, Size: 1,
			Source: api.VersionSourceEditor, Conflict: true}); err != nil {
			t.Fatal(err)
		}
	}
	var sent, answered LocalPush
	dt.tx(func(tx *Tx) {
		kept, plain = dt.file(tx, "a.md", "h1"), dt.file(tx, "b.md", "h2")
		dt.agree(tx, Agreed{HubFile: 7, FileID: kept.ID, Seq: 5, Revision: 5, Path: "a.md", Hash: "h1"})
		dt.agree(tx, Agreed{HubFile: 8, FileID: plain.ID, Seq: 3, Revision: 3, Path: "b.md", Hash: "h2"})
		conflict(tx, kept, "lost")
		conflict(tx, kept, "lost") // one push for one content
		conflict(tx, kept, "sent")
		conflict(tx, kept, "answered")
		var err error
		if sent, err = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: kept.ID, HubFile: 7, Base: 2, Path: "a.md",
			Hash: "sent", Conflict: true}); err != nil {
			t.Fatal(err)
		}
		if answered, err = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: kept.ID, HubFile: 7, Base: 2, Path: "a.md",
			Hash: "answered", Conflict: true}); err != nil {
			t.Fatal(err)
		}
		if err := tx.AnswerPush(ctx, dt.vault, answered.PushID, api.PushStatusAccepted, ""); err != nil {
			t.Fatal(err)
		}
	})
	for range 2 { // a missing row is not an error, and adds nothing
		dt.tx(func(tx *Tx) {
			for _, hub := range []int64{7, 8} {
				if err := tx.DropAgreed(ctx, dt.vault, hub); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, hub := range []int64{7, 8} {
		if _, err := dt.d.AgreedByHubFile(ctx, dt.vault, hub); !errors.Is(err, ErrNotFound) {
			t.Fatalf("hub file %d's agreed row after DropAgreed: %v; want none", hub, err)
		}
	}
	if lost, err := dt.d.PendingConflicts(ctx, dt.vault); err != nil || len(lost) != 0 {
		t.Fatalf("PendingConflicts = %+v, %v; want none: the pushes carry them", lost, err)
	}
	un, err := dt.d.Unanswered(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	sent.Base, sent.BaseHash = 5, "h1"
	want := []LocalPush{sent, {VaultID: dt.vault, PushID: answered.PushID + 1, FileID: kept.ID, HubFile: 7, Base: 5,
		BaseHash: "h1", Path: "a.md", Hash: "lost", At: testNow, Conflict: true}}
	if len(un) != len(want) || un[0] != want[0] || un[1] != want[1] {
		t.Fatalf("Unanswered = %+v; want %+v", un, want)
	}
}
