// Package problemtest helps tests of notesd's problem records: a fake
// problem.Recorder, and a datagram socket that stands in for the journal,
// whose records are decoded and checked with
// github.com/frostyard/std/problems.
package problemtest

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/std/problems"
)

// AppID is the app ID of every frostnotes record.
const AppID = "org.frostyard.Frostnotes"

// Fake is a problem.Recorder that keeps the codes it gets.
type Fake struct {
	mu    sync.Mutex
	codes []string
}

// Error implements problem.Recorder.
func (f *Fake) Error(code string, _ error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes = append(f.codes, code)
}

// Codes returns the codes recorded so far, in order.
func (f *Fake) Codes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.codes)
}

// Journal is a unixgram socket in a temporary folder that takes records in
// place of the journal's socket.
type Journal struct {
	// Path is the socket's path, for problems.WithSocket.
	Path string
	c    *net.UnixConn
}

// Listen binds a Journal in a short temporary folder (a socket's path holds
// at most 108 bytes, more than a test's TempDir can leave); it closes, and
// the folder is removed, when the test ends.
func Listen(t testing.TB) *Journal {
	t.Helper()
	dir, err := os.MkdirTemp("", "fn-journal-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "j")
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Close()
		_ = os.RemoveAll(dir)
	})
	return &Journal{Path: path, c: c}
}

// Next waits for the next record, decodes it, checks it with
// problems.Validate, and returns its fields.
func (j *Journal) Next(t testing.TB) map[string]string {
	t.Helper()
	if err := j.c.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	n, err := j.c.Read(buf)
	if err != nil {
		t.Fatalf("read a record: %v", err)
	}
	fields, err := problems.DecodeEntry(buf[:n])
	if err != nil {
		t.Fatalf("DecodeEntry: %v", err)
	}
	if _, err := problems.Validate(fields); err != nil {
		t.Fatalf("Validate: %v\n%q", err, fields)
	}
	return fields
}

// None fails when the socket holds a record. A recorder sends a record
// before the call that writes it returns, so the socket is read without
// waiting.
func (j *Journal) None(t testing.TB) {
	t.Helper()
	rc, err := j.c.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	var n int
	var rerr error
	if err := rc.Read(func(fd uintptr) bool {
		n, _, rerr = syscall.Recvfrom(int(fd), buf, syscall.MSG_DONTWAIT)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	switch {
	case rerr == nil:
		t.Fatalf("got an unexpected record: %q", buf[:n])
	case !errors.Is(rerr, syscall.EAGAIN):
		t.Fatalf("read: %v", rerr)
	}
}

// Want is what a test expects of a record of notesd.
type Want struct {
	Version string // FROSTYARD_APP_VERSION
	Class   string // panic or error
	Code    string // FROSTYARD_CODE; empty for a panic
}

// Check compares fields, a record that Next returned, with want: the app
// ID, the version, the Go runtime, the daemon component, the class and the
// code.
func Check(t testing.TB, fields map[string]string, want Want) {
	t.Helper()
	for name, value := range map[string]string{
		"MESSAGE_ID":            problems.MessageID,
		"FROSTYARD_APP_ID":      AppID,
		"FROSTYARD_APP_VERSION": want.Version,
		"FROSTYARD_RUNTIME":     "go",
		"FROSTYARD_COMPONENT":   string(problems.ComponentDaemon),
		"FROSTYARD_CLASS":       want.Class,
		"FROSTYARD_CODE":        want.Code,
	} {
		got, ok := fields[name]
		if value == "" {
			if ok {
				t.Errorf("%s = %q; want none", name, got)
			}
			continue
		}
		if got != value {
			t.Errorf("%s = %q; want %q", name, got, value)
		}
	}
}
