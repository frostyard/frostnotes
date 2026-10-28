//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	kitrpctest "github.com/frostyard/lodge/kit/rpctest"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
)

// sandbox is one notesd's environment in temporary directories, as a Flatpak
// instance, with no keyring and no session bus.
type sandbox struct {
	data, socket, vault string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	sb := sandbox{data: filepath.Join(root, "data"), socket: kitrpctest.Socket(t), vault: filepath.Join(root, "vault")}
	if err := os.Mkdir(sb.vault, 0o700); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(root, "flatpak-info")
	writeFile(t, info, "[Instance]\napp-commit=c0ffee\n")
	for k, v := range map[string]string{
		"FROSTNOTES_DATA_DIR": sb.data, "FROSTNOTES_SOCKET": sb.socket, "FLATPAK_ID": config.AppID,
		"FROSTNOTES_FLATPAK_INFO": info, "FROSTNOTES_SECRETS": "file",
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(root, "no-bus"),
	} {
		t.Setenv(k, v)
	}
	return sb
}

// start runs notesd; done receives run's result when it returns.
func (sb sandbox) start(t *testing.T) (c *api.Client, hello *api.Hello, done <-chan error) {
	t.Helper()
	ready, d := make(chan struct{}), make(chan error, 1)
	go func() { d <- runWith(t.Context(), nil, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-d:
		t.Fatalf("notesd stopped before it served: %v", err)
	}
	c, hello, err := api.Dial(t.Context(), sb.socket, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, hello, d
}

func (sb sandbox) lockIsFree(t *testing.T) {
	t.Helper()
	lock, err := lockDataDir(sb.data)
	if err != nil {
		t.Fatalf("the data lock is still held: %v", err)
	}
	_ = lock.Close()
}

// rpc.shutdown answers, then notesd stops as on SIGTERM: run returns nil, the
// data directory's lock is free and the socket answers no more. rpc.hello
// reports the build.
func TestRPCShutdownStopsNotesdAfterItAnswers(t *testing.T) {
	sb := newSandbox(t)
	c, h, done := sb.start(t)
	if h.Build == nil || *h.Build != "c0ffee" {
		t.Fatalf("hello build = %v; want c0ffee", h.Build)
	}
	// The caller has its answer, and only then does notesd stop.
	if err := c.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); err != nil {
		t.Fatalf("rpc.shutdown = %v; want its result", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("notesd stopped with %v; want a clean stop", err)
	}
	sb.lockIsFree(t)
	if c2, _, err := api.Dial(t.Context(), sb.socket, "test"); err == nil {
		_ = c2.Close()
		t.Fatal("the socket answers after shutdown")
	}
}

// No write is lost: with creates in flight when rpc.shutdown arrives, each
// create that was answered is on disk, and its note is in the index of the
// notesd that starts next on the same data directory. A create that was not
// answered failed with an error, as the caller's connection ended: its caller
// knows to write again.
func TestNoAnsweredWriteIsLostToShutdown(t *testing.T) {
	sb := newSandbox(t)
	c, _, done := sb.start(t)
	v, err := c.Vault().Add(t.Context(), &api.VaultAddParams{Path: sb.vault})
	if err != nil {
		t.Fatal(err)
	}
	// Three creates are answered before the stop, so the check has something to find.
	answered := []string{"a.md", "b.md", "c.md"}
	for _, p := range answered {
		if _, err := c.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: v.ID, Path: p, Content: "# " + p + "\n"}); err != nil {
			t.Fatal(err)
		}
	}
	const writers = 24
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for i := range writers {
		wg.Go(func() {
			p := fmt.Sprintf("n%02d.md", i)
			if _, err := c.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: v.ID, Path: p, Content: "# " + p + "\n"}); err == nil {
				mu.Lock()
				answered = append(answered, p)
				mu.Unlock()
			}
			if i == writers/2 {
				_ = c.RPC().Shutdown(context.WithoutCancel(t.Context()), &api.RPCShutdownParams{})
			}
		})
	}
	wg.Wait()
	if err := <-done; err != nil {
		t.Fatalf("notesd stopped with %v; want a clean stop", err)
	}
	sb.lockIsFree(t)

	c2, _, done2 := sb.start(t)
	for _, p := range answered {
		n, err := c2.Note().Read(t.Context(), &api.NoteReadParams{VaultID: v.ID, Path: p})
		if err != nil || n.Content != "# "+p+"\n" {
			t.Errorf("an answered create of %s is lost after the restart: %v", p, err)
		}
	}
	t.Logf("%d of %d creates were answered before the stop", len(answered), writers)
	if err := c2.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); err != nil {
		t.Fatal(err)
	}
	if err := <-done2; err != nil {
		t.Fatal(err)
	}
}

// A second notesd on the same data directory does not start, and the first
// goes on: shutdown stops only the notesd that was asked.
func TestSecondNotesdFailsOnTheLockAndTheFirstGoesOn(t *testing.T) {
	sb := newSandbox(t)
	c, _, done := sb.start(t)
	t.Setenv("FROSTNOTES_SOCKET", kitrpctest.Socket(t))
	if err := run(t.Context(), nil); err == nil {
		t.Fatal("a second notesd started on the data directory")
	}
	if err := c.RPC().Ping(t.Context(), &api.RPCPingParams{}); err != nil {
		t.Fatalf("rpc.ping on the first notesd = %v", err)
	}
	if err := c.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
