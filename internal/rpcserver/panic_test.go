package rpcserver_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/problem/problemtest"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// panicky is a note service whose list panics; its other methods are
// never called.
type panicky struct{ api.NoteService }

func (panicky) List(context.Context, *api.NoteListParams) ([]api.NoteEntry, error) {
	panic("a bug in a handler")
}

// A handler that panics answers internal; notesd and the connection live on.
func TestPanickingHandlerAnswersInternal(t *testing.T) {
	var o rpctest.Options
	o.Services = func(s *api.Services) { s.Note = panicky{} }
	srv := rpctest.StartWith(t, o)
	c := srv.Dial(t)
	if _, err := c.Note().List(t.Context(), &api.NoteListParams{VaultID: 1}); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("List = %v, want internal", err)
	}
	if _, err := c.Vault().List(t.Context(), &api.VaultListParams{}); err != nil {
		t.Fatalf("the connection after a panic: %v", err)
	}
}

// With a recorder, a handler's panic is recorded in the journal (class
// panic, with notesd's version and the handler's frame), and the client
// gets the same answer as without one: internal, and the connection lives
// on (ADR-0036).
func TestAPanickingHandlerIsRecorded(t *testing.T) {
	j := problemtest.Listen(t)
	var o rpctest.Options
	o.Services = func(s *api.Services) { s.Note = panicky{} }
	o.Panics = problems.New(problemtest.AppID, "1.2.3", problems.ComponentDaemon, problems.WithSocket(j.Path))
	srv := rpctest.StartWith(t, o)
	c := srv.Dial(t)
	if _, err := c.Vault().List(t.Context(), &api.VaultListParams{}); err != nil {
		t.Fatal(err)
	}
	j.None(t) // a request that returns records nothing
	_, err := c.Note().List(t.Context(), &api.NoteListParams{VaultID: 1})
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Code != api.CodeInternal || ae.Message != "note.list failed" {
		t.Fatalf("List = %v, want internal \"note.list failed\", the kit's answer to a panic", err)
	}
	f := j.Next(t)
	problemtest.Check(t, f, problemtest.Want{Version: "1.2.3", Class: "panic"})
	if want := "github.com/frostyard/frostnotes/internal/rpcserver_test.panicky.List"; !slices.Contains(strings.Split(f["FROSTYARD_FRAMES"], "\n"), want) {
		t.Errorf("frames %q lack %s", f["FROSTYARD_FRAMES"], want)
	}
	if f["MESSAGE"] != "a bug in a handler" {
		t.Errorf("MESSAGE = %q; want the panic value", f["MESSAGE"])
	}
	if _, err := c.Vault().List(t.Context(), &api.VaultListParams{}); err != nil {
		t.Fatalf("the connection after a panic: %v", err)
	}
}

// huge is a note service whose read answers more than a message holds.
type huge struct{ api.NoteService }

func (huge) Read(context.Context, *api.NoteReadParams) (*api.Note, error) {
	return &api.Note{Path: "Big.md", Content: strings.Repeat("x", api.MaxMessageSize)}, nil
}

// A reply over the message limit answers internal instead, which the
// client can read; the connection lives on (review finding 2, plan 0005
// Phase 7).
func TestAReplyTooLargeAnswersInternal(t *testing.T) {
	var o rpctest.Options
	o.Services = func(s *api.Services) { s.Note = huge{} }
	srv := rpctest.StartWith(t, o)
	c := srv.Dial(t)
	if _, err := c.Note().Read(t.Context(), &api.NoteReadParams{VaultID: 1, Path: "Big.md"}); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("Read = %v, want internal", err)
	}
	if _, err := c.Vault().List(t.Context(), &api.VaultListParams{}); err != nil {
		t.Fatalf("the connection after a reply too large: %v", err)
	}
}
