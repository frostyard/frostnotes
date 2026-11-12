package syncer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/store"
)

// TestUploadReportsABlobCheckError: a blobs store whose stat fails must not
// drop the hash. The session ends with the error, so loop.run logs it and
// retries, instead of the hub refusing the push as missing forever.
func TestUploadReportsABlobCheckError(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	bl := blobs.New(dir)
	id, err := bl.Put(ctx, strings.NewReader("a note's content"))
	if err != nil {
		t.Fatalf("put blob: %v", err)
	}
	// A regular file where the shard directory was makes stat fail with
	// ENOTDIR, which is not fs.ErrNotExist, for root as well.
	shard := filepath.Join(dir, id[:2])
	if err := os.RemoveAll(shard); err != nil {
		t.Fatalf("remove shard dir: %v", err)
	}
	if err := os.WriteFile(shard, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("replace shard dir with a file: %v", err)
	}

	s := New(Config{Blobs: bl})
	l := newLoop(s, 1, func() {})
	err = l.upload(ctx, nil, store.Remote{}, []store.LocalPush{{Hash: id}})
	if err == nil {
		t.Fatal("upload returned nil for a blob whose check failed; the hash was dropped")
	}
	if !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("upload error = %v, want it to wrap %v", err, syscall.ENOTDIR)
	}
}

// TestUploadSkipsAnAbsentBlob: a blob that is not there is skipped as
// before, so a push with no upload returns nil and never uses the session.
func TestUploadSkipsAnAbsentBlob(t *testing.T) {
	bl := blobs.New(t.TempDir())
	s := New(Config{Blobs: bl})
	l := newLoop(s, 1, func() {})
	if err := l.upload(t.Context(), nil, store.Remote{}, []store.LocalPush{{Hash: blobs.ID([]byte("never stored"))}}); err != nil {
		t.Fatalf("upload of an absent blob = %v, want nil", err)
	}
}

// brokenLog is an events.Log whose first LatestSeq fails.
type brokenLog struct {
	calls   atomic.Int64
	retried chan struct{} // closed when a later LatestSeq succeeds
	once    sync.Once
}

func (f *brokenLog) LatestSeq(context.Context) (int64, error) {
	if f.calls.Add(1) == 1 {
		return 0, errors.New("events: the log cannot be read")
	}
	f.once.Do(func() { close(f.retried) })
	return 0, nil
}

func (f *brokenLog) OldestSeq(context.Context) (int64, error) { return 0, nil }

func (f *brokenLog) ChangesSince(context.Context, int64) ([]api.EventEnvelope, error) {
	return nil, nil
}

// TestWatchLocalResubscribes: a failed Subscribe must not end watchLocal
// for good. It logs, waits, subscribes again, and the paths that change
// after that are marked dirty.
func TestWatchLocalResubscribes(t *testing.T) {
	var logs bytes.Buffer
	fl := &brokenLog{retried: make(chan struct{})}
	b := events.NewBroker(fl)
	s := New(Config{Broker: b, Log: slog.New(slog.NewTextHandler(&logs, nil)),
		Backoff: time.Millisecond, MaxBackoff: time.Millisecond})
	l := newLoop(s, 1, func() {})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.watchLocal(ctx)
	}()

	select {
	case <-fl.retried:
	case <-done:
		cancel()
		t.Fatal("watchLocal returned after a failed Subscribe")
	}

	const path = "notes/a.md"
	ev, err := api.NewEnvelope(0, api.NoteChanged{VaultID: 1, Path: path, Source: api.VersionSourceOutside})
	if err != nil {
		cancel()
		t.Fatalf("new envelope: %v", err)
	}
	// The kick after the recovered Subscribe and the event's own kick
	// coalesce in the one-slot wake channel. Publish again after each
	// consumed kick: the mark and its kick follow each other, so a second
	// publish always finds the channel empty. The timer is a failure
	// guard only.
	guard := time.After(10 * time.Second)
	for {
		b.Publish([]api.EventEnvelope{ev})
		select {
		case <-l.wake:
		case <-done:
			cancel()
			t.Fatal("watchLocal returned before it marked the changed path")
		case <-guard:
			cancel()
			t.Fatal("watchLocal marked no changed path after a recovered subscription")
		}
		if l.isDirty(path) {
			break
		}
	}

	cancel()
	select {
	case <-done:
	case <-guard:
		t.Fatal("watchLocal did not return after its context ended")
	}
	if got := logs.String(); !strings.Contains(got, "subscribe to local changes") {
		t.Fatalf("logs = %q, want a warning about the failed Subscribe", got)
	}
}

// isDirty reports whether path waits to be pushed.
func (l *loop) isDirty(path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.dirty[path]
	return ok
}
