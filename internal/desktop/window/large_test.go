//go:build integration

package window

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// budget is a frame at 60 Hz.
const budget = 16 * time.Millisecond

// A generated vault of 10,000 notes shows its tree and ranks the
// switcher's notes with no frame over 16 ms (plan 0009, Phase 4). A note
// of the time goes to the log: the Tester draws on the CPU, so a frame
// here costs more than one on a GPU.
func TestALargeVaultStaysWithinAFrame(t *testing.T) {
	f := start(t)
	dir := filepath.Join(t.TempDir(), "large")
	if _, err := vaultgen.Generate(dir, vaultgen.Options{Notes: 10000, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	v, err := f.App.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	// The scan reports through vault.scan events, which reach the loop.
	scanned := false
	f.a.Session.OnEvent = func(seq int64, ev api.Event) {
		f.a.event(seq, ev)
		if s, ok := ev.(api.VaultScan); ok && s.Status.VaultID == v.ID {
			t.Logf("scan: %s, %d of %d, %d notes", s.Status.State, s.Status.Done, s.Status.Total, s.Status.Notes)
			scanned = s.Status.State == api.ScanStateIdle
		}
	}
	f.loop.UntilWithin(t, "the scan", 15*time.Minute, func() bool { return scanned })
	f.a.reloadAll()
	f.until("the list", func() bool { return len(f.a.vaults) == 2 })
	for i := range f.a.vaults {
		if f.a.vaults[i].ID == v.ID {
			f.a.showVault(&f.a.vaults[i])
		}
	}
	f.until("the catalog", func() bool { return len(f.a.Catalog.Files()) >= 9000 })
	t.Logf("%d files", len(f.a.Catalog.Files()))
	f.loop.Advance(time.Minute)
	f.tt.Frame()

	measure := func(what string, step func()) {
		t.Helper()
		var times []time.Duration
		for range 30 {
			start := time.Now()
			step()
			times = append(times, time.Since(start))
		}
		slices.Sort(times)
		worst := times[len(times)-1]
		t.Logf("%s: median %v, worst %v", what, times[len(times)/2], worst)
		if worst > budget {
			t.Errorf("%s: a frame took %v", what, worst)
		}
	}
	measure("the tree", func() { f.tt.Frame() })

	f.tt.Key(ui.Cmd, ui.KeyO)
	f.tt.Frame()
	letters := []string{"n", "o", "t", "e", "0", "1", "2"}
	i := 0
	measure("the switcher", func() {
		f.a.Shell.Palette.Query = letters[i%len(letters)]
		if i%2 == 1 {
			f.a.Shell.Palette.Query = letters[i%len(letters)] + letters[(i+1)%len(letters)]
		}
		i++
		f.tt.Frame()
	})
}
