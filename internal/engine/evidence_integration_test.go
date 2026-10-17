//go:build integration

package engine_test

import (
	"encoding/json/v2"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/vault"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// TestIVimSaveIsSearchable is M1's exit evidence 3 (plan 0003, Phase 7): a
// note saved in vim is found by note.search, as notesctl search calls it,
// within 1 s, with the watcher's default 100 ms debounce. It edits ten
// notes of a 666-note generated vault with a real vim and times each from
// the moment vim exits to the search's answer. Run it with make
// m1-evidence.
func TestIVimSaveIsSearchable(t *testing.T) {
	vim, err := exec.LookPath("vim")
	if err != nil {
		t.Skip("needs vim")
	}
	dir := filepath.Join(t.TempDir(), "vault")
	m, err := vaultgen.Generate(dir, vaultgen.Options{Seed: 3, Notes: 666})
	if err != nil {
		t.Fatal(err)
	}
	av := openVaultWith(t, dir, vault.Config{})
	var slowest time.Duration
	for i := range 10 {
		p := m.Notes[i*61]
		word := fmt.Sprintf("medlarberry%d", i)
		cmd := exec.Command(vim, "-Nu", "NONE", "-i", "NONE", "-es", "-c", "normal! Go"+word, "-c", "wq", filepath.Join(dir, p))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("vim: %v\n%s", err, out)
		}
		saved := time.Now()
		av.waitFor("note.changed for "+p, func(env api.EventEnvelope) bool {
			var ev api.NoteChanged
			return env.Event == "note.changed" && json.Unmarshal(env.Data, &ev) == nil && ev.Path == p
		})
		hits, err := av.c.Note().Search(t.Context(), &api.NoteSearchParams{VaultID: av.id, Query: word})
		took := time.Since(saved)
		if err != nil || len(hits) != 1 || hits[0].Path != p {
			t.Fatalf("search for %s after the save = %+v, %v", word, hits, err)
		}
		slowest = max(slowest, took)
		t.Logf("%s: searchable %v after vim saved it", p, took.Round(time.Millisecond))
	}
	if slowest > time.Second {
		t.Errorf("the slowest edit was searchable after %v, more than 1 s", slowest)
	}
}
