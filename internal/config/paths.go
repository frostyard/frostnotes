// Package config resolves where notesd keeps its files (docs/design/overview.md#operational-notes).
package config

import "path/filepath"

// AppID is the reverse-DNS application ID (frostyard/core ADR-0016).
const AppID = "org.frostyard.Frostnotes"

// LockFile is the file in DataDir that the one notesd of the directory
// holds a lock on (flock), from before it opens the database until it exits.
const LockFile = "notesd.lock"

// InFlatpak reports whether the environment is this application's Flatpak
// sandbox (FLATPAK_ID is AppID): the app's own, not another Flatpak's.
func InFlatpak(getenv func(string) string) bool { return getenv("FLATPAK_ID") == AppID }

// Paths are notesd's file locations. Vaults live wherever the user keeps
// them; nothing here is inside a vault (ADR-0003).
type Paths struct {
	DataDir string // database and blob store; must be on a local filesystem
	Socket  string // the RPC socket
	DB      string // DataDir/notesd.db
	Blobs   string // DataDir/blobs
}

// Resolve computes Paths from the environment: the XDG base directories.
// FROSTNOTES_DATA_DIR and FROSTNOTES_SOCKET override the defaults, for
// tests and for running a second notesd.
func Resolve(getenv func(string) string) (Paths, error) {
	var p Paths
	var err error
	if p.DataDir = getenv("FROSTNOTES_DATA_DIR"); p.DataDir == "" {
		if p.DataDir, err = defaultDataDir(getenv); err != nil {
			return Paths{}, err
		}
	}
	if p.Socket = getenv("FROSTNOTES_SOCKET"); p.Socket == "" {
		if p.Socket, err = DefaultSocket(getenv); err != nil {
			return Paths{}, err
		}
	}
	p.DB = filepath.Join(p.DataDir, "notesd.db")
	p.Blobs = filepath.Join(p.DataDir, "blobs")
	return p, nil
}
