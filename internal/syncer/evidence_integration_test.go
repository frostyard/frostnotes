//go:build integration

package syncer_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/evidence"
)

// attachmentSize is evidence 2's attachment: 100 MB.
const attachmentSize = 100 << 20

// TestIHundredMegabyteAttachment is M2's exit evidence 2 (plan 0004,
// Phase 7): a 100 MB attachment syncs from one device to another through
// a hub, three notesd processes on loopback, arriving with its hash, and
// no process's peak memory grows more than 64 MB above its idle. Run it
// with make m2-evidence.
func TestIHundredMegabyteAttachment(t *testing.T) {
	bin := evidence.Build(t, "github.com/frostyard/frostnotes/cmd/notesd")
	hubAddr := evidence.FreeAddr(t)
	hub := &proc{Notesd: evidence.Start(t, bin, "hub", "-hub", hubAddr)}
	a := &proc{Notesd: evidence.Start(t, bin, "a")}
	b := &proc{Notesd: evidence.Start(t, bin, "b")}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	hv := hub.AddVault(ctx, t)
	if _, err := hub.C.Hub().Serve(ctx, &api.HubServeParams{VaultID: hv}); err != nil {
		t.Fatal(err)
	}
	url := "http://" + hubAddr
	for _, d := range []*proc{a, b} {
		d.AddVault(ctx, t)
		d.Join(ctx, t, hub.Notesd, url)
	}
	for _, p := range []*proc{hub, a, b} {
		p.resetPeak(t)
	}

	// The attachment, written on a's disk as another program would.
	want := writeRandom(t, filepath.Join(a.Dir, "files", "big.bin"), attachmentSize)
	begin := time.Now()
	b.WaitFor(ctx, t, "files/big.bin")
	took := time.Since(begin)
	got := hashFile(t, filepath.Join(b.Dir, "files", "big.bin"))
	if got != want {
		t.Fatalf("b's copy hashes to %s, a's to %s", got, want)
	}
	t.Logf("100 MB reached b with its hash in %v (the 2 s quiet period included)", took.Round(time.Millisecond))
	for _, p := range []*proc{hub, a, b} {
		idle, peak := p.memory(t)
		t.Logf("%s: idle %d MB, peak %d MB, growth %d MB", p.Name, idle>>20, peak>>20, (peak-idle)>>20)
		if peak-idle > 64<<20 {
			t.Errorf("%s's peak memory grew %d MB over its idle %d MB; at most 64 MB", p.Name, (peak-idle)>>20, idle>>20)
		}
	}
}

// proc is one notesd process and its idle memory.
type proc struct {
	*evidence.Notesd
	idle int64 // its resident memory when its peak was reset
}

// resetPeak resets the process's peak resident memory to what it holds
// now (Linux's clear_refs), and records that as its idle.
func (p *proc) resetPeak(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(fmt.Sprintf("/proc/%d/clear_refs", p.Cmd.Process.Pid), []byte("5"), 0); err != nil {
		t.Fatalf("reset %s's peak memory: %v", p.Name, err)
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
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", p.Cmd.Process.Pid))
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
