package hub

import (
	"testing"
	"time"
)

// A peer has limitBurst requests at once and one more every limitEvery;
// another peer has its own (docs/specs/lodge-app.md#limits).
func TestAPeerHasItsBucketOfRequests(t *testing.T) {
	var l limiter
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := range limitBurst {
		if !l.allow("node-a", at) {
			t.Fatalf("request %d of %d was refused", i+1, limitBurst)
		}
	}
	if l.allow("node-a", at) {
		t.Fatal("a request past the burst was allowed")
	}
	if !l.allow("node-b", at) {
		t.Fatal("another peer's request was refused")
	}
	at = at.Add(limitEvery)
	if !l.allow("node-a", at) {
		t.Fatal("a request a refill later was refused")
	}
	if l.allow("node-a", at) {
		t.Fatal("a second request in one refill was allowed")
	}
}

// Past limitPeers buckets, the full ones go, and then new peers share one.
func TestTheBucketsAreBounded(t *testing.T) {
	var l limiter
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for i := range limitPeers {
		l.allow(string(rune(0x1000+i)), at)
	}
	if len(l.buckets) != limitPeers {
		t.Fatalf("%d buckets, want %d", len(l.buckets), limitPeers)
	}
	// None has refilled: the next peer shares the overflow bucket.
	for range limitBurst {
		l.allow("late-1", at)
	}
	if l.allow("late-2", at) {
		t.Fatal("the overflow bucket was not shared")
	}
	// Once they have refilled, they go.
	later := at.Add(limitEvery)
	if !l.allow("late-3", later) || len(l.buckets) > 2 {
		t.Fatalf("after the refill, %d buckets are kept", len(l.buckets))
	}
}
