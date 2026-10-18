package engine

import (
	"context"

	"github.com/frostyard/frostnotes/api"
)

// The writes M3 adds to the note and vault domains
// (docs/plans/0005-m3-desktop-app.md, Phase 3) answer unavailable until
// Phase 4's second part builds them.

func notBuilt(method string) error {
	return api.Unavailable("%s is not built yet", method)
}

func (notes) Upload(context.Context, *api.NoteUploadParams) (*api.WriteResult, error) {
	return nil, notBuilt("note.upload")
}

func (notes) Template(context.Context, *api.NoteTemplateParams) (*api.ExpandedTemplate, error) {
	return nil, notBuilt("note.template")
}

func (notes) Mkdir(context.Context, *api.NoteMkdirParams) error {
	return notBuilt("note.mkdir")
}

func (notes) MoveFolder(context.Context, *api.NoteMoveFolderParams) (*api.FolderMoveResult, error) {
	return nil, notBuilt("note.moveFolder")
}

func (notes) DeleteFolder(context.Context, *api.NoteDeleteFolderParams) (*api.FolderDeleteResult, error) {
	return nil, notBuilt("note.deleteFolder")
}

func (vaults) Configure(context.Context, *api.VaultConfigureParams) (*api.VaultSettings, error) {
	return nil, notBuilt("vault.configure")
}
