// Package config resolves where notesd keeps its files (docs/design/overview.md#operational-notes).
package config

import "path/filepath"

// AppID is the reverse-DNS application ID (frostyard/core ADR-0016).
const AppID = "org.frostyard.Frostnotes"

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
