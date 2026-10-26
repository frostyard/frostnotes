package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// notesd's own log moves to one old copy when it holds more than 1 MiB, and
// a new one starts. A smaller log stays, and a missing one is made.
func TestTheLogFileRotatesAtOneMebibyte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notesd.log")
	old := filepath.Join(dir, "notesd.log.1")
	open := func() {
		t.Helper()
		f, err := openLogFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	noOld := func() {
		t.Helper()
		if _, err := os.Stat(old); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("an old log = %v; want none", err)
		}
	}

	// A missing file is made, empty and private.
	open()
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the new log = %v, %v; want an empty file of mode 0600", fi, err)
	}
	noOld()

	// A small file stays where it is.
	if err := os.WriteFile(path, []byte("a line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	open()
	if got, err := os.ReadFile(path); err != nil || string(got) != "a line\n" {
		t.Fatalf("the small log = %q, %v; want it kept", got, err)
	}
	noOld()

	// A file over 1 MiB moves, replacing the old copy.
	big := bytes.Repeat([]byte("x"), logLimit+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("an older log\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	open()
	if got, err := os.ReadFile(old); err != nil || !bytes.Equal(got, big) {
		t.Fatalf("the old log = %d bytes, %v; want the %d bytes it replaced", len(got), err, len(big))
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		t.Fatalf("the new log = %v, %v; want an empty file", fi, err)
	}
}
