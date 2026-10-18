package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/render"
	"github.com/frostyard/frostnotes/internal/store"
)

// Folders returns every folder below the vault's root that a scan walks,
// empty ones included, by path: hidden folders, the settings directory
// and ignored folders are left out. Unlike Walk it reads no file's
// information, so on a network share it costs one listing per folder.
func (v *Vault) Folders(ctx context.Context) ([]string, error) {
	v.mu.Lock()
	ignore := v.patterns()
	v.mu.Unlock()
	root := filepath.Clean(v.root)
	folders := []string{}
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == root || !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(d.Name(), ".") || ignore.Match(rel, true) {
			return filepath.SkipDir
		}
		folders = append(folders, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the folders of vault %d: %w", v.id, err)
	}
	slices.Sort(folders)
	return folders, nil
}

// Render renders note, the content of the note at p or what an editor
// holds for it, to sanitized HTML (docs/specs/render.md). Links resolve
// over the index, and embedded notes and files are what the index holds.
func (v *Vault) Render(ctx context.Context, p string, note []byte) (string, error) {
	if err := checkNotePath(p); err != nil {
		return "", err
	}
	if kindOf(p) != store.FileNote {
		return "", api.InvalidParams("%s is not a note: only notes render", p)
	}
	lookup := v.lookup(ctx)
	file := func(p string) (store.File, bool) {
		f, err := v.cfg.DB.FileByPath(ctx, v.id, p)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			v.log.Warn("render: look up a file", "path", p, "err", err)
		}
		return f, err == nil
	}
	return render.Render(note, render.Options{
		Path:    p,
		Resolve: func(from, target string) string { return markdown.Resolve(from, target, lookup) },
		File: func(p string) (string, bool) {
			f, ok := file(p)
			return f.Hash, ok
		},
		Note: func(p string) ([]byte, bool) {
			f, ok := file(p)
			if !ok || f.Kind() != store.FileNote {
				return nil, false
			}
			data, err := v.cfg.Blobs.Get(f.Hash)
			if err != nil {
				v.log.Warn("render: read an embedded note", "path", p, "err", err)
				return nil, false
			}
			return []byte(showText(data)), true
		},
	}), nil
}

// ReadVersion returns one of the vault's versions and its content, as
// Read shows a file's.
func (v *Vault) ReadVersion(ctx context.Context, id int64) (*api.VersionContent, error) {
	ver, err := v.cfg.DB.GetVersion(ctx, id)
	if errors.Is(err, store.ErrNotFound) || err == nil && ver.VaultID != v.id {
		return nil, api.NotFound("version %d does not exist in vault %d", id, v.id)
	}
	if err != nil {
		return nil, err
	}
	out := &api.VersionContent{Version: ver.API()}
	if ver.Hash == "" {
		return out, nil // a deletion
	}
	if ver.Size > bigFile {
		return nil, api.InvalidParams("version %d is %d bytes; the API reads files of at most %d", id, ver.Size, bigFile)
	}
	data, err := v.cfg.Blobs.Get(ver.Hash)
	if err != nil {
		return nil, fmt.Errorf("read version %d: %w", id, err)
	}
	out.Content = showText(data)
	return out, nil
}
