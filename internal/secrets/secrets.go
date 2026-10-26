// Package secrets keeps device tokens out of the database and the vault
// (ADR-0016, docs/specs/sync-protocol.md): the Secret Service over D-Bus,
// and a plain File for development and CI. Ported from frostmail's
// internal/secrets (frostmail ADR-0011).
package secrets

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound means no secret is stored under the key.
var ErrNotFound = errors.New("secrets: not found")

// Store keeps string secrets by key.
type Store interface {
	// Get returns the secret, or ErrNotFound.
	Get(ctx context.Context, key string) (string, error)
	// Set stores the secret, replacing any previous value.
	Set(ctx context.Context, key, value string) error
	// Delete removes the secret; a missing key is not an error.
	Delete(ctx context.Context, key string) error
}

// HubToken is the key of the device token for the hub with ID hubID.
func HubToken(hubID string) string { return "hub/" + hubID + "/token" }

// ProbeKey is the key a pairing writes and deletes first: a store that
// cannot keep it cannot keep a device token either, and the store's prompt
// then comes before the hub knows the device.
const ProbeKey = "probe"

// File is a Store kept in one JSON object ({"key": "value", ...}) in a file
// with mode 0600: plain text on disk, guarded only by file permissions, for
// development, CI and machines without a Secret Service.
type File struct {
	mu   sync.Mutex
	path string
}

var _ Store = (*File)(nil)

// NewFile returns a File store at path. The file is created on first Set;
// its directory must exist.
func NewFile(path string) *File { return &File{path: path} }

// Get returns the secret stored under key, or ErrNotFound. A missing file
// holds no secrets. A file whose mode allows group or other access is refused
// with an error naming the file and its mode.
func (f *File) Get(ctx context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return "", err
	}
	value, ok := m[key]
	if !ok {
		return "", fmt.Errorf("secrets: get %s: %w", key, ErrNotFound)
	}
	return value, nil
}

// Set stores value under key. It rewrites the whole file atomically: write a
// temporary file in the same directory with mode 0600, then rename it over
// the old file.
func (f *File) Set(ctx context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return err
	}
	m[key] = value
	return f.write(m)
}

// Delete removes key, rewriting the file atomically like Set. A missing key
// or a missing file is not an error.
func (f *File) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	m, err := f.read()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	return f.write(m)
}

// read loads the whole map. A missing file is an empty map; a file readable
// by group or other, or one that is not a JSON object of strings,
// is an error.
func (f *File) read() (map[string]string, error) {
	fi, err := os.Stat(f.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("secrets: stat %s: %w", f.path, err)
	}
	if err := checkMode(f.path, fi); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("secrets: read %s: %w", f.path, err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("secrets: decode %s: %w", f.path, err)
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}

// write replaces the file with data atomically: a temporary file in the
// target's directory, mode 0600, synced, then renamed over the target. The
// temporary file is removed on any error.
func (f *File) write(m map[string]string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("secrets: encode %s: %w", f.path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), ".secrets-*")
	if err != nil {
		return fmt.Errorf("secrets: create temp in %s: %w", filepath.Dir(f.path), err)
	}
	if err := syncTemp(tmp, data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("secrets: close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("secrets: rename %s over %s: %w", tmp.Name(), f.path, err)
	}
	return nil
}

// syncTemp writes data to tmp and flushes it with mode 0600.
func syncTemp(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("secrets: write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secrets: chmod %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("secrets: sync %s: %w", tmp.Name(), err)
	}
	return nil
}

// All returns every stored secret (for moving them to another store).
func (f *File) All() (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.read()
}

// Remove deletes the file; a missing file is not an error.
func (f *File) Remove() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("secrets: remove %s: %w", f.path, err)
	}
	return nil
}

// migrate moves every secret from the file store into another store,
// reading each back before the file is removed.
func migrate(ctx context.Context, from *File, to Store) error {
	all, err := from.All()
	if err != nil || len(all) == 0 {
		return err
	}
	for key, value := range all {
		if err := to.Set(ctx, key, value); err != nil {
			return fmt.Errorf("secrets: move %s: %w", key, err)
		}
		got, err := to.Get(ctx, key)
		if err != nil || got != value {
			return fmt.Errorf("secrets: %s did not read back: %v", key, err)
		}
	}
	return from.Remove()
}
