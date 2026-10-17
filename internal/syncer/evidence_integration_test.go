//go:build integration

package syncer_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// attachmentSize is evidence 2's attachment: 100 MB.
const attachmentSize = 100 << 20

// TestIHundredMegabyteAttachment is M2's exit evidence 2 (plan 0004,
// Phase 7): a 100 MB attachment syncs from one device to another through
// a hub, three notesd processes on loopback, arriving with its hash, and
// no process's peak memory grows more than 64 MB above its idle. Run it
// with make m2-evidence.
func TestIHundredMegabyteAttachment(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "notesd")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/frostyard/frostnotes/cmd/notesd").CombinedOutput(); err != nil {
		t.Fatalf("build notesd: %v\n%s", err, out)
	}
	hubAddr := freeAddr(t)
	hub := startNotesd(t, bin, "hub", "-hub", hubAddr)
	a := startNotesd(t, bin, "a")
	b := startNotesd(t, bin, "b")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	hv := hub.addVault(ctx, t)
	if _, err := hub.c.Hub().Serve(ctx, &api.HubServeParams{VaultID: hv}); err != nil {
		t.Fatal(err)
	}
	url := "http://" + hubAddr
	for _, d := range []*proc{a, b} {
		d.vault = d.addVault(ctx, t)
		d.join(ctx, t, hub, url)
	}
	for _, p := range []*proc{hub, a, b} {
		p.resetPeak(t)
	}

	// The attachment, written on a's disk as another program would.
	want := writeRandom(t, filepath.Join(a.dir, "files", "big.bin"), attachmentSize)
	begin := time.Now()
	b.waitFor(ctx, t, "files/big.bin")
	took := time.Since(begin)
	got := hashFile(t, filepath.Join(b.dir, "files", "big.bin"))
	if got != want {
		t.Fatalf("b's copy hashes to %s, a's to %s", got, want)
	}
	t.Logf("100 MB reached b with its hash in %v (the 2 s quiet period included)", took.Round(time.Millisecond))
	for _, p := range []*proc{hub, a, b} {
		idle, peak := p.memory(t)
		t.Logf("%s: idle %d MB, peak %d MB, growth %d MB", p.name, idle>>20, peak>>20, (peak-idle)>>20)
		if peak-idle > 64<<20 {
			t.Errorf("%s's peak memory grew %d MB over its idle %d MB; at most 64 MB", p.name, (peak-idle)>>20, idle>>20)
		}
	}
}

// proc is one notesd process.
type proc struct {
	name  string
	cmd   *exec.Cmd
	data  string
	dir   string // its vault
	c     *api.Client
	vault int64
	idle  int64 // its resident memory when its peak was reset
}

func startNotesd(t *testing.T, bin, name string, args ...string) *proc {
	t.Helper()
	root := t.TempDir()
	p := &proc{name: name, data: filepath.Join(root, "data"), dir: filepath.Join(root, "vault")}
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(root, "run") // notesd wants its socket's folder private
	if err := os.Mkdir(run, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(run, "notesd.sock")
	p.cmd = exec.Command(bin, args...)
	p.cmd.Env = append(os.Environ(), "FROSTNOTES_DATA_DIR="+p.data, "FROSTNOTES_SOCKET="+socket, "FROSTNOTES_SECRETS=file")
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.cmd.Process.Signal(os.Interrupt)
		_ = p.cmd.Wait()
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
	c, _, err := api.Dial(t.Context(), socket, "evidence")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	p.c = c
	return p
}

func (p *proc) addVault(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	v, err := p.c.Vault().Add(ctx, &api.VaultAddParams{Path: p.dir})
	if err != nil {
		t.Fatal(err)
	}
	return v.ID
}

// join pairs p with the hub and joins its one vault.
func (p *proc) join(ctx context.Context, t *testing.T, hub *proc, url string) {
	t.Helper()
	name := p.name
	pr, err := p.c.Sync().Pair(ctx, &api.SyncPairParams{URL: url, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.c.Hub().Approve(ctx, &api.HubApproveParams{Code: pr.Code}); err != nil {
		t.Fatal(err)
	}
	if r, err := p.c.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: pr.Pairing}); err != nil || r.State != api.PairingStateApproved {
		t.Fatalf("Paired = %+v, %v", r, err)
	}
	vs, err := hub.c.Hub().Vaults(ctx, &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.c.Sync().Join(ctx, &api.SyncJoinParams{VaultID: p.vault, URL: url, Name: vs[0].Name}); err != nil {
		t.Fatal(err)
	}
}

// waitFor waits, by p's note.changed events, until sync has applied the
// file at path.
func (p *proc) waitFor(ctx context.Context, t *testing.T, path string) {
	t.Helper()
	c, _, err := api.Dial(ctx, socketOf(p), "evidence events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.dir, filepath.FromSlash(path))); err == nil {
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
			t.Fatalf("%s never received %s", p.name, path)
		}
	}
}

func socketOf(p *proc) string {
	for _, kv := range p.cmd.Env {
		if s, ok := strings.CutPrefix(kv, "FROSTNOTES_SOCKET="); ok {
			return s
		}
	}
	return ""
}

// resetPeak resets the process's peak resident memory to what it holds
// now (Linux's clear_refs), and records that as its idle.
func (p *proc) resetPeak(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(fmt.Sprintf("/proc/%d/clear_refs", p.cmd.Process.Pid), []byte("5"), 0); err != nil {
		t.Fatalf("reset %s's peak memory: %v", p.name, err)
	}
	p.idle, _ = p.status(t)
}

// memory returns the process's idle and peak resident memory, in bytes.
func (p *proc) memory(t *testing.T) (int64, int64) {
	t.Helper()
	_, peak := p.status(t)
	return p.idle, peak
}

// status reads VmRSS and VmHWM from /proc.
func (p *proc) status(t *testing.T) (rss, hwm int64) {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", p.cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || (k != "VmRSS" && k != "VmHWM") {
			continue
		}
		kb, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if k == "VmRSS" {
			rss = kb << 10
		} else {
			hwm = kb << 10
		}
	}
	return rss, hwm
}

// writeRandom writes size seeded random bytes at path through a temporary
// file and a rename, and returns their hash.
func writeRandom(t *testing.T, path string, size int64) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(filepath.Dir(path), ".big.tmp")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	rng := rand.New(rand.NewPCG(1, 2))
	buf := make([]byte, 1<<20)
	for n := int64(0); n < size; n += int64(len(buf)) {
		for i := range buf {
			buf[i] = byte(rng.Uint32())
		}
		if _, err := io.MultiWriter(f, h).Write(buf[:min(int64(len(buf)), size-n)]); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// freeAddr picks a free loopback address for the hub.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
