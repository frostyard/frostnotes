package syncer_test

import (
	"errors"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
)

// A revision's content comes from the device's blobs, or from the hub
// when the device never pulled it.
func TestReadRevisionGivesItsContent(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
	ctx := t.Context()
	b.net.cut()
	a.write("n.md", fiveLines+"six\n")
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	a.write("n.md", fiveLines+"six\nseven\n")
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	b.net.heal()
	settle(t, h, a.node, b.node)

	unpulled := blobs.ID([]byte(fiveLines + "six\n"))
	if has, err := b.srv.Blobs.Has(unpulled); err != nil || has {
		t.Fatalf("b holds the revision it never pulled: %v, %v", has, err)
	}
	revs, err := b.c.Sync().History(ctx, &api.SyncHistoryParams{VaultID: b.vault, Path: "n.md"})
	if err != nil || len(revs) != 3 {
		t.Fatalf("history = %+v, %v", revs, err)
	}
	for i, want := range []string{fiveLines + "six\nseven\n", fiveLines + "six\n", fiveLines} {
		got, err := b.c.Sync().ReadRevision(ctx, &api.SyncReadRevisionParams{VaultID: b.vault, Seq: revs[i].Seq})
		if err != nil || got.Content != want || got.Revision.Seq != revs[i].Seq || got.Revision.Hash != revs[i].Hash {
			t.Errorf("ReadRevision(%d) = %+v, %v; want %q", revs[i].Seq, got, err, want)
		}
	}
	if has, err := b.srv.Blobs.Has(unpulled); err != nil || !has {
		t.Errorf("the fetched revision is not in b's blobs: %v, %v", has, err)
	}

	if _, err := b.c.Sync().ReadRevision(ctx, &api.SyncReadRevisionParams{VaultID: b.vault, Seq: 9999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("ReadRevision of a revision the hub lacks = %v, want notFound", err)
	}
	if _, err := h.c.Sync().ReadRevision(ctx, &api.SyncReadRevisionParams{VaultID: h.vault, Seq: revs[0].Seq}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("ReadRevision on the hub's own vault = %v, want notFound: it syncs with no hub", err)
	}
}
