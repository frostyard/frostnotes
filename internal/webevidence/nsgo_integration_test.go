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

// Drive the mounted WebAssembly app through Firefox's pointer and keyboard.
func TestNSGoLodgeInline(t *testing.T) {
	w := startWorld(t)
	b := startBrowser(t, "nsgo-inline", 1160, 760)
	b.navigate(w.url + "/notes")
	b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 30*time.Second))
	click := func(label string) {
		t.Helper()
		b.click(b.waitFind(fmt.Sprintf("[aria-label=%q]", label), 20*time.Second))
	}
	click("Garden")
	b.await(10*time.Second, "source loaded", func() bool {
		return b.truthy(`return document.querySelector("textarea")?.value===arguments[0]`, garden)
	})
	const md = "# Garden\n- [ ] prune\nSee [[Other|orchard]]."
	b.click(b.waitFind("textarea", time.Second))
	b.chord(keyControl, "a")
	b.keys(md)
	click("Live preview")
	click("Task on line 2")
	eventually(t, 10*time.Second, "inline checkbox saved", func() bool { return w.disk("Garden.md") == "# Garden\n- [x] prune\nSee [[Other|orchard]]." })
	click("Edit line 2")
	waitField := func(value string) {
		t.Helper()
		b.await(5*time.Second, "active Markdown line "+value, func() bool {
			return b.truthy(`return document.activeElement?.tagName==="TEXTAREA" && document.activeElement.value===arguments[0]`, value)
		})
	}
	waitField("- [x] prune")
	b.chord("\ue013") // WebDriver ArrowUp
	waitField("# Garden")
	b.chord("\ue010") // End
	b.keys("!")
	waitField("# Garden!")
	eventually(t, 10*time.Second, "inline typing saved", func() bool { return w.disk("Garden.md") == "# Garden!\n- [x] prune\nSee [[Other|orchard]]." })
	b.chord(keyControl, "z")
	waitField("# Garden")
	eventually(t, 10*time.Second, "source undo after typing", func() bool { return w.disk("Garden.md") == "# Garden\n- [x] prune\nSee [[Other|orchard]]." })
	t.Log("Inline editor:", b.screenshot("garden"))
	// Another writer changes the same heading while its raw line is active.
	// The next undo must never replay the previous edit over the new base.
	current, err := w.hub.C.Note().Read(t.Context(), &api.NoteReadParams{VaultID: w.hub.Vault, Path: "Garden.md"})
	if err != nil {
		t.Fatal(err)
	}
	const remote = "# Orchard\n- [x] prune\nSee [[Other|orchard]]."
	_, err = w.hub.C.Note().Write(t.Context(), &api.NoteWriteParams{VaultID: w.hub.Vault, Path: current.Path, Base: current.Hash, Content: remote})
	if err != nil {
		t.Fatal(err)
	}
	click("Edit line 1")
	waitField("# Orchard")
	b.chord(keyControl, "z")
	waitField("# Orchard")
	click("Open Other")
	b.await(10*time.Second, "opened linked note", func() bool { return b.truthy(`return document.body.innerText.includes("Pears by the wall.")`) })
}

// A second client creates a nested note while the browser is already open.
// Discover it by its contents in Lodge, edit it, and read it back via RPC.
func TestNSGoLodgeFind(t *testing.T) {
	w := startWorld(t)
	b := startBrowser(t, "nsgo-find", 1160, 760)
	b.navigate(w.url + "/notes")
	b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 30*time.Second))
	click := func(label string) {
		t.Helper()
		b.click(b.waitFind(fmt.Sprintf("[aria-label=%q]", label), 10*time.Second))
	}
	click("Garden")
	const p = "Plants/Field notes.md"
	const content = "A scarlet quince"
	_, err := w.hub.C.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: w.hub.Vault, Path: p, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	click("Search vault")
	b.click(b.waitFind("input[aria-label='Find note']", time.Second))
	b.keys("scarlet")
	b.await(10*time.Second, "indexed body match", func() bool {
		return b.truthy(`return [...document.querySelectorAll('[role=option]')].some(el=>el.textContent.includes(arguments[0]))`, p)
	})
	b.chord("\ue007") // Return chooses the highlighted match.
	b.await(5*time.Second, "selected result", func() bool {
		return b.truthy(`return document.querySelector("textarea")?.value===arguments[0]`, content)
	})
	b.click(b.waitFind("textarea", time.Second))
	b.chord(keyControl, "a")
	b.keys("Shared harvest")
	eventually(t, 10*time.Second, "other client reads browser edit", func() bool {
		n, err := w.hub.C.Note().Read(t.Context(), &api.NoteReadParams{VaultID: w.hub.Vault, Path: p})
		return err == nil && n.Content == "Shared harvest"
	})
}

func TestNSGoLodgeManage(t *testing.T) {
	w := startWorld(t)
	b := startBrowser(t, "nsgo-manage", 1160, 760)
	b.navigate(w.url + "/notes")
	b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 30*time.Second))
	click := func(label string) {
		t.Helper()
		b.click(b.waitFind(fmt.Sprintf("[aria-label=%q]", label), 10*time.Second))
	}
	click("Garden")
	b.await(8*time.Second, "Garden loaded", func() bool {
		return b.truthy(`return document.querySelector("textarea")?.value===arguments[0]`, garden)
	})
	b.click(b.waitFind("textarea", time.Second))
	b.chord(keyControl, "a")
	b.keys("draft")
	if w.disk("Garden.md") != garden {
		t.Fatal("expected an unsaved browser draft at rename")
	}
	_, err := w.hub.C.Note().Move(t.Context(), &api.NoteMoveParams{VaultID: w.hub.Vault, Path: "Garden.md", To: "Renamed.md"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, 8*time.Second, "dirty draft saved under the remote name", func() bool { return w.disk("Renamed.md") == "draft" })
	click("Rename or move note")
	click("Destination path")
	b.chord(keyControl, "a")
	b.keys("Plants/Orchard")
	click("Move")
	eventually(t, 8*time.Second, "browser move", func() bool { return w.disk("Plants/Orchard.md") == "draft" })
	click("Move to trash")
	click("Trash note")
	b.await(8*time.Second, "trashed editor closed", func() bool { return b.truthy(`return document.body.innerText.includes("Select a note to edit")`) })
	click("Restore deleted note")
	click("Deleted note path")
	b.keys("Plants/Orchard.md")
	click("Restore")
	b.await(8*time.Second, "restored editor", func() bool { return b.truthy(`return document.querySelector("textarea")?.value==="draft"`) })
	if w.disk("Plants/Orchard.md") != "draft" {
		t.Fatal("restored bytes differ")
	}
}
