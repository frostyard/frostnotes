package system

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
)

// WindowLifetime keeps the daemon available to every open Flatpak window,
// including windows using the other frontend. Only the last process applies
// the background preference. Losing a window to a crash releases its lock.
func WindowLifetime(getenv Getenv) (func(), error) {
	if !InFlatpak(getenv) {
		return func() {}, nil
	}
	dir, err := StateDir(getenv)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "windows.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		defer file.Close()
		if syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			StopOnExit(context.Background(), getenv)
		}
	}, nil
}
