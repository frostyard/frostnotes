package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

func TestHelloBuild(t *testing.T) {
	with := rpctest.StartWith(t, rpctest.Options{Build: "0a1b2c"})
	out, err := runNotesctl(t, "--socket", with.Socket, "hello", "--build")
	if err != nil || out != "0a1b2c\n" {
		t.Fatalf("hello --build = %q, %v; want the build only", out, err)
	}
	// --build wins over --json, and a notesd with no build gives an empty line.
	without := rpctest.Start(t)
	out, err = runNotesctl(t, "--socket", without.Socket, "--json", "hello", "--build")
	if err != nil || out != "\n" {
		t.Fatalf("hello --build with no build = %q, %v; want an empty line", out, err)
	}
	// Plain hello keeps its output.
	out, err = runNotesctl(t, "--socket", with.Socket, "hello")
	if err != nil || out != "notesd rpctest (protocol 1)\n" {
		t.Fatalf("hello = %q, %v", out, err)
	}
	missing := filepath.Join(t.TempDir(), "notesd.sock")
	if out, err := runNotesctl(t, "--socket", missing, "hello", "--build"); err == nil || out != "" {
		t.Fatalf("hello --build without notesd = %q, %v; want an error and no output", out, err)
	}
}

// notesdAnswers reports whether a notesd answers rpc.hello on socket.
func notesdAnswers(ctx context.Context, socket string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, socket, "notesctl test")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func TestShutdownStopsNotesdAndWaits(t *testing.T) {
	t.Setenv("FROSTNOTES_DATA_DIR", t.TempDir())
	srv := rpctest.Start(t)
	out, err := runNotesctl(t, "--socket", srv.Socket, "shutdown")
	if err != nil || out != "notesd stopped\n" {
		t.Fatalf("shutdown = %q, %v", out, err)
	}
	select {
	case <-srv.Stopped(): // by the time notesctl returned, notesd no longer answered
	default:
		t.Fatal("shutdown returned while notesd was still serving")
	}
	if notesdAnswers(t.Context(), srv.Socket) {
		t.Fatal("notesd answers after shutdown")
	}
	// Nothing answers now: that is success.
	out, err = runNotesctl(t, "--socket", srv.Socket, "shutdown")
	if err != nil || out != "notesd is not running\n" {
		t.Fatalf("shutdown with no notesd = %q, %v; want %q", out, err, "notesd is not running\n")
	}
}

// stayingRPC accepts rpc.shutdown and does nothing, as a notesd that does
// not stop.
type stayingRPC struct{ api.RPCService }

func (stayingRPC) Shutdown(context.Context, *api.RPCShutdownParams) error { return nil }

func TestShutdownFailsWhenNotesdKeepsAnswering(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.RPC = stayingRPC{s.RPC} }})
	var out bytes.Buffer
	err := runShutdown(t.Context(), &out, srv.Socket, t.TempDir(), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still answers") {
		t.Fatalf("runShutdown = %v; want an error that notesd still answers", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q; want none", out.String())
	}
	if !notesdAnswers(t.Context(), srv.Socket) {
		t.Fatal("the stand-in notesd stopped")
	}
}

// A data directory that does not exist holds no lock.
func TestShutdownWithNoDataDirectory(t *testing.T) {
	srv := rpctest.Start(t)
	missing := filepath.Join(t.TempDir(), "none")
	if err := runShutdown(t.Context(), &bytes.Buffer{}, srv.Socket, missing, shutdownWait); err != nil {
		t.Fatal(err)
	}
}
