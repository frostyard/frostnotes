package engine_test

import (
	"errors"
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
