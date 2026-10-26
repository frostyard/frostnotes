package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockDataDir takes an exclusive lock on dir/notesd.lock and returns the
// open file; the lock lasts until the file is closed or the process ends,
// even by a crash. A second notesd on the same data directory fails here
// at once, as the socket's "already listening" check cannot stop one that
// uses another socket, such as one in another Flatpak sandbox
// (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
func lockDataDir(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "notesd.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("another notesd uses %s", dir)
		}
		return nil, fmt.Errorf("lock %s: %w", dir, err)
	}
	return f, nil
}
