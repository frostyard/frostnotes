package main

import (
	"strings"
	"testing"
)

func TestHubListensOnLoopbackOnly(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:7880", "[::1]:7880", "localhost:0", "127.0.0.2:1"} {
		if err := checkLoopback(ok); err != nil {
			t.Errorf("checkLoopback(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{":7880", "0.0.0.0:7880", "[::]:7880", "100.64.0.1:7880", "hub.example:7880", "127.0.0.1"} {
		if err := checkLoopback(bad); err == nil {
			t.Errorf("checkLoopback(%q) accepted a non-loopback address", bad)
		}
	}
}

func TestDataDirIsLockedPerNotesd(t *testing.T) {
	dir := t.TempDir()
	first, err := lockDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockDataDir(dir); err == nil || !strings.Contains(err.Error(), "another notesd uses "+dir) {
		t.Fatalf("a second lock on %s = %v, want \"another notesd uses\"", dir, err)
	}
	other, err := lockDataDir(t.TempDir())
	if err != nil {
		t.Fatalf("a lock on another directory: %v", err)
	}
	_ = other.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := lockDataDir(dir)
	if err != nil {
		t.Fatalf("a lock after the first was released: %v", err)
	}
	_ = again.Close()
}
