package syncer_test

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/syncer"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// node is one notesd of a test's sync setup: the hub or a device, with
// one vault.
type node struct {
	t     *testing.T
	srv   *rpctest.Server
	c     *api.Client
	vault int64
	dir   string
}

func start(t *testing.T, o rpctest.Options, dir string) *node {
	t.Helper()
	o.Vault.Debounce = 10 * time.Millisecond
	if os.Getenv("SYNC_DEBUG") != "" {
		o.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	srv := rpctest.StartWith(t, o)
	n := &node{t: t, srv: srv, c: srv.Dial(t), dir: dir}
	v, err := n.c.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	n.vault = v.ID
	n.waitScanned()
	return n
}

// waitScanned waits for the vault's first scan.
func (n *node) waitScanned() {
	n.t.Helper()
	for range 500 {
		st, err := n.c.Vault().Status(n.t.Context(), &api.VaultStatusParams{ID: n.vault})
		if err != nil {
			n.t.Fatal(err)
		}
		if st.State == api.ScanStateIdle && st.ScannedAt != nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	n.t.Fatal("the vault's scan did not finish")
}

func vaultDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// newHub starts a hub serving a vault with files.
func newHub(t *testing.T, files map[string]string) *node {
	t.Helper()
	h := start(t, rpctest.Options{HubListen: true}, vaultDir(t, files))
	if _, err := h.c.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: h.vault}); err != nil {
		t.Fatal(err)
	}
	return h
}

// newDevice starts a device with a vault of files, pairs it with the hub
// and joins the hub's vault.
func newDevice(t *testing.T, h *node, name string, files map[string]string) *node {
	t.Helper()
	return pairAndJoin(t, h, start(t, deviceOptions(name), vaultDir(t, files)), name)
}

// deviceOptions are a test device's: short quiet periods and backoff.
func deviceOptions(name string) rpctest.Options {
	return rpctest.Options{Syncer: syncer.Config{Device: name, Quiet: time.Millisecond, MaxQuiet: 10 * time.Millisecond,
		Idle: time.Second, Backoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond}}
}

// settle waits until every device has nothing to push and has pulled
// everything the hub's log holds.
func settle(t *testing.T, h *node, devices ...*node) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for range 50 {
		vs, err := h.c.Hub().Vaults(ctx, &api.HubVaultsParams{})
		if err != nil {
			t.Fatal(err)
		}
		quiet := true
		for _, d := range devices {
			if err := d.srv.Syncer.WaitIdle(ctx, d.vault); err != nil {
				t.Fatalf("WaitIdle: %v", err)
			}
			st, err := d.c.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &d.vault})
			if err != nil {
				t.Fatal(err)
			}
			if st[0].Pending != 0 || st[0].Cursor < vs[0].Seq {
				quiet = false
			}
		}
		if after, _ := h.c.Hub().Vaults(ctx, &api.HubVaultsParams{}); quiet && after[0].Seq == vs[0].Seq {
			return
		}
	}
	t.Fatal("the devices did not settle")
}

// files reads every vault file under dir, but the settings folder.
func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		data, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// same fails unless every node's vault holds the same files.
func same(t *testing.T, nodes ...*node) map[string]string {
	t.Helper()
	want := files(t, nodes[0].dir)
	for _, n := range nodes[1:] {
		got := files(t, n.dir)
		var diff []string
		for p, c := range want {
			if got[p] != c {
				diff = append(diff, p)
			}
		}
		for p := range got {
			if _, ok := want[p]; !ok {
				diff = append(diff, p)
			}
		}
		if len(diff) > 0 {
			sort.Strings(diff)
			t.Fatalf("vaults differ at %d paths, among them %q", len(diff), diff[:min(5, len(diff))])
		}
	}
	return want
}

func (n *node) write(p, content string) {
	n.t.Helper()
	ctx := n.t.Context()
	note, err := n.c.Note().Read(ctx, &api.NoteReadParams{VaultID: n.vault, Path: p})
	if err != nil {
		if _, err := n.c.Note().Create(ctx, &api.NoteCreateParams{VaultID: n.vault, Path: p, Content: content}); err != nil {
			n.t.Fatal(err)
		}
		return
	}
	if _, err := n.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: n.vault, Path: p, Base: note.Hash, Content: content}); err != nil {
		n.t.Fatal(err)
	}
}

func (n *node) read(p string) string {
	n.t.Helper()
	data, err := os.ReadFile(filepath.Join(n.dir, filepath.FromSlash(p)))
	if err != nil {
		n.t.Fatalf("read %s: %v", p, err)
	}
	return string(data)
}

func TestAGeneratedVaultSyncsBothWays(t *testing.T) {
	dir := t.TempDir()
	if _, err := vaultgen.Generate(filepath.Join(dir, "v"), vaultgen.Options{Seed: 4, Notes: 666}); err != nil {
		t.Fatal(err)
	}
	h := newHub(t, nil)
	a2 := pairAndJoin(t, h, start(t, deviceOptions("a"), filepath.Join(dir, "v")), "a")
	b := newDevice(t, h, "b", nil)
	settle(t, h, a2, b)
	got := same(t, h, a2, b)
	if len(got) < 666 {
		t.Fatalf("%d files synced, want the generated vault's", len(got))
	}
	// And back: b edits a note and adds one, a sees both.
	b.write("from-b.md", "written on b\n")
	var some string
	for p := range got {
		if strings.HasSuffix(p, ".md") {
			some = p
			break
		}
	}
	b.write(some, got[some]+"\nedited on b\n")
	settle(t, h, a2, b)
	same(t, h, a2, b)
	if a2.read("from-b.md") != "written on b\n" || !strings.HasSuffix(a2.read(some), "edited on b\n") {
		t.Fatal("b's edits did not reach a")
	}
}

// pairAndJoin pairs device d with the hub and joins the hub's vault.
func pairAndJoin(t *testing.T, h, d *node, name string) *node {
	t.Helper()
	ctx := t.Context()
	p, err := d.c.Sync().Pair(ctx, &api.SyncPairParams{URL: h.srv.HubURL, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		t.Fatal(err)
	}
	if r, err := d.c.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing}); err != nil || r.State != api.PairingStateApproved {
		t.Fatalf("Paired = %+v, %v", r, err)
	}
	vs, err := h.c.Hub().Vaults(ctx, &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.c.Sync().Join(ctx, &api.SyncJoinParams{VaultID: d.vault, URL: h.srv.HubURL, Name: vs[0].Name}); err != nil {
		t.Fatal(err)
	}
	return d
}
