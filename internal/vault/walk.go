// Package vault scans, watches and writes vault directories (docs/design/vault.md).
package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SettingsDir is the settings directory directly under a vault's root.
const SettingsDir = ".frostnotes"

// Kind identifies a vault file's role.
type Kind int

const (
	// KindNote is a file whose name ends in the case-sensitive suffix ".md".
	KindNote Kind = iota + 1
	// KindAttachment is any other regular file outside the settings directory.
	KindAttachment
	// KindSettings is a regular file under SettingsDir.
	KindSettings
)

// Entry describes a regular file in a vault.
type Entry struct {
	Path    string // Slash-separated, relative to the root.
	Kind    Kind
	Size    int64
	ModTime time.Time // Unrounded FileInfo modification time.
}

// Skip records an entry that cannot be treated as a vault file.
type Skip struct {
	Path   string
	Reason string // "symlink" or "not a regular file".
}

// Listing contains a complete walk, with non-nil slices sorted by Path in byte order.
type Listing struct {
	Files   []Entry
	Skipped []Skip
	// Dirs holds every folder walked, SettingsDir included and the root
	// left out: the folders a watcher watches.
	Dirs []string
	// Temps holds the temporary files of atomic writes a crash left
	// behind, which a scan removes.
	Temps []string
}

// Walk lists a vault's files without following symlinks. Any error discards the listing.
func Walk(ctx context.Context, root string, ignore *Ignore) (Listing, error) {
	walkRoot := root
	if root != "" {
		walkRoot = filepath.Clean(root)
	}
	info, err := os.Lstat(walkRoot)
	if err != nil {
		return Listing{}, fmt.Errorf("walk vault %s: %w", root, err)
	}
	if !info.IsDir() {
		return Listing{}, fmt.Errorf("walk vault %s: %w", root, errors.New("root is not a directory"))
	}
	listing := Listing{Files: []Entry{}, Skipped: []Skip{}, Dirs: []string{}, Temps: []string{}}
	err = filepath.WalkDir(walkRoot, func(name string, d fs.DirEntry, walkErr error) error {
		return listing.visit(ctx, walkRoot, name, d, walkErr, ignore)
	})
	if err != nil {
		return Listing{}, fmt.Errorf("walk vault %s: %w", root, err)
	}
	slices.SortFunc(listing.Files, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(listing.Skipped, func(a, b Skip) int { return strings.Compare(a.Path, b.Path) })
	slices.Sort(listing.Dirs)
	slices.Sort(listing.Temps)
	return listing, nil
}

func (l *Listing) visit(ctx context.Context, root, name string, d fs.DirEntry, walkErr error, ignore *Ignore) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// WalkDir reports a directory read failure on a second visit; never hide it.
	if walkErr != nil {
		return walkErr
	}
	if name == root {
		return nil
	}
	rel, err := filepath.Rel(root, name)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	settingsRoot := rel == SettingsDir && d.IsDir()
	settings := settingsRoot || strings.HasPrefix(rel, SettingsDir+"/")
	hidden := strings.HasPrefix(d.Name(), ".") && !settingsRoot
	if hidden && d.Type().IsRegular() && isTemp(d.Name()) {
		l.Temps = append(l.Temps, rel)
	}
	if hidden || (!settings && ignore.Match(rel, d.IsDir())) {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if d.Type()&fs.ModeSymlink != 0 {
		l.Skipped = append(l.Skipped, Skip{Path: rel, Reason: "symlink"})
		return nil
	}
	if d.IsDir() {
		l.Dirs = append(l.Dirs, rel)
		return nil
	}
	info, err := d.Info()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		l.Skipped = append(l.Skipped, Skip{Path: rel, Reason: "not a regular file"})
		return nil
	}
	kind := KindAttachment
	if settings {
		kind = KindSettings
	} else if strings.HasSuffix(rel, ".md") {
		kind = KindNote
	}
	l.Files = append(l.Files, Entry{Path: rel, Kind: kind, Size: info.Size(), ModTime: info.ModTime()})
	return nil
}
