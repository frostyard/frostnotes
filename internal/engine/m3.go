package engine

import (
	"context"

	"github.com/frostyard/frostnotes/api"
)

// The methods M3 adds to the note and vault domains
// (docs/plans/0005-m3-desktop-app.md, Phase 3) answer unavailable until
// Phase 4 builds them.

func notBuilt(method string) error {
	return api.Unavailable("%s is not built yet", method)
}

func (notes) Files(context.Context, *api.NoteFilesParams) (*api.VaultFiles, error) {
	return nil, notBuilt("note.files")
}

func (notes) Resolve(context.Context, *api.NoteResolveParams) ([]api.Resolution, error) {
	return nil, notBuilt("note.resolve")
}

func (notes) Render(context.Context, *api.NoteRenderParams) (*api.RenderedNote, error) {
	return nil, notBuilt("note.render")
}

func (notes) Upload(context.Context, *api.NoteUploadParams) (*api.WriteResult, error) {
	return nil, notBuilt("note.upload")
}

func (notes) ReadVersion(context.Context, *api.NoteReadVersionParams) (*api.VersionContent, error) {
	return nil, notBuilt("note.readVersion")
}

func (notes) Tags(context.Context, *api.NoteTagsParams) ([]api.TagCount, error) {
	return nil, notBuilt("note.tags")
}

func (notes) Properties(context.Context, *api.NotePropertiesParams) ([]api.PropertyCount, error) {
	return nil, notBuilt("note.properties")
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

func (vaults) Settings(context.Context, *api.VaultSettingsParams) (*api.VaultSettings, error) {
	return nil, notBuilt("vault.settings")
}

func (vaults) Configure(context.Context, *api.VaultConfigureParams) (*api.VaultSettings, error) {
	return nil, notBuilt("vault.configure")
}
