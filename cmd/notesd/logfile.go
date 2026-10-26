package main

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// logLimit is the size over which notesd's log moves to its one old copy.
const logLimit = 1 << 20 // 1 MiB

// logTo keeps this process's output in path: an old path moves to path.1,
// path is opened for appending, and stdout and stderr are pointed at it, so
// that slog (it writes to os.Stderr) and a panic both land in the file. It
// runs only once this process holds the data directory's lock: a second
// notesd on the same directory must never move the log of the one that
// runs.
func logTo(path string) error {
	f, err := openLogFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // fds 1 and 2 hold the file open
	for _, fd := range []int{1, 2} {
		if err := syscall.Dup3(int(f.Fd()), fd, 0); err != nil {
			return err
		}
	}
	return nil
}

// openLogFile moves an oversized path to path.1 and opens path for
// appending, making it with mode 0600 when it does not exist.
func openLogFile(path string) (*os.File, error) {
	if err := rotateLog(path); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
}

// rotateLog moves path to path.1, replacing an old path.1, when path holds
// more than 1 MiB. A smaller path stays, and a missing one is nothing to
// rotate.
func rotateLog(path string) error {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Size() <= logLimit {
		return nil
	}
	return os.Rename(path, path+".1")
}
