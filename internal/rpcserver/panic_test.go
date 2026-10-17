package rpcserver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/frostyard/frostnotes/api"
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
	srv := rpctest.StartWith(t, rpctest.Options{Note: panicky{}})
	c := srv.Dial(t)
	if _, err := c.Note().List(t.Context(), &api.NoteListParams{VaultID: 1}); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("List = %v, want internal", err)
	}
	if _, err := c.Vault().List(t.Context(), &api.VaultListParams{}); err != nil {
		t.Fatalf("the connection after a panic: %v", err)
	}
}
