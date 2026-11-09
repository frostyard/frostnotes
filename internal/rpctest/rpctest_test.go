package rpctest

import (
	"context"
	"testing"
)

// TestDoneClosesAfterDerivedContextsEnd checks what Server.Done promises:
// it closes only after Stop's cancel has ended every context derived from
// the server's context. With the parent context's own Done channel instead,
// a goroutine it wakes can still find a derived context alive: Go closes a
// parent's Done channel before it cancels the contexts derived from it.
func TestDoneClosesAfterDerivedContextsEnd(t *testing.T) {
	srv := StartWith(t, Options{})
	const derived = 20000
	ctxs := make([]context.Context, 0, derived)
	cancels := make([]context.CancelFunc, 0, derived)
	for range derived {
		ctx, cancel := context.WithCancel(srv.ctx)
		ctxs = append(ctxs, ctx)
		cancels = append(cancels, cancel)
	}
	t.Cleanup(func() {
		for _, cancel := range cancels {
			cancel()
		}
	})

	live := make(chan int, 1)
	go func() {
		<-srv.Done()
		var n int
		for _, ctx := range ctxs {
			if ctx.Err() == nil {
				n++
			}
		}
		live <- n
	}()
	srv.Stop()
	if n := <-live; n != 0 {
		t.Errorf("Done closed before the contexts derived from the server's context ended: %d of %d are still live", n, derived)
	}
}
