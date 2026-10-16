//go:build integration

package vault

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// The kill -9 harness (plan 0003, Phase 4; Phase 7's exit evidence 4). It
// re-runs this test binary as a child notesd that writes, or indexes, a
// generated vault, kills it at a random point, restarts on the same data,
// and checks that every file holds a complete version notesd knows and
// that the index equals a fresh rebuild. FROSTNOTES_KILL_RUNS sets the
// kills per mode (default 100); run it with make vault-it.

const (
	killMode  = "FROSTNOTES_KILL_CHILD" // writes or scan: run as the child
	killData  = "FROSTNOTES_KILL_DATA"
	killVault = "FROSTNOTES_KILL_VAULT"
	killSeed  = "FROSTNOTES_KILL_SEED"
	killBatch = 50 // the child's scan batches: small, so kills land between them
)

func TestIKillNotesd(t *testing.T) {
	runs := 100
	if s := os.Getenv("FROSTNOTES_KILL_RUNS"); s != "" {
		var err error
		if runs, err = strconv.Atoi(s); err != nil {
			t.Fatalf("FROSTNOTES_KILL_RUNS=%q: %v", s, err)
		}
	}
	pristine := filepath.Join(t.TempDir(), "pristine")
	if _, err := vaultgen.Generate(pristine, vaultgen.Options{Seed: 5, Notes: 300}); err != nil {
		t.Fatal(err)
	}
	original := readTree(t, pristine)

	// How long a cold scan takes sets the range of the scan mode's kills.
	start := time.Now()
	fresh := openTestVault(t, pristine, t.TempDir(), func(c *Config) { c.Batch = killBatch })
	fresh.scan()
	coldScan := time.Since(start)
	t.Logf("a cold scan of %d files takes %v", len(original), coldScan)

	for _, mode := range []string{"writes", "scan"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "vault")
			copyTree(t, pristine, dir)
			data := t.TempDir()
			window := 60 * time.Millisecond
			if mode == "scan" {
				window = coldScan + coldScan/5
			}
			rng := rand.New(rand.NewPCG(uint64(len(mode)), 7))
			for run := range runs {
				if mode == "scan" && run%2 == 0 {
					data = t.TempDir() // a cold index; odd runs reindex
				}
				delay := time.Duration(rng.Int64N(int64(window)))
				killChild(t, mode, data, dir, run, delay)
				checkAfterKill(t, data, dir, original, run)
			}
		})
	}
}

// killChild runs the child until it is ready, lets it work for delay, and
// kills it.
func killChild(t *testing.T, mode, data, dir string, run int, delay time.Duration) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestIKillChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), killMode+"="+mode, killData+"="+data, killVault+"="+dir,
		killSeed+"="+strconv.Itoa(run), fmt.Sprintf("FROSTNOTES_KILL_REINDEX=%v", run%2 == 1))
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if sc.Text() == "ready" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	if !<-ready {
		_ = cmd.Wait()
		t.Fatalf("run %d: the child ended before it was ready", run)
	}
	time.Sleep(delay) // the kill's random point, not synchronization
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
}

// TestIKillChild is the child notesd; it only runs under TestIKillNotesd.
func TestIKillChild(t *testing.T) {
	mode := os.Getenv(killMode)
	if mode == "" {
		t.Skip("run by TestIKillNotesd")
	}
	tv := openTestVault(t, os.Getenv(killVault), os.Getenv(killData), func(c *Config) { c.Batch = killBatch })
	tv.db.Now = time.Now
	ctx := context.Background()
	seed, _ := strconv.Atoi(os.Getenv(killSeed))
	if mode == "scan" {
		fmt.Println("ready")
		reindex := os.Getenv("FROSTNOTES_KILL_REINDEX") == "true"
		for {
			if _, err := tv.v.runScan(ctx, reindex); err != nil {
				t.Fatal(err)
			}
			reindex = true
		}
	}
	tv.scan()
	fmt.Println("ready")
	rng := rand.New(rand.NewPCG(uint64(seed), 11))
	for i := 0; ; i++ {
		notes, err := tv.db.ListNotes(ctx, tv.v.id, store.NoteFilter{})
		if err != nil {
			t.Fatal(err)
		}
		var created []api.NoteEntry
		for _, n := range notes {
			if strings.HasPrefix(n.Path, "Kill/") {
				created = append(created, n)
			}
		}
		id := fmt.Sprintf("%d-%d", seed, i)
		switch op := rng.IntN(10); {
		case op < 2 || len(created) == 0:
			_, err = tv.v.Create(ctx, "Kill/new-"+id+".md", []byte("created "+id), editor)
		case op == 2:
			n := created[rng.IntN(len(created))]
			err = tv.v.Delete(ctx, n.Path, n.Hash, editor)
		case op == 3:
			n := created[rng.IntN(len(created))]
			to := "Kill/moved/" + path.Base(n.Path)
			if strings.HasPrefix(n.Path, "Kill/moved/") {
				to = "Kill/" + path.Base(n.Path)
			}
			_, err = tv.v.Move(ctx, n.Path, to, false, editor)
		default:
			n := notes[rng.IntN(len(notes))]
			var cur *api.Note
			if cur, err = tv.v.Read(ctx, n.Path); err == nil {
				_, err = tv.v.Write(ctx, n.Path, cur.Hash, []byte(cur.Content+editLine(i)), agent)
			}
		}
		if err != nil {
			t.Fatalf("op %d: %v", i, err)
		}
	}
}

func editLine(i int) string { return fmt.Sprintf("\nedit %d %s", i, strings.Repeat("x", 1000)) }

var (
	editLines   = regexp.MustCompile(`^(\nedit \d+ x{1000})*$`)
	createdLine = regexp.MustCompile(`^created [0-9-]+`)
)

// checkAfterKill restarts on data as notesd would, then checks the vault
// and its index.
func checkAfterKill(t *testing.T, data, dir string, original map[string]string, run int) {
	t.Helper()
	tv := openTestVault(t, dir, data, func(c *Config) { c.Batch = killBatch })
	tv.scan()
	ctx := t.Context()
	files := readTree(t, dir)
	for p, content := range files {
		if strings.HasPrefix(path.Base(p), tempPrefix) {
			t.Fatalf("run %d: %s, a write's temporary file, survived the restart", run, p)
		}
		// Every file holds a complete version: what the generator wrote,
		// or what a create made, followed by whole edits.
		rest, ok := strings.CutPrefix(content, original[p])
		if _, orig := original[p]; !orig {
			m := createdLine.FindString(content)
			rest, ok = content[len(m):], m != "" && strings.HasPrefix(p, "Kill/")
		}
		if !ok || !editLines.MatchString(rest) {
			t.Fatalf("run %d: %s holds no version a writer made (%d bytes)", run, p, len(content))
		}
		row := tv.row(p)
		if row.Hash != blobs.ID([]byte(content)) {
			t.Fatalf("run %d: the index's hash of %s is not the file's", run, p)
		}
		for _, v := range tv.versions(row.ID) {
			if v.Hash == "" {
				continue
			}
			if ok, err := tv.blobs.Has(v.Hash); err != nil || !ok {
				t.Fatalf("run %d: a version of %s has no blob", run, p)
			}
		}
		if ok, err := tv.db.HasVersion(ctx, row.ID, row.Hash); err != nil || !ok {
			t.Fatalf("run %d: %s's content is none of its versions", run, p)
		}
	}
	for p := range original {
		if _, ok := files[p]; !ok {
			t.Fatalf("run %d: %s, which no writer deletes, is gone", run, p)
		}
	}
	got, err := tv.db.IndexSnapshot(ctx, tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := openTestVault(t, dir, t.TempDir(), nil)
	rebuilt.scan()
	want, err := rebuilt.db.IndexSnapshot(ctx, rebuilt.v.id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		for _, row := range got {
			if !slices.Contains(want, row) {
				t.Errorf("run %d: only after the restart: %.200s", run, row)
			}
		}
		for _, row := range want {
			if !slices.Contains(got, row) {
				t.Errorf("run %d: only in the rebuild: %.200s", run, row)
			}
		}
		t.FailNow()
	}
	_ = tv.db.Close()
	_ = rebuilt.db.Close()
}

// readTree returns every regular file under dir, hidden ones included,
// by vault path.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	for p, content := range readTree(t, from) {
		full := filepath.Join(to, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
