package store

import (
	"context"
	"errors"
	"testing"
)

// TestACachedLinkLookupStillSeesACancelledContext guards linkResolver's
// cache: a cached answer must fail as its query would once ctx is done, with
// the same error the query gives.
func TestACachedLinkLookupStillSeesACancelledContext(t *testing.T) {
	ft := newFileTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ft.tx(func(tx *Tx) {
		ft.put(tx, "Inbox.md")
		ft.put(tx, "Plan.md")

		// Fill both caches with a live context: Inbox.md -> Plan looks up
		// the name and then the path; Plan.md -> "" looks up the path only.
		r := newLinkResolver(tx, ft.vault)
		for _, c := range []struct{ source, target, want string }{
			{"Inbox.md", "Plan", "Plan.md"},
			{"Plan.md", "", "Plan.md"},
		} {
			f, ok, err := r.resolve(ctx, c.source, c.target)
			if err != nil || !ok || f.Path != c.want {
				t.Fatalf("resolve %q -> %q with a live context = %+v, %t, %v; want %q", c.source, c.target, f, ok, err, c.want)
			}
		}
		cancel()

		for _, c := range []struct{ source, target string }{{"Inbox.md", "Plan"}, {"Plan.md", ""}} {
			_, _, cachedErr := r.resolve(ctx, c.source, c.target)
			_, _, freshErr := newLinkResolver(tx, ft.vault).resolve(ctx, c.source, c.target)
			if cachedErr == nil {
				t.Fatalf("resolve %q -> %q from the cache after cancel returned no error, want %v", c.source, c.target, freshErr)
			}
			if freshErr == nil || cachedErr.Error() != freshErr.Error() {
				t.Errorf("cached %q -> %q after cancel = %q, want %q", c.source, c.target, cachedErr, freshErr)
			}
			if !errors.Is(cachedErr, context.Canceled) {
				t.Errorf("cached %q -> %q after cancel = %v, want context.Canceled", c.source, c.target, cachedErr)
			}
		}
	})
}
