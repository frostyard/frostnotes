//go:build integration

package syncer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// TestISyncLatency measures how long an edit on one device takes to reach
// another through a hub on loopback, with the default quiet period (plan
// 0004, Phase 4's open question on latency). It reports; it sets no bar.
func TestISyncLatency(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": "one\n"})
	o := rpctest.Options{Syncer: syncer.Config{Device: "a"}}
	a := pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "a")
	o.Syncer.Device = "b"
	b := pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "b")
	settle(t, h, a, b)
	ctx := t.Context()
	watch, _, err := api.Dial(ctx, b.srv.Socket, "watch")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watch.Close() }()
	if _, err := watch.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	var total time.Duration
	const runs = 5
	for i := range runs {
		begin := time.Now()
		a.write("n.md", fmt.Sprintf("edit %d\n", i))
		deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
		for done := false; !done; {
			select {
			case ev := <-watch.Notifications():
				if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
					if ch, ok := e.(api.NoteChanged); ok && ch.Path == "n.md" && ch.Source == api.VersionSourceSync {
						done = true
					}
				}
			case <-deadline.Done():
				t.Fatal("the edit did not arrive")
			}
		}
		cancel()
		took := time.Since(begin)
		total += took
		t.Logf("edit %d reached b after %v", i, took)
	}
	t.Logf("mean %v over %d edits, with the default quiet period", total/runs, runs)
}
