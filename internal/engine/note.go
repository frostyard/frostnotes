package engine

import (
	"context"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// notes implements the note domain. Plan 0003's Phase 4 builds it over
// internal/vault; until then every method answers unavailable.
type notes struct{ db *store.DB }

// notYet is the answer of a method Phase 4 has not built.
func notYet(method string) error {
	return api.Unavailable("%s is not implemented yet (plan 0003, Phase 4)", method)
}

func (notes) List(context.Context, *api.NoteListParams) ([]api.NoteEntry, error) {
	return nil, notYet("note.list")
}

func (notes) Read(context.Context, *api.NoteReadParams) (*api.Note, error) {
	return nil, notYet("note.read")
}

func (notes) Search(context.Context, *api.NoteSearchParams) ([]api.SearchHit, error) {
	return nil, notYet("note.search")
}

func (notes) Backlinks(context.Context, *api.NoteBacklinksParams) ([]api.Backlink, error) {
	return nil, notYet("note.backlinks")
}

func (notes) Links(context.Context, *api.NoteLinksParams) ([]api.Link, error) {
	return nil, notYet("note.links")
}

func (notes) Create(context.Context, *api.NoteCreateParams) (*api.WriteResult, error) {
	return nil, notYet("note.create")
}

func (notes) Write(context.Context, *api.NoteWriteParams) (*api.WriteResult, error) {
	return nil, notYet("note.write")
}

func (notes) Move(context.Context, *api.NoteMoveParams) (*api.MoveResult, error) {
	return nil, notYet("note.move")
}

func (notes) Delete(context.Context, *api.NoteDeleteParams) error {
	return notYet("note.delete")
}

func (notes) Versions(context.Context, *api.NoteVersionsParams) ([]api.Version, error) {
	return nil, notYet("note.versions")
}

func (notes) Revert(context.Context, *api.NoteRevertParams) (*api.WriteResult, error) {
	return nil, notYet("note.revert")
}

func (notes) Daily(context.Context, *api.NoteDailyParams) (*api.DailyNote, error) {
	return nil, notYet("note.daily")
}
