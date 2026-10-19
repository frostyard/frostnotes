//go:build !windows

package rpcserver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/rpcserver"
)

func TestListenRejectsOpenDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "fn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := rpcserver.New(rpcserver.Options{}).Listen(filepath.Join(dir, "notesd.sock")); err == nil || !strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("Listen in a 0755 directory = %v", err)
	}
}
