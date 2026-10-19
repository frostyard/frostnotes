package engine

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

// notes implements the note domain: queries read the index; reads and
// writes go through the vault's actor, which owns its files.
type notes struct {
	db *store.DB
	m  *vault.Manager
}

// author is who makes a write. A session's or an agent's connection fixes
// it, whatever the params say (ADR-0026); otherwise it is source editor
// unless the writer says agent, and the client name given to rpc.hello
// unless the writer names one.
func author(ctx context.Context, source *api.VersionSource, name *string) (vault.Author, error) {
	if src, who, ok := api.IdentityFrom(ctx).Author(); ok {
		return vault.Author{Source: src, Name: who}, nil
	}
	a := vault.Author{Source: api.VersionSourceEditor}
	if source != nil {
		if *source != api.VersionSourceEditor && *source != api.VersionSourceAgent {
			return vault.Author{}, api.InvalidParams("source %q: a write's source is editor or agent", *source)
		}
		a.Source = *source
	}
	if name != nil {
		a.Name = strings.TrimSpace(*name)
	} else if c := api.ConnFrom(ctx); c != nil {
		a.Name = c.Client()
	}
	return a, nil
}

// file returns vault's file at p from the index.
func (n notes) file(ctx context.Context, vaultID int64, p string) (store.File, error) {
	if _, err := n.m.Vault(vaultID); err != nil {
		return store.File{}, err
	}
	if err := vault.CheckPath(p); err != nil {
		return store.File{}, api.InvalidParams("%v", err)
	}
	f, err := n.db.FileByPath(ctx, vaultID, p)
	return f, apiError(err, p)
}

func (n notes) List(ctx context.Context, p *api.NoteListParams) ([]api.NoteEntry, error) {
	if _, err := n.m.Vault(p.VaultID); err != nil {
		return nil, err
	}
	var f store.NoteFilter
	if p.Folder != nil {
		folder, err := vault.CheckFolder(*p.Folder)
		if err != nil {
			return nil, api.InvalidParams("%v", err)
		}
		f.Folder = folder
	}
	if p.Tag != nil {
		f.Tag = *p.Tag
	}
	if p.Property != nil {
		f.Property = *p.Property
	}
	if p.Value != nil {
		if p.Property == nil {
			return nil, api.InvalidParams("value needs property")
		}
		f.Value = *p.Value
	}
	return n.db.ListNotes(ctx, p.VaultID, f)
}

func (n notes) Read(ctx context.Context, p *api.NoteReadParams) (*api.Note, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	return v.Read(ctx, p.Path)
}

func (n notes) Search(ctx context.Context, p *api.NoteSearchParams) ([]api.SearchHit, error) {
	if _, err := n.m.Vault(p.VaultID); err != nil {
		return nil, err
	}
	limit := int64(20)
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 100 {
		return nil, api.InvalidParams("limit %d: at least 1 and at most 100", limit)
	}
	return n.db.SearchNotes(ctx, p.VaultID, store.ParseQuery(p.Query), int(limit))
}

func (n notes) Backlinks(ctx context.Context, p *api.NoteBacklinksParams) ([]api.Backlink, error) {
	f, err := n.file(ctx, p.VaultID, p.Path)
	if err != nil {
		return nil, err
	}
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	return v.Backlinks(ctx, f.ID)
}

func (n notes) Links(ctx context.Context, p *api.NoteLinksParams) ([]api.Link, error) {
	f, err := n.file(ctx, p.VaultID, p.Path)
	if err != nil {
		return nil, err
	}
	if f.Kind() != store.FileNote {
		return []api.Link{}, nil // an attachment links nowhere
	}
	return n.db.Links(ctx, f.ID)
}

func (n notes) Create(ctx context.Context, p *api.NoteCreateParams) (*api.WriteResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, p.Source, p.Author)
	if err != nil {
		return nil, err
	}
	return v.Create(ctx, p.Path, []byte(p.Content), who)
}

func (n notes) Write(ctx context.Context, p *api.NoteWriteParams) (*api.WriteResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, p.Source, p.Author)
	if err != nil {
		return nil, err
	}
	return v.Write(ctx, p.Path, p.Base, []byte(p.Content), who)
}

func (n notes) Move(ctx context.Context, p *api.NoteMoveParams) (*api.MoveResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	return v.Move(ctx, p.Path, p.To, p.UpdateLinks != nil && *p.UpdateLinks, who)
}

func (n notes) Delete(ctx context.Context, p *api.NoteDeleteParams) error {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return err
	}
	who, err := author(ctx, nil, nil)
	if err != nil {
		return err
	}
	return v.Delete(ctx, p.Path, p.Base, who)
}

func (n notes) Versions(ctx context.Context, p *api.NoteVersionsParams) ([]api.Version, error) {
	if _, err := n.m.Vault(p.VaultID); err != nil {
		return nil, err
	}
	if err := vault.CheckPath(p.Path); err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	id := int64(0)
	f, err := n.db.FileByPath(ctx, p.VaultID, p.Path)
	switch {
	case err == nil:
		id = f.ID
	case errors.Is(err, store.ErrNotFound):
		// A deleted file: the last file at that path.
		if id, err = n.db.LastFileAt(ctx, p.VaultID, p.Path); err != nil {
			return nil, apiError(err, p.Path)
		}
	default:
		return nil, err
	}
	limit := 0
	if p.Limit != nil {
		limit = int(*p.Limit)
	}
	list, err := n.db.Versions(ctx, id, limit)
	if err != nil {
		return nil, err
	}
	out := make([]api.Version, 0, len(list))
	for _, v := range list {
		out = append(out, v.API())
	}
	return out, nil
}

func (n notes) Revert(ctx context.Context, p *api.NoteRevertParams) (*api.WriteResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	return v.Revert(ctx, p.Path, p.Version, p.Base, who)
}

func (n notes) Daily(ctx context.Context, p *api.NoteDailyParams) (*api.DailyNote, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	date := time.Now()
	if p.Date != nil {
		if date, err = time.ParseInLocation(time.DateOnly, *p.Date, time.Local); err != nil {
			return nil, api.InvalidParams("date %q is not YYYY-MM-DD", *p.Date)
		}
	}
	who, err := author(ctx, p.Source, p.Author)
	if err != nil {
		return nil, err
	}
	return v.Daily(ctx, date, who)
}

// Files lists the index's files and the vault's folders, with every
// folder a listed file lies in, so the listing holds together while the
// disk changes under it.
func (n notes) Files(ctx context.Context, p *api.NoteFilesParams) (*api.VaultFiles, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	files, err := n.db.Files(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	folders, err := v.Folders(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(folders))
	for _, f := range folders {
		seen[f] = true
	}
	out := &api.VaultFiles{Files: make([]api.VaultFile, 0, len(files))}
	for _, f := range files {
		out.Files = append(out.Files, api.VaultFile{Path: f.Path, Kind: api.FileKind(f.Kind()),
			Hash: f.Hash, Size: f.Size, ModifiedAt: f.ModTime})
		for dir := path.Dir(f.Path); dir != "." && !seen[dir]; dir = path.Dir(dir) {
			seen[dir] = true
			folders = append(folders, dir)
		}
	}
	slices.Sort(folders)
	out.Folders = folders
	return out, nil
}

// maxResolve is the most targets one note.resolve takes.
const maxResolve = 1000

func (n notes) Resolve(ctx context.Context, p *api.NoteResolveParams) ([]api.Resolution, error) {
	if _, err := n.m.Vault(p.VaultID); err != nil {
		return nil, err
	}
	if err := vault.CheckPath(p.From); err != nil {
		return nil, api.InvalidParams("from: %v", err)
	}
	if len(p.Targets) > maxResolve {
		return nil, api.InvalidParams("%d targets: at most %d", len(p.Targets), maxResolve)
	}
	var lookupErr error
	files := func(name string) []string {
		paths, err := n.db.FilesNamed(ctx, p.VaultID, name)
		lookupErr = cmp.Or(lookupErr, err)
		return paths
	}
	out := make([]api.Resolution, 0, len(p.Targets))
	for _, target := range p.Targets {
		r := api.Resolution{Target: target}
		if to := markdown.Resolve(p.From, target, files); to != "" {
			r.Path = &to
		}
		out = append(out, r)
	}
	if lookupErr != nil {
		return nil, lookupErr
	}
	return out, nil
}

func (n notes) Render(ctx context.Context, p *api.NoteRenderParams) (*api.RenderedNote, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	var note string
	if p.Content != nil {
		note = *p.Content
	} else {
		got, err := v.Read(ctx, p.Path)
		if err != nil {
			return nil, err
		}
		note = got.Content
	}
	html, err := v.Render(ctx, p.Path, []byte(note))
	if err != nil {
		return nil, err
	}
	return &api.RenderedNote{HTML: html}, nil
}

func (n notes) ReadVersion(ctx context.Context, p *api.NoteReadVersionParams) (*api.VersionContent, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	return v.ReadVersion(ctx, p.Version)
}

func (n notes) Tags(ctx context.Context, p *api.NoteTagsParams) ([]api.TagCount, error) {
	if _, err := n.m.Vault(p.VaultID); err != nil {
		return nil, err
	}
	return n.db.TagCounts(ctx, p.VaultID)
}

func (n notes) Properties(ctx context.Context, p *api.NotePropertiesParams) ([]api.PropertyCount, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	types, err := vault.LoadTypes(v.Root())
	if err != nil {
		return nil, fmt.Errorf("vault %d's property types: %w", p.VaultID, err)
	}
	return n.db.PropertyCounts(ctx, p.VaultID, types)
}

// maxUpload is the most bytes note.upload takes (ADR-0023).
const maxUpload = 10 << 20

func (n notes) Upload(ctx context.Context, p *api.NoteUploadParams) (*api.WriteResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, p.Source, p.Author)
	if err != nil {
		return nil, err
	}
	if len(p.Data) > base64.StdEncoding.EncodedLen(maxUpload) {
		return nil, api.InvalidParams("an upload is at most %d bytes: copy a larger file into the vault's folder, where the scan finds it", maxUpload)
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, api.InvalidParams("data is not base64: %v", err)
	}
	if len(data) > maxUpload {
		return nil, api.InvalidParams("an upload is at most %d bytes: copy a larger file into the vault's folder, where the scan finds it", maxUpload)
	}
	return v.Create(ctx, p.Path, data, who)
}

func (n notes) Template(ctx context.Context, p *api.NoteTemplateParams) (*api.ExpandedTemplate, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	title, at := "", time.Now()
	if p.Title != nil {
		title = *p.Title
	}
	if p.At != nil {
		at = *p.At
	}
	content, err := v.Template(ctx, p.Template, title, at)
	if err != nil {
		return nil, err
	}
	return &api.ExpandedTemplate{Content: content}, nil
}

func (n notes) Mkdir(ctx context.Context, p *api.NoteMkdirParams) error {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return err
	}
	return v.Mkdir(ctx, p.Path)
}

func (n notes) MoveFolder(ctx context.Context, p *api.NoteMoveFolderParams) (*api.FolderMoveResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	return v.MoveFolder(ctx, p.Path, p.To, p.UpdateLinks != nil && *p.UpdateLinks, who)
}

func (n notes) DeleteFolder(ctx context.Context, p *api.NoteDeleteFolderParams) (*api.FolderDeleteResult, error) {
	v, err := n.m.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	who, err := author(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	return v.DeleteFolder(ctx, p.Path, p.Files, who)
}
