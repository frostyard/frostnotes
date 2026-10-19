package hub

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// What a peer may ask of the hub without a device token, POST /sync/pair
// and POST /sync/hub (docs/specs/lodge-app.md#limits): a bucket of
// limitBurst requests, refilled at one every limitEvery, per peer. Behind
// lodge the peer is lodge's node ID, a rate key and never an admission;
// elsewhere it is the remote address.
const (
	limitBurst = 30
	limitEvery = time.Second
	// limitPeers bounds the buckets kept; past it, full buckets go, and
	// then new peers share one.
	limitPeers = 1024
)

// limiter is a token bucket per peer.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	at     time.Time
}

// allow takes a request from key's bucket at now, and reports whether
// there was one to take.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = map[string]*bucket{}
	}
	b := l.buckets[key]
	if b == nil {
		if len(l.buckets) >= limitPeers {
			l.prune(now)
		}
		if len(l.buckets) >= limitPeers {
			key = "" // a shared bucket for the overflow
			b = l.buckets[key]
		}
	}
	if b == nil {
		b = &bucket{tokens: limitBurst, at: now}
		l.buckets[key] = b
	}
	b.tokens = min(limitBurst, b.tokens+float64(now.Sub(b.at))/float64(limitEvery))
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// prune forgets the buckets that have refilled.
func (l *limiter) prune(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+float64(now.Sub(b.at))/float64(limitEvery) >= limitBurst {
			delete(l.buckets, k)
		}
	}
}

// limited answers 429 to a peer past its bucket, with when to try again.
func (h *Hub) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := peerOf(r).Key
		if key == "" {
			key, _, _ = net.SplitHostPort(r.RemoteAddr)
		}
		if !h.limit.allow(key, h.cfg.Now()) {
			w.Header().Set("Retry-After", strconv.Itoa(int(limitEvery/time.Second)))
			http.Error(w, "too many requests from this peer; try again shortly", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}
