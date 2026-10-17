package main

// CONTRACT TEST for task card T-0031 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func TestSyncStatus(t *testing.T) {
	synced := when.Add(-time.Minute)
	dev := &fakeSync{status: []api.SyncStatus{
		{VaultID: 1, URL: "https://hub.example", HubVault: "Notes", State: api.SyncStateIdle, Cursor: 40, HubSeq: 42,
			Pending: 2, Conflicts: 1, Refused: []api.SyncRefusal{{Path: "Plan.md", Reason: api.RefuseReasonCollision, At: when}},
			Skipped: []string{".hidden/x.md"}, SyncedAt: &synced},
		{VaultID: 2, URL: "https://hub.example", HubVault: "Work", State: api.SyncStateOffline, Error: ptr("dial tcp: refused")},
	}}
	srv := syncServer(t, &fakeHub{}, dev, "garden", "work")
	want := "1\tNotes\thttps://hub.example\tidle\t40/42\t2 pending\t1 conflicted\n" +
		"\trefused Plan.md: collision\n" +
		"\tskipped .hidden/x.md\n" +
		"2\tWork\thttps://hub.example\toffline\t0/0\t0 pending\t0 conflicted\n" +
		"\terror: dial tcp: refused\n"
	if out := notes(t, srv, "sync", "status"); out != want {
		t.Fatalf("sync status = %q, want %q", out, want)
	}
	checkCalls(t, dev.took(), &api.SyncStatusParams{})
	notes(t, srv, "--vault", "work", "sync", "status")
	checkCalls(t, dev.took(), &api.SyncStatusParams{VaultID: ptr(int64(2))})
	var got []api.SyncStatus
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "sync", "status")), &got); err != nil || len(got) != 2 {
		t.Fatalf("sync status --json = %+v, %v", got, err)
	}
	dev.took()
	dev.status = nil
	if out := notes(t, srv, "sync", "status"); out != "no synced vaults\n" {
		t.Fatalf("sync status with none = %q", out)
	}
}

func TestSyncConflictsAndResolve(t *testing.T) {
	dev := &fakeSync{}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	if out := notes(t, srv, "sync", "conflicts"); out != "no conflicts\n" {
		t.Fatalf("sync conflicts with none = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncConflictsParams{VaultID: 1})
	dev.conflicts = []api.SyncConflict{{Path: "Plan.md", File: 7, Revisions: []api.Revision{
		{Seq: 12, File: 7, Op: api.RevisionOpConflict, Path: "Plan.md", Hash: strings.Repeat("a", 64), DeviceName: "laptop",
			Source: api.VersionSourceEditor, Author: "frostnotes-app", At: when},
		{Seq: 15, File: 7, Op: api.RevisionOpConflict, Path: "Plan.md", Hash: strings.Repeat("b", 64), DeviceName: "desk",
			Source: api.VersionSourceOutside, At: when.Add(time.Hour)},
	}}}
	want := "Plan.md\n\t12\t2026-10-16T21:10:00Z\tlaptop\tfrostnotes-app\n\t15\t2026-10-16T22:10:00Z\tdesk\t-\n"
	if out := notes(t, srv, "sync", "conflicts"); out != want {
		t.Fatalf("sync conflicts = %q, want %q", out, want)
	}
	dev.took()
	if out := notes(t, srv, "sync", "resolve", "Plan.md"); out != "resolved Plan.md\n" {
		t.Fatalf("sync resolve = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncResolveParams{VaultID: 1, Path: "Plan.md"})
}

func TestSyncResetAndLeave(t *testing.T) {
	dev := &fakeSync{status: []api.SyncStatus{{VaultID: 1, URL: "https://hub.example", HubVault: "Notes", State: api.SyncStateSyncing}}}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	if out := notes(t, srv, "sync", "reset"); out != "reset vault 1: syncing\n" {
		t.Fatalf("sync reset = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncResetParams{VaultID: 1})
	if out := notes(t, srv, "sync", "leave"); out != "vault 1 no longer syncs; its files stay\n" {
		t.Fatalf("sync leave = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncLeaveParams{VaultID: 1})
}

func TestSyncVerify(t *testing.T) {
	dev := &fakeSync{report: api.VerifyReport{Ok: true, Files: 674, Differing: []string{}, Unrecorded: []int64{}, Conflicts: []string{"Plan.md"}}}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	if out := notes(t, srv, "sync", "verify"); out != "conflicted\tPlan.md\nok: 674 files match the hub\n" {
		t.Fatalf("sync verify = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncVerifyParams{VaultID: 1})
	dev.report = api.VerifyReport{Files: 674, Differing: []string{"a.md", "b/c.png"}, Unrecorded: []int64{31}, Conflicts: []string{}}
	out, err := runNotesctl(t, "--socket", srv.Socket, "sync", "verify")
	if err == nil || !strings.Contains(err.Error(), "differs from its hub") {
		t.Fatalf("sync verify of a vault that differs = %v, want an error", err)
	}
	if want := "differs\ta.md\ndiffers\tb/c.png\nunrecorded\tpush 31\n"; out != want {
		t.Fatalf("sync verify = %q, want %q", out, want)
	}
}

func TestHistory(t *testing.T) {
	dev := &fakeSync{history: []api.Revision{
		{Seq: 20, File: 3, Op: api.RevisionOpMove, Path: "b/Plan.md", Hash: strings.Repeat("c", 64), DeviceName: "laptop",
			Source: api.VersionSourceEditor, Author: "notesctl", At: when.Add(time.Hour)},
		{Seq: 9, File: 3, Op: api.RevisionOpDelete, Path: "Plan.md", DeviceName: "hub", Source: api.VersionSourceOutside, At: when},
		{Seq: 4, File: 3, Op: api.RevisionOpCreate, Path: "Plan.md", Hash: strings.Repeat("d", 64), DeviceName: "desk",
			Source: api.VersionSourceAgent, Author: "claude-code", At: when, Pruned: true},
	}}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	want := "20\t2026-10-16T22:10:00Z\tmove\tlaptop\tnotesctl\tcccccccccccc\tb/Plan.md\n" +
		"9\t2026-10-16T21:10:00Z\tdelete\thub\t-\tdeleted\tPlan.md\n" +
		"4\t2026-10-16T21:10:00Z\tcreate\tdesk\tclaude-code\tdddddddddddd pruned\tPlan.md\n"
	if out := notes(t, srv, "history", "b/Plan.md"); out != want {
		t.Fatalf("history = %q, want %q", out, want)
	}
	checkCalls(t, dev.took(), &api.SyncHistoryParams{VaultID: 1, Path: "b/Plan.md"})
	notes(t, srv, "history", "b/Plan.md", "--limit", "5")
	checkCalls(t, dev.took(), &api.SyncHistoryParams{VaultID: 1, Path: "b/Plan.md", Limit: ptr(int64(5))})
	var got []api.Revision
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "history", "b/Plan.md")), &got); err != nil || len(got) != 3 {
		t.Fatalf("history --json = %+v, %v", got, err)
	}
}
