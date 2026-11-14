package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/events"
)

// The prune pass keeps the changes log's newest Config.KeepEvents events
// and drops those older than Config.Keep: a subscriber from before the
// retained log gets resync, and one inside it gets the newest events.
func TestPruneTrimsTheChangesLog(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": "v0\n"}, func(c *Config) { c.KeepEvents = 3 })
	tv.scan()
	hash := tv.row("Plan.md").Hash
	for i := range 5 {
		hash = tv.write("Plan.md", hash, fmt.Sprintf("v%d\n", i+1), editor).Hash
	}
	ctx := t.Context()
	latest := latestSeq(t, tv)
	if latest < 6 {
		t.Fatalf("latest seq before the prune = %d, want at least 6", latest)
	}
	m := &Manager{cfg: *tv.v.cfg, vaults: map[int64]*Vault{tv.v.id: tv.v}}
	if err := m.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	b := events.NewBroker(tv.db)
	if info := resync(t, b, latest-4); info.Seq != latest {
		t.Errorf("resync from seq %d gives seq %d, want %d", latest-4, info.Seq, latest)
	}
	if got := replay(t, b, latest-3, 3); !slices.Equal(got, []int64{latest - 2, latest - 1, latest}) {
		t.Errorf("replay from seq %d = %v, want the newest 3 events", latest-3, got)
	}

	// 31 days on, one more write: the age rule drops every older event.
	*tv.clock = tv.clock.Add(31 * 24 * time.Hour)
	tv.write("Plan.md", hash, "v6\n", editor)
	newest := latestSeq(t, tv)
	if err := m.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if oldest, err := tv.db.OldestSeq(ctx); err != nil || oldest != latest+1 {
		t.Errorf("oldest seq after the age prune = %d, %v; want %d", oldest, err, latest+1)
	}
	resync(t, b, latest-1)
	if got := replay(t, b, latest, int(newest-latest)); len(got) == 0 || got[len(got)-1] != newest {
		t.Errorf("replay from seq %d = %v, want up to %d", latest, got, newest)
	}
}

func latestSeq(t *testing.T, tv *testVault) int64 {
	t.Helper()
	seq, err := tv.db.LatestSeq(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

// resync subscribes from since, which must need a resync.
func resync(t *testing.T, b *events.Broker, since int64) api.Subscription {
	t.Helper()
	s, info, err := b.Subscribe(t.Context(), &since)
	if err != nil {
		t.Fatalf("subscribe from seq %d: %v", since, err)
	}
	s.Close()
	if !info.Resync {
		t.Errorf("subscribe from seq %d = %+v, want resync", since, info)
	}
	return info
}

// replay subscribes from since, which must not need a resync, and returns
// the seqs of the first n events replayed.
func replay(t *testing.T, b *events.Broker, since int64, n int) []int64 {
	t.Helper()
	s, info, err := b.Subscribe(t.Context(), &since)
	if err != nil || info.Resync {
		t.Fatalf("subscribe from seq %d = %+v, %v; want a replay", since, info, err)
	}
	defer s.Close()
	// The timeout only bounds a failing test: the replay is sent at once.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var seqs []int64
	enough := errors.New("enough")
	err = s.Run(ctx, func(ev api.EventEnvelope) error {
		seqs = append(seqs, ev.Seq)
		if len(seqs) == n {
			return enough
		}
		return nil
	})
	if !errors.Is(err, enough) {
		t.Fatalf("replay from seq %d = %v after %v", since, err, seqs)
	}
	return seqs
}
