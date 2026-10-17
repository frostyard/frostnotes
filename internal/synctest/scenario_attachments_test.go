package synctest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestScenarioAttachmentConflict(t *testing.T) {
	w := NewWorld(t, Options{Seed: 4})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	a, b, c := w.Token(), w.Token(), w.Token()
	const path = "files/pic.bin"
	if err := w.WriteDisk(d0, path, []byte("\x00\xff"+a+"\xff first")); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d0)
	w.Offline(d1)
	first := "\x00\xff" + b + "\xff from d0"
	last := "\x00\xff" + c + "\xff from d1"
	if err := w.WriteDisk(d0, path, []byte(first)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDisk(d1, path, []byte(last)); err != nil {
		t.Fatal(err)
	}
	w.Online(d0)
	if err := w.SettleOnline(); err != nil {
		t.Fatal(err)
	}
	w.Online(d1)
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	if got := w.Heads()[path]; got != last {
		t.Fatalf("hub's head = %q, want d1's content %q", got, last)
	}
	if f, ok := w.HubFile(path); !ok || !f.Conflicted {
		t.Fatalf("hub's attachment: found %v, conflicted %v; want both true", ok, f.Conflicted)
	}
	checkAttachmentConflictRevision(t, w, path, first)
}

func checkAttachmentConflictRevision(t *testing.T, w *World, path, content string) {
	t.Helper()
	history, err := w.History(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	want := hex.EncodeToString(sum[:])
	for _, rev := range history {
		if rev.Op == api.RevisionOpConflict && rev.Hash == want {
			return
		}
	}
	t.Fatalf("history of %s has no conflict revision with d0's hash %s: %+v", path, want, history)
}

func TestScenarioBigAttachment(t *testing.T) {
	w := NewWorld(t, Options{Seed: 4})
	defer w.Close()
	d2 := w.Devices[2]
	content := make([]byte, bigSize)
	for i := range content {
		content[i] = byte(i * 7)
	}
	copy(content[100:], w.Token())
	const path = "files/big.bin"
	if err := w.WriteDisk(d2, path, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	for _, d := range w.Devices {
		if got := Files(d)[path]; !bytes.Equal([]byte(got), content) {
			t.Fatalf("%s's %s differs from the original %d bytes (got %d bytes)", d.Name, path, len(content), len(got))
		}
	}
}

func TestScenarioBigNote(t *testing.T) {
	w := NewWorld(t, Options{Seed: 4})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	const path = "long.md"
	const line = "a line of a long note\n"
	base := strings.Repeat(line, (10<<20)/len(line)+1)
	if err := w.WriteDisk(d0, path, []byte(base)); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d0)
	w.Offline(d1)
	e, f := w.Token(), w.Token()
	first, last := base+e+"\n", f+"\n"+base
	if err := w.WriteDisk(d0, path, []byte(first)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDisk(d1, path, []byte(last)); err != nil {
		t.Fatal(err)
	}
	w.Online(d0)
	if err := w.SettleOnline(); err != nil {
		t.Fatal(err)
	}
	w.Online(d1)
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	got := w.Heads()[path]
	if got != last {
		t.Fatalf("hub's big note differs from d1's content (got %d bytes, want %d)", len(got), len(last))
	}
	if !strings.Contains(got, f) || strings.Contains(got, e) {
		t.Fatal("hub's big note must contain d1's token and not d0's token")
	}
	if file, ok := w.HubFile(path); !ok || !file.Conflicted {
		t.Fatalf("hub's big note: found %v, conflicted %v; want both true", ok, file.Conflicted)
	}
}

func TestScenarioAttachmentsProfile(t *testing.T) {
	profile := Profile{
		Weights: map[Op]int{Attach: 4, BigAttach: 1, Edit: 3, Delete: 1, Offline: 2, Online: 3},
	}
	for _, tc := range []struct {
		name       string
		seed       uint64
		concurrent bool
	}{
		{name: "stepwise", seed: 1},
		{name: "concurrent", seed: 2, concurrent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const steps = 100
			opts := Options{Seed: tc.seed, Concurrent: tc.concurrent, Profile: &profile}
			if err := Run(t, opts, PlanWith(tc.seed, 3, steps, profile.Weights)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
