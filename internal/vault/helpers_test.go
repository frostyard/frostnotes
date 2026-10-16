package vault

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/store"
)

// testVault is a vault with a database and blob store of its own, whose
// actor does not run: tests scan and write directly, so nothing races
// them.
type testVault struct {
	t      *testing.T
	v      *Vault
	db     *store.DB
	blobs  *blobs.Store
	dir    string
	clock  *time.Time
	events *[]api.EventEnvelope
	mtime  time.Time // the next outside edit's modification time
}

var testStart = time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)

func newTestVault(t *testing.T, files map[string]string, tune func(*Config)) *testVault {
	t.Helper()
	dir := makeVault(t, files)
	return openTestVault(t, dir, t.TempDir(), tune)
}

// openTestVault registers dir in a database under data.
func openTestVault(t *testing.T, dir, data string, tune func(*Config)) *testVault {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(data, "notesd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clock := testStart
	db.Now = func() time.Time { return clock }
	var events []api.EventEnvelope
	db.OnCommit = func(evs []api.EventEnvelope) { events = append(events, evs...) }
	vaults, err := db.ListVaults(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var sv store.Vault
	if len(vaults) > 0 {
		sv = vaults[0]
	} else if err := db.Tx(t.Context(), func(tx *store.Tx) error {
		sv, err = tx.InsertVault(t.Context(), "v", dir)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DB: db, Blobs: blobs.New(filepath.Join(data, "blobs")), Now: func() time.Time { return clock }}
	if tune != nil {
		tune(&cfg)
	}
	cfg = cfg.withDefaults()
	events = nil
	return &testVault{t: t, v: newVault(&cfg, sv), db: db, blobs: cfg.Blobs, dir: dir, clock: &clock, events: &events,
		mtime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (tv *testVault) scan() {
	tv.t.Helper()
	if _, err := tv.v.runScan(tv.t.Context(), false); err != nil {
		tv.t.Fatalf("scan: %v", err)
	}
}

// edit writes a file as another program would, with a modification time
// of its own, so a scan's comparison of size and time sees it.
func (tv *testVault) edit(p, content string) {
	tv.t.Helper()
	full := filepath.Join(tv.dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		tv.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		tv.t.Fatal(err)
	}
	tv.mtime = tv.mtime.Add(time.Second)
	if err := os.Chtimes(full, tv.mtime, tv.mtime); err != nil {
		tv.t.Fatal(err)
	}
}

func (tv *testVault) disk(p string) string {
	tv.t.Helper()
	data, err := os.ReadFile(filepath.Join(tv.dir, filepath.FromSlash(p)))
	if err != nil {
		tv.t.Fatal(err)
	}
	return string(data)
}

func (tv *testVault) exists(p string) bool {
	_, err := os.Lstat(filepath.Join(tv.dir, filepath.FromSlash(p)))
	return err == nil
}

// row returns the index's row for p, or a zero File.
func (tv *testVault) row(p string) store.File {
	tv.t.Helper()
	f, err := tv.v.row(tv.t.Context(), p)
	if err != nil {
		tv.t.Fatal(err)
	}
	if f == nil {
		return store.File{}
	}
	return *f
}

func (tv *testVault) versions(fileID int64) []store.Version {
	tv.t.Helper()
	vs, err := tv.db.Versions(tv.t.Context(), fileID, 100)
	if err != nil {
		tv.t.Fatal(err)
	}
	return vs
}

// links renders a note's links as "target -> resolved" lines.
func (tv *testVault) links(p string) string {
	tv.t.Helper()
	ls, err := tv.db.Links(tv.t.Context(), tv.row(p).ID)
	if err != nil {
		tv.t.Fatal(err)
	}
	var b strings.Builder
	for _, l := range ls {
		dest := ""
		if l.Resolved != nil {
			dest = *l.Resolved
		}
		b.WriteString(l.Target + l.Subpath + " -> " + dest + "\n")
	}
	return b.String()
}

// takeEvents returns the events committed since the last call, as
// "name path" lines (note.moved: "note.moved from to").
func (tv *testVault) takeEvents() []string {
	tv.t.Helper()
	var out []string
	for _, e := range *tv.events {
		var d struct {
			Path   string `json:"path"`
			From   string `json:"from"`
			To     string `json:"to"`
			Source string `json:"source"`
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			tv.t.Fatal(err)
		}
		switch e.Event {
		case "note.moved":
			out = append(out, e.Event+" "+d.From+" "+d.To)
		case "note.changed":
			out = append(out, e.Event+" "+d.Path+" "+d.Source)
		default:
			out = append(out, e.Event+" "+d.Path)
		}
	}
	*tv.events = nil
	return out
}

var editor = Author{Source: api.VersionSourceEditor, Name: "test"}
var agent = Author{Source: api.VersionSourceAgent, Name: "an agent"}
