//go:build !windows

package rpctest

import (
	"os"
	"path/filepath"
)

// SocketPath is a new socket path for a test notesd, in a short private
// directory because Unix socket paths are limited to 108 bytes; cleanup
// removes the directory.
func SocketPath() (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "fn")
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(dir, "notesd.sock"), func() { _ = os.RemoveAll(dir) }, nil
}
