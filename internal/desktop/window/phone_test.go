//go:build integration

package window

import (
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// An edit on the phone, which goes to the hub, reaches the desktop's open
// editor within 2 s (M3's evidence, plan 0009, Phase 6).
func TestAnEditOnThePhoneReachesTheDesktop(t *testing.T) {
	hub := rpctest.StartWith(t, rpctest.Options{HubListen: true})
	hc := hub.Dial(t)
	name := "Shared"
	hv, err := hc.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir(), Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	created, err := hc.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: hv.ID, Path: "Log.md", Content: "one\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hc.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: hv.ID}); err != nil {
		t.Fatal(err)
	}

	f := start(t)
	f.openVault()
	j := &joinFlow{mode: modeJoin, phase: joinForm, vaultID: f.a.vault.ID, url: hub.HubURL, name: name}
	f.a.sets.joining = j
	j.onDone = f.a.closeJoining
	j.start(f.a)
	f.until("the code", func() bool { return j.phase == joinApprove && j.code != "" })
	if _, err := hc.Hub().Approve(t.Context(), &api.HubApproveParams{Code: j.code}); err != nil {
		t.Fatal(err)
	}
	f.until("the join", func() bool { return f.a.sets.joining == nil })
	f.a.Catalog.Reload()
	f.loop.UntilWithin(t, "the first pull", time.Minute, func() bool {
		f.tt.Frame()
		f.loop.Advance(time.Second)
		return f.a.hasFile("Log.md")
	})
	f.a.open("Log.md", workspace.Here)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })

	began := time.Now()
	if _, err := hc.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: hv.ID, Path: "Log.md", Base: created.Hash, Content: "one\nfrom the phone\n"}); err != nil {
		t.Fatal(err)
	}
	d := f.a.activeEditor().Binding().Document()
	f.loop.UntilWithin(t, "the edit", 10*time.Second, func() bool {
		f.tt.Frame()
		f.loop.Advance(notes.SaveDelay)
		return d.Text() == "one\nfrom the phone\n"
	})
	took := time.Since(began)
	t.Logf("the edit took %v", took)
	if took > 2*time.Second {
		t.Errorf("the edit took %v", took)
	}
	if got := f.a.activeEditor().Binding().Buffer().String(); got != "one\nfrom the phone\n" {
		t.Fatalf("the editor shows %q", got)
	}
}
