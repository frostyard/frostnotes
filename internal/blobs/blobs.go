// Package blobs stores immutable file contents by SHA-256 under
// <dir>/<first two hex digits>/<id> (docs/design/vault.md).
package blobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrInvalidID rejects IDs that are not exactly 64 lowercase hex digits.
var ErrInvalidID = errors.New("blobs: invalid id")

// ErrCorrupt indicates that stored content does not match its ID.
var ErrCorrupt = errors.New("blobs: content does not match its id")

// Store is a directory of immutable blobs. It is safe for concurrent use.
type Store struct{ dir string }

// New returns a Store rooted at dir without creating any files or directories.
func New(dir string) *Store { return &Store{dir: dir} }

// ID returns the lowercase hex SHA-256 of data.
func ID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IDReader returns the lowercase hex SHA-256 of everything r yields: ID
// for content too big to hold in memory.
func IDReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validID(id string) bool {
	if len(id) != sha256.Size*2 {
		return false
	}
	for i := range len(id) {
		if c := id[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (s *Store) path(id string) (string, error) {
	if !validID(id) {
		return "", ErrInvalidID
	}
	return filepath.Join(s.dir, id[:2], id), nil
}

// Put streams r into a temporary file, then atomically publishes the blob.
// Existing blobs are kept. Directories are created with mode 0700 and blobs
// with mode 0600. Cancellation before publication leaves no temporary file.
func (s *Store) Put(ctx context.Context, r io.Reader) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("blobs: create store dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".put-*")
	if err != nil {
		return "", fmt.Errorf("blobs: create temp: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		return "", fmt.Errorf("blobs: read content: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return "", fmt.Errorf("blobs: chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("blobs: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("blobs: close temp: %w", err)
	}
	id := hex.EncodeToString(h.Sum(nil))
	if err := s.publish(ctx, tmp.Name(), id); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) publish(ctx context.Context, name, id string) error {
	shard := filepath.Join(s.dir, id[:2])
	if err := os.MkdirAll(shard, 0o700); err != nil {
		return fmt.Errorf("blobs: create shard dir: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	final := filepath.Join(shard, id)
	if _, err := os.Stat(final); err == nil {
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("blobs: remove duplicate temp: %w", err)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("blobs: stat blob: %w", err)
	}
	if err := os.Rename(name, final); err != nil {
		return fmt.Errorf("blobs: rename into place: %w", err)
	}
	dir, err := os.Open(shard)
	if err != nil {
		return fmt.Errorf("blobs: open shard dir: %w", err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("blobs: sync shard dir: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("blobs: close shard dir: %w", err)
	}
	return nil
}

// Open opens a blob without verifying its contents. A missing blob returns
// an error matching fs.ErrNotExist; a malformed ID returns ErrInvalidID.
func (s *Store) Open(id string) (io.ReadCloser, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("blobs: open %s: %w", id, err)
	}
	return f, nil
}

// Get reads a blob and verifies its ID, returning nil and an error matching
// ErrCorrupt if its content has changed. Missing blobs match fs.ErrNotExist.
func (s *Store) Get(id string) ([]byte, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("blobs: read %s: %w", id, err)
	}
	if ID(data) != id {
		return nil, fmt.Errorf("blobs: verify %s: %w", id, ErrCorrupt)
	}
	return data, nil
}

// Has reports whether a blob exists; malformed IDs return ErrInvalidID.
func (s *Store) Has(id string) (bool, error) {
	p, err := s.path(id)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("blobs: stat %s: %w", id, err)
	}
	return true, nil
}

// Remove deletes a blob. Missing blobs are ignored; malformed IDs return
// ErrInvalidID.
func (s *Store) Remove(id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("blobs: remove %s: %w", id, err)
	}
	return nil
}

// IDs returns all regularly stored blob IDs in ascending order, ignoring
// stray files. Empty and missing stores return a non-nil empty slice.
func (s *Store) IDs(ctx context.Context) ([]string, error) {
	ids := []string{}
	shards, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return ids, nil
	}
	if err != nil {
		return nil, fmt.Errorf("blobs: list store: %w", err)
	}
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(filepath.Join(s.dir, shard.Name()))
		if err != nil {
			return nil, fmt.Errorf("blobs: list shard %s: %w", shard.Name(), err)
		}
		for _, entry := range entries {
			id := entry.Name()
			if validID(id) && id[:2] == shard.Name() && entry.Type().IsRegular() {
				ids = append(ids, id)
			}
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// RemoveTemp removes regular .put-* files directly in the store whose
// modification time precedes before, returning the number removed.
func (s *Store) RemoveTemp(before time.Time) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("blobs: list temporary files: %w", err)
	}
	n := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".put-") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, fmt.Errorf("blobs: stat temp %s: %w", entry.Name(), err)
		}
		if !info.ModTime().Before(before) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return n, fmt.Errorf("blobs: remove temp %s: %w", entry.Name(), err)
		}
		n++
	}
	return n, nil
}
