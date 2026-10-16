// Package events fans committed events out to subscribed connections and
// replays the durable log for subscribers that resume (docs/specs/rpc-protocol.md).
package events

import (
	"context"
	"errors"
	"sync"

	"github.com/frostyard/frostnotes/api"
)

// Buffer is how many undelivered events a subscription holds before the
// broker drops it as lagging; the client reconnects and resumes by seq.
const Buffer = 1024

// ErrLagged ends a subscription whose consumer fell more than Buffer behind.
var ErrLagged = errors.New("events: subscriber lagged")

// Log is the durable event log (store.DB implements it).
type Log interface {
	LatestSeq(ctx context.Context) (int64, error)
	OldestSeq(ctx context.Context) (int64, error)
	ChangesSince(ctx context.Context, seq int64) ([]api.EventEnvelope, error)
}

// Broker delivers events to subscriptions. Publish never blocks.
type Broker struct {
	log  Log
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// NewBroker returns a broker that replays from log.
func NewBroker(log Log) *Broker {
	return &Broker{log: log, subs: map[*Subscription]struct{}{}}
}

// Publish delivers events, in order, to every subscription; it is the
// store's OnCommit hook. A full subscription is dropped with ErrLagged.
func (b *Broker) Publish(evs []api.EventEnvelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		for _, ev := range evs {
			select {
			case s.live <- ev:
				continue
			default:
			}
			s.fail(ErrLagged)
			delete(b.subs, s)
			break
		}
	}
}

// Subscription is one connection's event stream.
type Subscription struct {
	b      *Broker
	live   chan api.EventEnvelope
	replay []api.EventEnvelope
	done   chan struct{}
	once   sync.Once
	err    error
}

// Subscribe registers a subscription before reading the log, so no event
// falls between replay and live delivery. With sinceSeq nil it delivers live
// events only. The result reports resync when sinceSeq is no longer
// replayable, in which case nothing is replayed.
func (b *Broker) Subscribe(ctx context.Context, sinceSeq *int64) (*Subscription, api.Subscription, error) {
	s := &Subscription{b: b, live: make(chan api.EventEnvelope, Buffer), done: make(chan struct{})}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	latest, err := b.log.LatestSeq(ctx)
	if err != nil {
		s.Close()
		return nil, api.Subscription{}, err
	}
	info := api.Subscription{Seq: latest}
	if sinceSeq == nil || *sinceSeq == latest {
		return s, info, nil
	}
	oldest, err := b.log.OldestSeq(ctx)
	if err != nil {
		s.Close()
		return nil, api.Subscription{}, err
	}
	if *sinceSeq > latest || *sinceSeq < 0 || oldest == 0 || *sinceSeq < oldest-1 {
		info.Resync = true
		return s, info, nil
	}
	s.replay, err = b.log.ChangesSince(ctx, *sinceSeq)
	if err != nil {
		s.Close()
		return nil, api.Subscription{}, err
	}
	return s, info, nil
}

// Run sends the replay, then live events, until ctx ends, the subscription
// is closed or lags, or send fails. Durable live events already replayed are
// skipped, so each seq is delivered once and in order.
func (s *Subscription) Run(ctx context.Context, send func(api.EventEnvelope) error) error {
	var last int64
	for _, ev := range s.replay {
		if err := send(ev); err != nil {
			return err
		}
		last = ev.Seq
	}
	s.replay = nil
	for {
		select {
		case ev := <-s.live:
			if ev.Seq != 0 && ev.Seq <= last {
				continue
			}
			if err := send(ev); err != nil {
				return err
			}
		case <-s.done:
			return s.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Close unregisters the subscription and stops Run.
func (s *Subscription) Close() {
	s.b.mu.Lock()
	delete(s.b.subs, s)
	s.b.mu.Unlock()
	s.fail(nil)
}

func (s *Subscription) fail(err error) {
	s.once.Do(func() {
		s.err = err
		close(s.done)
	})
}
