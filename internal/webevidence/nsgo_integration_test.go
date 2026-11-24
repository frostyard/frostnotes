//go:build integration

package webevidence_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// The NSGo frame talks to the real Lodge gateway and notesd. Disallowing
// the disposable node closes its WebSocket while keeping the Unix client
// available. Only the fixture's allow-list changes.
func TestNSGoLodgeReconnect(t *testing.T) {
	w := startWorld(t)
	b := startBrowser(t, "nsgo", 1160, 760)
	b.navigate(w.url + "/notes")
	b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 30*time.Second))
	b.await(30*time.Second, "Garden in the NSGo note list", func() bool {
		return b.truthy(`return [...document.querySelectorAll("[role=button]")].some(el=>el.textContent.trim()==="Garden")`)
	})
	b.script(nil, `[...document.querySelectorAll("[role=button]")].find(el=>el.textContent.trim()==="Garden").click()`)
	waitText := func(want string) {
		t.Helper()
		b.await(15*time.Second, "open editor to show "+want, func() bool {
			return b.truthy(`return document.querySelector("textarea")?.value === arguments[0]`, want)
		})
	}
	edit := func(text string) {
		t.Helper()
		b.click(b.waitFind("textarea", 5*time.Second))
		b.chord(keyControl, "a")
		b.keys(text)
	}
	ctx := t.Context()
	writeNative := func(text string) {
		t.Helper()
		n, err := w.hub.C.Note().Read(ctx, &api.NoteReadParams{VaultID: w.hub.Vault, Path: "Garden.md"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.hub.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: w.hub.Vault, Path: n.Path, Base: n.Hash, Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	waitText(garden)
	edit("Browser draft")
	eventually(t, 10*time.Second, "browser autosave on disk", func() bool { return w.disk("Garden.md") == "Browser draft" })
	writeNative("Oak")
	waitText("Oak")

	w.ctl(t, "disallow", "node", "e2e-node")
	b.await(15*time.Second, "disconnected editor", func() bool { return b.truthy(`return document.body.innerText.includes("Connecting to notesd")`) })
	const draft = "Fir"
	edit(draft)
	waitText(draft)
	b.await(10*time.Second, "offline autosave to report the pending draft", func() bool { return b.truthy(`return document.body.innerText.includes("Could not save")`) })
	if got := w.disk("Garden.md"); got != "Oak" {
		t.Fatalf("disconnected browser changed disk to %q", got)
	}

	writeNative("Elm")
	w.ctl(t, "allow", "node", "e2e-node")
	waitText("Elm")
	b.await(15*time.Second, "conflict notice", func() bool { return b.truthy(`return document.body.innerText.includes("Edits overlapped")`) })
	versions, err := w.hub.C.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: w.hub.Vault, Path: "Garden.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		if !v.Conflict {
			continue
		}
		saved, err := w.hub.C.Note().ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: w.hub.Vault, Version: v.ID})
		if err != nil {
			t.Fatal(err)
		}
		if saved.Content == draft && w.disk("Garden.md") == "Elm" && b.truthy(`return document.body.innerText.includes(arguments[0])`, fmt.Sprintf("version %d.", v.ID)) {
			t.Logf("Verified the stored note and conflict version %d. Screenshot %s", v.ID, b.screenshot("reconnected"))
			return
		}
	}
	t.Fatal("the notice, stored file and recovered draft disagree")
}
