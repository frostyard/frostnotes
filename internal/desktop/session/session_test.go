package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

type fixture struct {
	*desktoptest.Notesd
	loop *desktoptest.Loop
	s    *Session
}

func start(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{Notesd: desktoptest.StartNotesd(t), loop: desktoptest.NewLoop()}
	f.s = New(f.loop, func(ctx context.Context) (*api.Client, *api.Hello, error) {
		return api.Dial(ctx, f.Srv.Socket, ClientName)
	}, 0)
	f.s.Start()
	f.loop.Until(t, "the connection", func() bool { return f.s.State() == Connected })
	t.Cleanup(f.s.Close)
	return f
}

func TestEventsReachTheOpenNotes(t *testing.T) {
	f := start(t)
	base := f.Create("Plan.md", "one\n")
	d := f.s.Open(f.Vault, "Plan.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	if f.s.Open(f.Vault, "Plan.md") != d {
		t.Fatal("a second tab of the note got another document")
	}

	f.Write("Plan.md", base, "one\ntwo\n")
	f.loop.Until(t, "the change to arrive", func() bool { return d.Text() == "one\ntwo\n" })

	if _, err := f.Other.Note().Move(t.Context(), &api.NoteMoveParams{VaultID: f.Vault, Path: "Plan.md", To: "Ideas/Plan.md"}); err != nil {
		t.Fatal(err)
	}
	f.loop.Until(t, "the move to arrive", func() bool { return d.Path() == "Ideas/Plan.md" })
	if f.s.Seq() == 0 {
		t.Fatal("the session kept no event sequence")
	}
}

func TestTheSessionReconnects(t *testing.T) {
	f := start(t)
	f.Create("Plan.md", "one\n")
	d := f.s.Open(f.Vault, "Plan.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	f.Restart()
	f.loop.Until(t, "the drop", func() bool { return f.s.State() == Reconnecting })
	if f.s.Reason() == "" || f.s.Notes() != nil {
		t.Fatalf("reason %q, notes %v", f.s.Reason(), f.s.Notes())
	}
	f.loop.Advance(RetryDelay)
	f.loop.Until(t, "the reconnection", func() bool { return f.s.State() == Connected })

	// Events flow on the new connection.
	f.Write("Plan.md", f.Read("Plan.md").Hash, "one\nfrom elsewhere\n")
	f.loop.Until(t, "the change after the reconnection", func() bool { return d.Text() == "one\nfrom elsewhere\n" })
}

func TestReleaseSavesAndForgets(t *testing.T) {
	f := start(t)
	f.Create("Plan.md", "one\n")
	d := f.s.Open(f.Vault, "Plan.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	done := false
	f.s.Release(d)
	f.s.Flush(func() { done = true })
	f.loop.Until(t, "the flush", func() bool { return done })
	if f.s.Unsaved() {
		t.Fatal("unsaved after the flush")
	}
	if f.s.Open(f.Vault, "Plan.md") == d {
		t.Fatal("a released document came back")
	}
}

func TestAReleasedNoteKeepsItsTextUntilASaveLands(t *testing.T) {
	f := start(t)
	f.Create("Plan.md", "one\n")
	d := f.s.Open(f.Vault, "Plan.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	f.Srv.Stop()
	f.loop.Until(t, "the drop", func() bool { return f.s.State() == Reconnecting })
	cs, err := notes.ChangesOf(len(d.Text()), notes.Change{From: 3, To: 3, Insert: " two"})
	if err != nil {
		t.Fatal(err)
	}
	d.Edit(cs)

	f.s.Release(d) // the save fails and waits to retry
	if !f.s.Unsaved() {
		t.Fatal("the released note's text is gone")
	}
	f.Srv = rpctest.StartWith(t, rpctest.Options{DataDir: f.Data})
	f.App, f.Other = f.Srv.Dial(t), f.Srv.Dial(t)
	f.loop.Advance(RetryDelay)
	f.loop.Until(t, "the reconnection", func() bool { return f.s.State() == Connected })
	f.loop.Until(t, "the save", func() bool { return !f.s.Unsaved() })
	if got := f.Read("Plan.md").Content; got != "one two\n" {
		t.Fatalf("file holds %q", got)
	}
	if f.s.Open(f.Vault, "Plan.md") == d {
		t.Fatal("a saved, released document stayed")
	}
}

func TestARefusalAfterReleaseKeepsTheDocument(t *testing.T) {
	f := start(t)
	base := f.Create("Plan.md", "one\n")
	d := f.s.Open(f.Vault, "Plan.md")
	f.loop.Until(t, "the note to load", d.Loaded)
	cs, err := notes.ChangesOf(len(d.Text()), notes.Change{From: 3, To: 3, Insert: " typed"})
	if err != nil {
		t.Fatal(err)
	}
	d.Edit(cs)
	// The file moves on, and notesd no longer has the base to merge from.
	f.Write("Plan.md", base, "other\n")
	if err := os.Remove(filepath.Join(f.Blobs, base[:2], base)); err != nil {
		t.Fatal(err)
	}

	f.s.Release(d)
	f.loop.Until(t, "the refusal", func() bool { return d.Notice() != nil && d.Notice().Kind == notes.NoticeRefused })
	if !f.s.Unsaved() {
		t.Fatal("the refused text is not counted")
	}
	if got := f.s.Open(f.Vault, "Plan.md"); got != d {
		t.Fatal("the refused text was disposed with its document")
	}
	if n := d.Notice(); n == nil || n.Text != "one typed\n" {
		t.Fatalf("notice %+v", n)
	}
	d.Dismiss()
	f.s.Release(d)
	f.loop.Until(t, "the release", func() bool { return !f.s.Unsaved() })
	if f.s.Open(f.Vault, "Plan.md") == d {
		t.Fatal("a dismissed, released document stayed")
	}
}
