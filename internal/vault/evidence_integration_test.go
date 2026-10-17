//go:build integration

package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestIColdIndexOfObsidianVault is M1's exit evidence 1 (plan 0003, Phase
// 7): the vault FROSTNOTES_OBSIDIAN_VAULT names indexes from an empty data
// directory in under 5 s, with its files evicted from the page cache
// first. The vault is only read: its files' sizes and times are compared
// before and after. Run it with make m1-evidence.
func TestIColdIndexOfObsidianVault(t *testing.T) {
	root := os.Getenv("FROSTNOTES_OBSIDIAN_VAULT")
	if root == "" {
		t.Skip("set FROSTNOTES_OBSIDIAN_VAULT")
	}
	before := stats(t, root)
	evict(t, root)
	tv := openTestVault(t, root, t.TempDir(), nil)
	start := time.Now()
	tv.scan()
	took := time.Since(start)
	notes, attachments, err := tv.db.FileCounts(t.Context(), tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := tv.v.Status(t.Context())
	t.Logf("indexed %d notes and %d attachments cold in %v (network file system: %v)", notes, attachments, took, s.Network)
	if took > 5*time.Second {
		t.Errorf("the cold index took %v, more than 5 s", took)
	}
	if notes == 0 {
		t.Error("the vault has no notes")
	}
	if after := stats(t, root); !slices.Equal(before, after) {
		t.Error("indexing changed the vault's files")
	}
}

// stats lists every file and folder under root, hidden ones included, with
// its size and modification time.
func stats(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("%s %d %d", p, fi.Size(), fi.ModTime().UnixNano()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// evict drops root's files from the page cache, so the index reads them
// from the file system: on a network share, from the server.
func evict(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() || strings.Contains(p, "/.obsidian/") {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
	})
	if err != nil {
		t.Fatal(err)
	}
}
