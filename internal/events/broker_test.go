package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// memLog is an in-memory Log holding seqs oldest..latest.
type memLog struct{ evs []api.EventEnvelope }

func (m *memLog) LatestSeq(context.Context) (int64, error) {
	if len(m.evs) == 0 {
		return 0, nil
	}
	return m.evs[len(m.evs)-1].Seq, nil
}

func (m *memLog) OldestSeq(context.Context) (int64, error) {
	if len(m.evs) == 0 {
		return 0, nil
	}
	return m.evs[0].Seq, nil
}

func (m *memLog) ChangesSince(_ context.Context, seq int64) ([]api.EventEnvelope, error) {
	var out []api.EventEnvelope
	for _, ev := range m.evs {
		if ev.Seq > seq {
			out = append(out, ev)
		}
	}
	return out, nil
}

func durable(seq int64) api.EventEnvelope {
	return api.EventEnvelope{Seq: seq, Event: "account.changed", Data: []byte(`{}`)}
}

func ptr(v int64) *int64 { return &v }

// collect runs s until n events arrive.
func collect(t *testing.T, s *Subscription, n int) []int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var seqs []int64
	stop := errors.New("enough")
	err := s.Run(ctx, func(ev api.EventEnvelope) error {
		seqs = append(seqs, ev.Seq)
		if len(seqs) == n {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Run = %v after %v", err, seqs)
	}
	return seqs
}

func TestReplayThenLiveWithoutDuplicates(t *testing.T) {
	log := &memLog{evs: []api.EventEnvelope{durable(1), durable(2), durable(3)}}
	b := NewBroker(log)
	s, info, err := b.Subscribe(t.Context(), ptr(1))
	if err != nil || info.Seq != 3 || info.Resync {
		t.Fatalf("Subscribe = %+v, %v", info, err)
	}
	// Seq 3 was committed and published after registration but before the
	// replay read: it arrives on both paths and must be delivered once.
	b.Publish([]api.EventEnvelope{durable(3), durable(4), {Event: "view.delta"}})
	got := collect(t, s, 4)
	want := []int64{2, 3, 4, 0}
	if len(got) != len(want) {
		t.Fatalf("seqs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seqs = %v, want %v", got, want)
		}
	}
}

func TestResyncWhenPruned(t *testing.T) {
	b := NewBroker(&memLog{evs: []api.EventEnvelope{durable(10), durable(11)}})
	for _, since := range []int64{5, 99, -1} {
		_, info, err := b.Subscribe(t.Context(), ptr(since))
		if err != nil || !info.Resync {
			t.Errorf("sinceSeq %d: %+v, %v; want resync", since, info, err)
		}
	}
	if _, info, _ := b.Subscribe(t.Context(), ptr(9)); info.Resync {
		t.Error("sinceSeq 9 (oldest-1) must replay, not resync")
	}
}

func TestLiveOnly(t *testing.T) {
	b := NewBroker(&memLog{evs: []api.EventEnvelope{durable(1)}})
	s, info, err := b.Subscribe(t.Context(), nil)
	if err != nil || info.Seq != 1 {
		t.Fatalf("Subscribe = %+v, %v", info, err)
	}
	b.Publish([]api.EventEnvelope{durable(2)})
	if got := collect(t, s, 1); got[0] != 2 {
		t.Fatalf("live = %v", got)
	}
}

func TestLaggingSubscriberIsDropped(t *testing.T) {
	b := NewBroker(&memLog{})
	s, _, err := b.Subscribe(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	evs := make([]api.EventEnvelope, Buffer+1)
	for i := range evs {
		evs[i] = durable(int64(i + 1))
	}
	b.Publish(evs)
	err = s.Run(t.Context(), func(api.EventEnvelope) error { return nil })
	if !errors.Is(err, ErrLagged) {
		t.Fatalf("Run = %v, want ErrLagged", err)
	}
	if len(b.subs) != 0 {
		t.Fatal("lagging subscription still registered")
	}
}
