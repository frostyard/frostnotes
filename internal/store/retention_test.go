package store

// CONTRACT TEST for task card T-0028 (docs/tasks). Do not edit.

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// retentionNow is the clock the pass runs at.
var retentionNow = time.Date(2027, 6, 15, 12, 0, 0, 0, time.UTC)

// at appends a revision made ago before retentionNow.
func (ht *hubTest) at(ago time.Duration, file int64, op api.RevisionOp, content string) Revision {
	ht.t.Helper()
	ht.d.Now = func() time.Time { return retentionNow.Add(-ago) }
	defer func() { ht.d.Now = func() time.Time { return testNow } }()
	return ht.add(file, op, fmt.Sprintf("f%d.md", file), content)
}

func (ht *hubTest) prune(loc *time.Location) []string {
	ht.t.Helper()
	var freed []string
	ht.tx(func(tx *Tx) {
		var err error
		if freed, err = tx.PruneRevisions(ht.t.Context(), retentionNow, loc); err != nil {
			ht.t.Fatal(err)
		}
	})
	return freed
}

// pruned lists the seqs of the vault's pruned revisions.
func (ht *hubTest) pruned() []int64 {
	ht.t.Helper()
	rows, err := ht.d.db.QueryContext(ht.t.Context(), `SELECT seq FROM revisions WHERE pruned = 1 ORDER BY seq`)
	if err != nil {
		ht.t.Fatal(err)
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			ht.t.Fatal(err)
		}
		out = append(out, seq)
	}
	return out
}

const day = 24 * time.Hour

func TestPruneRevisions(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	// File 1, edited over more than a year.
	r1 := ht.at(400*day, 1, api.RevisionOpCreate, "a1")          // pruned: r2 is its month's last
	r2 := ht.at(400*day-time.Hour, 1, api.RevisionOpPut, "a2")   // kept: its month's last
	r3 := ht.at(100*day+9*time.Hour, 1, api.RevisionOpPut, "a3") // pruned: r4 is its day's last
	r4 := ht.at(100*day+2*time.Hour, 1, api.RevisionOpPut, "a4") // kept: its day's last
	r5 := ht.at(40*day+2*time.Hour, 1, api.RevisionOpPut, "a5")  // pruned
	r6 := ht.at(40*day+time.Hour, 1, api.RevisionOpMove, "a5")   // kept: a move is its day's last state, with r5's content
	r7 := ht.at(10*day, 1, api.RevisionOpPut, "a7")              // kept: under 30 days
	r8 := ht.at(day, 1, api.RevisionOpPut, "a8")                 // kept: under 30 days, and the head
	// File 2: its head is very old, and stays.
	h := ht.at(700*day, 2, api.RevisionOpCreate, "b1")
	// File 3: a conflict while flagged stays, and so does the head it lost to.
	ht.at(200*day+time.Hour, 3, api.RevisionOpCreate, "c1")
	k := ht.at(200*day, 3, api.RevisionOpConflict, "c-lost")
	// File 4: a resolved conflict is kept only for 30 days, and is never a day's last state.
	p4 := ht.at(90*day+time.Hour, 4, api.RevisionOpCreate, "d1")
	k4 := ht.at(90*day, 4, api.RevisionOpConflict, "d-lost")
	ht.at(80*day, 4, api.RevisionOpResolve, "d1")
	d4 := ht.at(50*day, 4, api.RevisionOpPut, "d2") // the head
	// File 5: deleted; its last state before the delete is its day's last.
	c5 := ht.at(120*day+time.Hour, 5, api.RevisionOpCreate, "e1")
	ht.at(120*day, 5, api.RevisionOpDelete, "")

	freed := ht.prune(time.UTC)
	got := ht.pruned()
	want := []int64{r1.Seq, r3.Seq, r5.Seq, k4.Seq}
	slices.Sort(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("pruned seqs %v, want %v (kept: %d %d %d %d %d, head %d, conflict %d, %d %d %d)",
			got, want, r2.Seq, r4.Seq, r6.Seq, r7.Seq, r8.Seq, h.Seq, k.Seq, p4.Seq, d4.Seq, c5.Seq)
	}
	// a5's content is still a kept move's, so only the others are freed.
	wantFreed := []string{fakeHash("a1"), fakeHash("a3"), fakeHash("d-lost")}
	slices.Sort(wantFreed)
	if fmt.Sprint(freed) != fmt.Sprint(wantFreed) {
		t.Fatalf("freed %v, want %v", freed, wantFreed)
	}
	if again := ht.prune(time.UTC); again == nil || len(again) != 0 {
		t.Fatalf("a second pass freed %#v, want a non-nil empty slice", again)
	}
	if got := ht.pruned(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("a second pass changed the pruned set to %v", got)
	}
	if ok, _ := ht.d.HasContent(ctx, fakeHash("a3")); ok {
		t.Fatal("a pruned revision's content is still handed out")
	}
}

func TestPruneRevisionsKeepsHashesOthersName(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	old := ht.at(100*day+2*time.Hour, 1, api.RevisionOpCreate, "shared")
	ht.at(100*day+time.Hour, 1, api.RevisionOpPut, "f1-day-last")
	ht.at(5*day, 2, api.RevisionOpCreate, "shared") // another file's kept revision
	ht.at(100*day+2*time.Hour, 3, api.RevisionOpCreate, "in-files")
	ht.at(100*day+time.Hour, 3, api.RevisionOpPut, "g2")
	ht.at(100*day+2*time.Hour, 4, api.RevisionOpCreate, "in-versions")
	ht.at(100*day+time.Hour, 4, api.RevisionOpPut, "h2")
	ht.tx(func(tx *Tx) {
		if _, err := tx.PutFile(ctx, File{VaultID: ht.vault.VaultID, Path: "x.md", Size: 1, ModTime: time.Unix(1, 0),
			Hash: fakeHash("in-files")}); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.AddVersion(ctx, Version{VaultID: ht.vault.VaultID, FileID: 9, Path: "y.md", Hash: fakeHash("in-versions"),
			Size: 1, Source: api.VersionSourceEditor}); err != nil {
			t.Fatal(err)
		}
	})
	freed := ht.prune(time.UTC)
	if len(ht.pruned()) != 3 || ht.pruned()[0] != old.Seq {
		t.Fatalf("pruned %v; want the three creates", ht.pruned())
	}
	if len(freed) != 0 {
		t.Fatalf("freed %v; each pruned hash is still named elsewhere", freed)
	}
}

func TestPruneRevisionsCountsDaysInTheHubsZone(t *testing.T) {
	// 03:00 and 06:00 UTC on 1 March are one day in UTC, two in UTC-5.
	first := retentionNow.Sub(time.Date(2027, 3, 1, 3, 0, 0, 0, time.UTC))
	second := retentionNow.Sub(time.Date(2027, 3, 1, 6, 0, 0, 0, time.UTC))
	for loc, want := range map[*time.Location]int{time.UTC: 1, time.FixedZone("UTC-5", -5*3600): 0} {
		ht := newHubTest(t)
		ht.at(first, 1, api.RevisionOpCreate, "x1")
		ht.at(second, 1, api.RevisionOpPut, "x2")
		ht.at(day, 1, api.RevisionOpPut, "x3")
		ht.prune(loc)
		if got := len(ht.pruned()); got != want {
			t.Errorf("in %s, pruned %d revisions, want %d", loc, got, want)
		}
	}
}

func TestPruneRevisionsCountsMonthsInTheHubsZone(t *testing.T) {
	// 31 March 23:00 and 1 April 02:00 UTC-5 are two months there, one in UTC.
	loc := time.FixedZone("UTC-5", -5*3600)
	first := retentionNow.Sub(time.Date(2026, 3, 31, 23, 0, 0, 0, loc))
	second := retentionNow.Sub(time.Date(2026, 4, 1, 2, 0, 0, 0, loc))
	for zone, want := range map[*time.Location]int{time.UTC: 1, loc: 0} {
		ht := newHubTest(t)
		ht.at(first, 1, api.RevisionOpCreate, "m1")
		ht.at(second, 1, api.RevisionOpPut, "m2")
		ht.at(day, 1, api.RevisionOpPut, "m3")
		ht.prune(zone)
		if got := len(ht.pruned()); got != want {
			t.Errorf("in %s, pruned %d revisions, want %d", zone, got, want)
		}
	}
}
