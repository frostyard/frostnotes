package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// testNow is the fixed clock of openTest databases.
var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// openTest opens a migrated database in a temp dir with a fixed clock, and
// collects committed events into the returned slice pointer.
func openTest(t *testing.T) (*DB, *[]api.EventEnvelope) {
	t.Helper()
	d, err := Open(t.Context(), filepath.Join(t.TempDir(), "notesd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	d.Now = func() time.Time { return testNow }
	var events []api.EventEnvelope
	d.OnCommit = func(evs []api.EventEnvelope) { events = append(events, evs...) }
	return d, &events
}
