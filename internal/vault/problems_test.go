package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/std/problems"
	"github.com/fsnotify/fsnotify"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/merge"
	"github.com/frostyard/frostnotes/internal/problem"
	"github.com/frostyard/frostnotes/internal/problem/problemtest"
)

// The records of this package's tests: few, under the recorder's cap of
// 10 a minute for each process.
const testVersion = "1.2.3"

// journalVault is a test vault whose problems go to a test journal.
func journalVault(t *testing.T, files map[string]string, tune func(*Config)) (*testVault, *problemtest.Journal) {
	t.Helper()
	j := problemtest.Listen(t)
	rec := problems.New(problemtest.AppID, testVersion, problems.ComponentDaemon, problems.WithSocket(j.Path))
	return newTestVault(t, files, func(c *Config) {
		c.Problems = rec
		if tune != nil {
			tune(c)
		}
	}), j
}

// A write whose content the blob store cannot keep fails with an internal
// error, recorded as vault.write_failed (ADR-0036).
func TestAWriteThatFailsOnDiskIsRecorded(t *testing.T) {
	store := filepath.Join(t.TempDir(), "blobs")
	tv, j := journalVault(t, map[string]string{"Plan.md": three}, func(c *Config) { c.Blobs = blobs.New(store) })
	tv.scan()
	base := tv.row("Plan.md").Hash
	// The blob store's folder becomes a file: every put fails.
	if err := os.RemoveAll(store); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := tv.v.Write(t.Context(), "Plan.md", base, []byte("changed\n"), editor)
	var ae *api.Error
	if err == nil || errors.As(err, &ae) {
		t.Fatalf("Write = %v; want an internal error", err)
	}
	f := j.Next(t)
	problemtest.Check(t, f, problemtest.Want{Version: testVersion, Class: "error", Code: problem.VaultWriteFailed})
	j.None(t)
}

// A client's error (a stale base, a bad path, a create on a file that
// exists) is the client's: nothing is recorded.
func TestAClientErrorOfAWriteIsNotRecorded(t *testing.T) {
	tv, j := journalVault(t, map[string]string{"Plan.md": three}, nil)
	tv.scan()
	if _, err := tv.v.Write(t.Context(), "Plan.md", "stale", []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("Write with a stale base = %v; want conflict", err)
	}
	if _, err := tv.v.Write(t.Context(), "../Plan.md", "", []byte("x"), editor); !errors.Is(err, api.ErrInvalidParams) {
		t.Fatalf("Write outside the vault = %v; want invalidParams", err)
	}
	if _, err := tv.v.Create(t.Context(), "Plan.md", []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("Create on a file that exists = %v; want conflict", err)
	}
	j.None(t)
}

// writeFailed records only an error that is not the client's, nor the end
// of the request, nor a simulated crash.
func TestWriteFailedRecordsOnlyInternalErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		record bool
	}{
		{"nil", nil, false},
		{"disk", errors.New("write Plan.md: no space left on device"), true},
		{"wrapped disk", fmt.Errorf("blobs: %w", os.ErrPermission), true},
		{"conflict", api.Conflict("Plan.md changed"), false},
		{"wrapped invalid params", fmt.Errorf("x: %w", api.InvalidParams("bad")), false},
		{"canceled", fmt.Errorf("blobs: %w", context.Canceled), false},
		{"deadline", context.DeadlineExceeded, false},
		{"crashed", ErrCrashed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &problemtest.Fake{}
			tv := newTestVault(t, nil, func(c *Config) { c.Problems = fake })
			if got := tv.v.writeFailed(tc.err); got != tc.err {
				t.Fatalf("writeFailed returned %v; want %v", got, tc.err)
			}
			codes := fake.Codes()
			if tc.record != (len(codes) == 1 && codes[0] == problem.VaultWriteFailed) || len(codes) > 1 {
				t.Fatalf("recorded %q; want a record: %v", codes, tc.record)
			}
		})
	}
}

// A panic of the merge library is recorded as merge.failed, and goes on
// with the same value; a merge that returns records nothing.
func TestAMergeThatPanicsIsRecordedAndPanicsAgain(t *testing.T) {
	j := problemtest.Listen(t)
	rec := problems.New(problemtest.AppID, testVersion, problems.ComponentDaemon, problems.WithSocket(j.Path))
	if got := guardMerge(rec, func() merge.Result { return merge.Text([]byte("a\n"), []byte("b\n"), []byte("a\n")) }); string(got.Text) != "b\n" {
		t.Fatalf("guardMerge = %q; want the merge's result", got.Text)
	}
	j.None(t)

	boom := errors.New("a bug in the merge")
	func() {
		defer func() {
			if x := recover(); x != boom {
				t.Fatalf("recovered %v; want the merge's own panic value", x)
			}
		}()
		guardMerge(rec, func() merge.Result { panic(boom) })
		t.Fatal("guardMerge returned after a panic")
	}()
	problemtest.Check(t, j.Next(t), problemtest.Want{Version: testVersion, Class: "error", Code: problem.MergeFailed})
}

// An inotify overflow is recorded as watch.overflow_rescan before the
// rescan; another watcher error only rescans, as before.
func TestAWatchOverflowIsRecorded(t *testing.T) {
	tv, j := journalVault(t, nil, nil)
	w := &watcher{v: tv.v}

	w.watchError(errors.New("some other watcher error"))
	j.None(t)
	if len(tv.v.scanReq) != 1 {
		t.Fatal("another watcher error did not ask for a scan")
	}
	<-tv.v.scanReq

	w.watchError(fsnotify.ErrEventOverflow)
	problemtest.Check(t, j.Next(t), problemtest.Want{Version: testVersion, Class: "error", Code: problem.WatchOverflowRescan})
	if len(tv.v.scanReq) != 1 {
		t.Fatal("an overflow did not ask for a scan")
	}
}
