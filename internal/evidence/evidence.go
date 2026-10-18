// Package evidence runs real notesd processes for the milestones' exit
// evidence (make m2-evidence and make m3-evidence; plans 0004 and 0005,
// Phase 7): each with its own data directory, private socket folder,
// vault folder and file secrets, a hub on loopback, all stopped when the
// test ends.
package evidence

import (
	"bufio"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// Notesd is one notesd process.
type Notesd struct {
	Name   string
	Cmd    *exec.Cmd
	Data   string // its data directory
	Dir    string // its vault's folder
	Socket string
	C      *api.Client
	Vault  int64 // its vault, once AddVault has added it
}

// Build builds a command of this module, such as
// "github.com/frostyard/frostnotes/cmd/notesd", into a temporary folder and
// returns the binary's path.
func Build(t testing.TB, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

// Start starts notesd from bin with args, waits until it says it started,
// and connects to it.
func Start(t testing.TB, bin, name string, args ...string) *Notesd {
	t.Helper()
	root := t.TempDir()
	p := &Notesd{Name: name, Data: filepath.Join(root, "data"), Dir: filepath.Join(root, "vault")}
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(root, "run") // notesd wants its socket's folder private
	if err := os.Mkdir(run, 0o700); err != nil {
		t.Fatal(err)
	}
	p.Socket = filepath.Join(run, "notesd.sock")
	p.Cmd = exec.Command(bin, args...)
	p.Cmd.Env = append(os.Environ(), "FROSTNOTES_DATA_DIR="+p.Data, "FROSTNOTES_SOCKET="+p.Socket, "FROSTNOTES_SECRETS=file")
	stderr, err := p.Cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.Cmd.Process.Signal(os.Interrupt)
		_ = p.Cmd.Wait()
	})
	started := make(chan struct{})
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			select {
			case lines <- sc.Text():
			default:
			}
			if strings.Contains(sc.Text(), "notesd started") {
				close(started)
				break
			}
		}
		close(lines)
		_, _ = io.Copy(io.Discard, stderr)
	}()
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		var said []string
		for l := range lines {
			said = append(said, l)
		}
		t.Fatalf("%s did not start:\n%s", name, strings.Join(said, "\n"))
	}
	c, _, err := api.Dial(t.Context(), p.Socket, "evidence")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	p.C = c
	return p
}

// AddVault adds the process's vault folder as its vault.
func (p *Notesd) AddVault(ctx context.Context, t testing.TB) int64 {
	t.Helper()
	v, err := p.C.Vault().Add(ctx, &api.VaultAddParams{Path: p.Dir})
	if err != nil {
		t.Fatal(err)
	}
	p.Vault = v.ID
	return v.ID
}

// Join pairs p with the hub at url and joins its vault to the hub's one.
func (p *Notesd) Join(ctx context.Context, t testing.TB, hub *Notesd, url string) {
	t.Helper()
	name := p.Name
	pr, err := p.C.Sync().Pair(ctx, &api.SyncPairParams{URL: url, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.C.Hub().Approve(ctx, &api.HubApproveParams{Code: pr.Code}); err != nil {
		t.Fatal(err)
	}
	if r, err := p.C.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: pr.Pairing}); err != nil || r.State != api.PairingStateApproved {
		t.Fatalf("Paired = %+v, %v", r, err)
	}
	vs, err := hub.C.Hub().Vaults(ctx, &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.C.Sync().Join(ctx, &api.SyncJoinParams{VaultID: p.Vault, URL: url, Name: vs[0].Name}); err != nil {
		t.Fatal(err)
	}
}

// WaitFor waits, by p's note.changed events, until sync has applied the
// file at path.
func (p *Notesd) WaitFor(ctx context.Context, t testing.TB, path string) {
	t.Helper()
	c, _, err := api.Dial(ctx, p.Socket, "evidence events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(path))); err == nil {
		return
	}
	for {
		select {
		case ev := <-c.Notifications():
			if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
				if ch, ok := e.(api.NoteChanged); ok && ch.Path == path && ch.Source == api.VersionSourceSync {
					return
				}
			}
		case <-ctx.Done():
			t.Fatalf("%s never received %s", p.Name, path)
		}
	}
}

// FreeAddr picks a free loopback address, for a hub.
func FreeAddr(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
