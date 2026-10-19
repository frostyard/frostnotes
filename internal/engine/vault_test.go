package engine_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

func TestVaultAddResolvesAndNames(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	dir := t.TempDir()
	real := filepath.Join(dir, "Notes")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	v, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: link})
	if err != nil {
		t.Fatal(err)
	}
	if v.Path != real || v.Name != "Notes" {
		t.Fatalf("Add(%s) = %+v; want path %s, name Notes", link, v, real)
	}
	if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: real}); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("the same directory again = %v, want conflict", err)
	}
	list, err := c.Vault().List(ctx, &api.VaultListParams{})
	if err != nil || len(list) != 1 || list[0] != *v {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if err := c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: v.ID}); err != nil {
		t.Fatal(err)
	}
	if err := c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: v.ID}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("second remove = %v, want notFound", err)
	}
}

func TestVaultAddRejects(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	dir := t.TempDir()
	outer := filepath.Join(dir, "outer")
	inner := filepath.Join(outer, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "file.md")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: outer}); err != nil {
		t.Fatal(err)
	}
	blank := " "
	for name, tc := range map[string]struct {
		p    api.VaultAddParams
		want error
	}{
		"relative path":  {api.VaultAddParams{Path: "notes"}, api.ErrInvalidParams},
		"missing":        {api.VaultAddParams{Path: filepath.Join(dir, "nope")}, api.ErrInvalidParams},
		"a file":         {api.VaultAddParams{Path: file}, api.ErrInvalidParams},
		"blank name":     {api.VaultAddParams{Path: dir, Name: &blank}, api.ErrInvalidParams},
		"inside a vault": {api.VaultAddParams{Path: inner}, api.ErrConflict},
		"contains vault": {api.VaultAddParams{Path: dir}, api.ErrConflict},
	} {
		if _, err := c.Vault().Add(ctx, &tc.p); !errors.Is(err, tc.want) {
			t.Errorf("%s: Add = %v, want %v", name, err, tc.want)
		}
	}
}

// Create new vault (ADR-0024): notesd makes the folder, and only when the
// vault may be registered; removing the vault leaves the folder.
func TestVaultAddCreates(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	dir := t.TempDir()
	yes := true
	made := filepath.Join(dir, "Garden")
	v, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: made, Create: &yes})
	if err != nil || v.Path != made || v.Name != "Garden" {
		t.Fatalf("Add(create %s) = %+v, %v", made, v, err)
	}
	if fi, err := os.Stat(made); err != nil || !fi.IsDir() {
		t.Fatalf("the vault's folder: %v, %v", fi, err)
	}
	file := filepath.Join(dir, "file.md")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "Other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		path string
		want error
	}{
		"a vault already":      {made, api.ErrConflict},
		"an existing folder":   {other, api.ErrConflict},
		"a file":               {file, api.ErrConflict},
		"inside a vault":       {filepath.Join(made, "Inner"), api.ErrConflict},
		"no parent":            {filepath.Join(dir, "missing", "Vault"), api.ErrInvalidParams},
		"a file as the parent": {filepath.Join(file, "Vault"), api.ErrInvalidParams},
		"the root":             {string(filepath.Separator), api.ErrInvalidParams},
	} {
		if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: tc.path, Create: &yes}); !errors.Is(err, tc.want) {
			t.Errorf("%s: Add(create %s) = %v, want %v", name, tc.path, err, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(made, "Inner")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a refused vault's folder was made: %v", err)
	}
	if err := c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: v.ID}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(made); err != nil || !fi.IsDir() {
		t.Errorf("after vault.remove the folder is %v, %v; want it kept", fi, err)
	}
}
