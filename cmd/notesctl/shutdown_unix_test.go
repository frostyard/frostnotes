//go:build !windows

package main

// These tests need a real data lock (flock) or a Unix socket listener of their own.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	kitrpctest "github.com/frostyard/lodge/kit/rpctest"

	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// holdLock takes notesd's lock on dir, as the notesd of that data directory
// does, and returns the function that releases it.
func holdLock(t *testing.T, dir string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, config.LockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release = func() { once.Do(func() { _ = f.Close() }) }
	t.Cleanup(release)
	return release
}

// After the socket stops answering, notesd can still hold the data lock.
// shutdown waits until it can take that lock, and fails when it cannot.
func TestShutdownWaitsForTheDataLock(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FROSTNOTES_DATA_DIR", data)
	release := holdLock(t, data) // stands for the stopping notesd's lock

	// The socket stops, the lock stays: shutdown fails at its limit.
	first := rpctest.Start(t)
	var out bytes.Buffer
	err := runShutdown(t.Context(), &out, first.Socket, data, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still holds the lock") || out.Len() != 0 {
		t.Fatalf("runShutdown = %q, %v; want an error that notesd still holds the lock", out.String(), err)
	}
	<-first.Stopped()

	// The lock goes while shutdown waits for it: shutdown then returns 0.
	second := rpctest.Start(t)
	done := make(chan error, 1)
	go func() { done <- runShutdown(t.Context(), &bytes.Buffer{}, second.Socket, data, shutdownWait) }()
	<-second.Stopped() // the socket answers no more, the lock is still held
	select {
	case err := <-done:
		t.Fatalf("shutdown returned %v while the lock was held", err)
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("shutdown after the lock went = %v; want success", err)
	}
	// shutdown took the lock only to let it go.
	if free, err := lockFree(data); err != nil || !free {
		t.Fatalf("the lock after shutdown = %v, %v; want it free", free, err)
	}
}

// No socket answers, but the lock is held: a notesd that is starting (it
// locks before it listens) or stopping. shutdown waits for the lock, and
// exits 0 once it is free; when it is never free, it fails by its deadline
// with the lock error.
func TestShutdownWithNoSocketWaitsForTheLock(t *testing.T) {
	data := t.TempDir()
	socket := filepath.Join(t.TempDir(), "notesd.sock") // nothing listens
	release := holdLock(t, data)

	var out bytes.Buffer
	err := runShutdown(t.Context(), &out, socket, data, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still holds the lock") || !errors.Is(err, context.DeadlineExceeded) || out.Len() != 0 {
		t.Fatalf("runShutdown = %q, %v; want the lock error at the deadline", out.String(), err)
	}

	out.Reset()
	done := make(chan error, 1)
	go func() { done <- runShutdown(t.Context(), &out, socket, data, shutdownWait) }()
	select {
	case err := <-done:
		t.Fatalf("shutdown returned %v while the lock was held", err) // the lock is still ours
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("shutdown after the lock went = %v; want success", err)
	}
	if out.String() == "" {
		t.Fatal("shutdown printed nothing")
	}
	if free, err := lockFree(data); err != nil || !free {
		t.Fatalf("the lock after shutdown = %v, %v; want it free", free, err)
	}

	// Nothing answers and the lock is free at once: the plain message.
	out.Reset()
	if err := runShutdown(t.Context(), &out, socket, data, shutdownWait); err != nil || out.String() != "notesd is not running\n" {
		t.Fatalf("runShutdown = %q, %v; want %q", out.String(), err, "notesd is not running\n")
	}
}

// listen runs a stand-in on a new socket: it answers rpc.hello when hello is
// set, never answers rpc.shutdown, and never answers anything else.
func listen(t *testing.T, hello bool) string {
	t.Helper()
	socket := kitrpctest.Socket(t)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = nc.Close() })
			go func() {
				sc := bufio.NewScanner(nc)
				for sc.Scan() {
					var req struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
					}
					if json.Unmarshal(sc.Bytes(), &req) != nil || req.Method != "rpc.hello" || !hello {
						continue // no answer
					}
					_, _ = fmt.Fprintf(nc, `{"jsonrpc":"2.0","id":%s,"result":{"protocol":1,"server":"stand-in"}}`+"\n", req.ID)
				}
			}()
		}
	}()
	return socket
}

// One deadline covers every call: a listener that accepts and never answers
// rpc.hello, or that answers it and never answers rpc.shutdown, ends
// shutdown at its deadline, not never.
func TestShutdownEndsAtItsDeadlineWhenNotesdNeverAnswers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hello bool
		want  string
	}{
		{"no answer to rpc.hello", false, "did not answer on"},
		{"no answer to rpc.shutdown", true, "did not answer rpc.shutdown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runShutdown(t.Context(), &out, listen(t, tc.hello), t.TempDir(), 300*time.Millisecond)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("runShutdown = %v; want an error with %q at the deadline", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("output = %q; want none", out.String())
			}
		})
	}
}
