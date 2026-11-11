//go:build integration

package webevidence_test

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/evidence"
)

// The notes the hub's vault starts with; Links.md's link is to the shell's
// own origin, which the web app opens through the shell (lodge-client's
// open), back in notes.
const (
	garden = "# Garden\n\nRows of quince.\n"
	other  = "# Other\n\nPears by the wall.\n"
	phone  = "# Phone\n\nTyped on the phone:\n"
)

// png is a 1 × 1 PNG, the image the evidence pastes.
const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// sizes are the evidence's two windows: a desktop's and a phone's (plan
// 0006: 390 × 844 until Phase 7).
var sizes = []struct {
	name          string
	width, height int
}{
	{"desktop", 1280, 800},
	{"phone", 390, 844},
}

// world is lodge's development build on loopback with notesd --lodge as
// notes and the hub, its vault seeded.
type world struct {
	t        *testing.T // the test the processes belong to, past any subtest
	root     string
	url      string // lodge's origin
	content  string // lodge-content's origin
	lodgectl string
	notesd   string
	hub      *evidence.Notesd
	log      *lockedBuffer
}

// startWorld lays out lodge's root with its shell and notes' web build,
// starts the development build, and notesd --lodge as the hub.
func startWorld(t *testing.T) *world {
	t.Helper()
	e2e, web := os.Getenv("LODGE_E2E"), os.Getenv("NOTES_WEB")
	if e2e == "" || web == "" {
		t.Skip("LODGE_E2E (lodge's development build) and NOTES_WEB (notes' install tree) are make m4-evidence's")
	}
	// Unix socket paths are at most 108 bytes: a short root.
	root, err := os.MkdirTemp("", "m4e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.CopyFS(filepath.Join(root, "usr/share/lodge/shell"), os.DirFS(filepath.Join(e2e, "shell"))); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(root, "usr/share/lodge/apps/notes"), os.DirFS(web)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "run/lodge/apps"), 0o755); err != nil {
		t.Fatal(err)
	}
	listen, content := evidence.FreeAddr(t), evidence.FreeAddr(t)
	w := &world{t: t, root: root, url: "http://" + listen, content: "http://" + content,
		lodgectl: filepath.Join(e2e, "lodgectl"), log: &lockedBuffer{}}
	cmd := exec.Command(filepath.Join(e2e, "lodge-e2e"), "--root", root, "--listen", listen, "--content-listen", content)
	cmd.Stdout, cmd.Stderr = w.log, w.log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	})
	eventually(t, 30*time.Second, "lodge's control socket", func() bool {
		_, err := os.Stat(filepath.Join(root, "run/lodge/control.sock"))
		return err == nil
	})
	w.ctl(t, "allow", "login", "e2e@example.com")
	w.ctl(t, "allow", "node", "e2e-node")

	w.notesd = evidence.Build(t, "github.com/frostyard/frostnotes/cmd/notesd")
	w.hub = evidence.StartEnv(t, w.notesd, "hub", []string{"LODGE_ROOT=" + root}, "-lodge")
	for name, text := range map[string]string{
		"Garden.md": garden,
		"Other.md":  other,
		"Phone.md":  phone,
		"Links.md":  "# Links\n\nSee [other](" + w.url + "/notes/1/Other.md).\n",
	} {
		if err := os.WriteFile(filepath.Join(w.hub.Dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := t.Context()
	if id := w.hub.AddVault(ctx, t); id != 1 {
		t.Fatalf("the hub's vault is %d; Links.md names vault 1", id)
	}
	if _, err := w.hub.C.Hub().Serve(ctx, &api.HubServeParams{VaultID: w.hub.Vault}); err != nil {
		t.Fatal(err)
	}
	w.ctl(t, "reload")
	eventually(t, 30*time.Second, "notes in lodge", func() bool {
		resp, err := http.Get(w.url + "/app/notes/")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})
	return w
}

func (w *world) ctl(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command(w.lodgectl, args...)
	cmd.Env = append(os.Environ(), "LODGE_ROOT="+w.root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lodgectl %s: %v\n%s\nlodge:\n%s", strings.Join(args, " "), err, out, w.log)
	}
}

// disk is the hub's copy of a note.
func (w *world) disk(name string) string {
	data, _ := os.ReadFile(filepath.Join(w.hub.Dir, filepath.FromSlash(name)))
	return string(data)
}

// openNote loads a note's canonical URL in the shell and switches into
// notes' frame once its editor shows want.
func (w *world) openNote(t *testing.T, b *browser, name, want string) {
	t.Helper()
	b.top()
	b.navigate(w.url + "/notes/1/" + url.PathEscape(name))
	b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 30*time.Second))
	w.waitEditor(t, b, want)
}

// waitEditor waits until the frame's editor holds want.
func (w *world) waitEditor(t *testing.T, b *browser, want string) {
	t.Helper()
	b.await(30*time.Second, fmt.Sprintf("the editor to hold %q", want), func() bool {
		var text string
		b.script(&text, `const el = document.querySelector(".cm-content"); return el ? el.innerText : "";`)
		return strings.Contains(text, want)
	})
}

// leaveEditor takes the focus from the frame's editor.
func leaveEditor(b *browser) {
	b.t.Helper()
	b.script(nil, `if (document.activeElement instanceof HTMLElement) document.activeElement.blur();`)
}

// typeAtEnd types text at the end of the frame's note: Ctrl+End, then the
// text's keys, as a person types.
func typeAtEnd(b *browser, text string) {
	b.t.Helper()
	b.click(b.waitFind(".cm-content", 10*time.Second))
	b.chord(keyControl, keyEnd)
	b.keys(text)
}

// TestIWebAppInLodge is M4's exit evidence 1 on loopback and Phase 5's
// "done when", at a desktop size and at 390 × 844: notes' web build in
// lodge's development build opens a note from its canonical URL, edits it
// onto the hub's disk, shows an outside edit, finds a note with the quick
// switcher, creates one, pastes an image and shows it from lodge-content,
// follows a lodge link through open, lets the switcher's key reach the
// shell, and comes back after notesd restarts.
func TestIWebAppInLodge(t *testing.T) {
	w := startWorld(t)
	for _, size := range sizes {
		t.Run(size.name, func(t *testing.T) {
			b := startBrowser(t, size.name, size.width, size.height)
			mark := func(what string) string { return fmt.Sprintf("%s on the %s", what, size.name) }

			// A note opens from its canonical URL, at the window's width.
			w.openNote(t, b, "Garden.md", "Rows of quince.")
			var width int
			b.script(&width, "return window.innerWidth;")
			t.Logf("%s: notes' frame is %d wide", size.name, width)
			narrow := width < 600
			if narrow != (size.name == "phone") {
				t.Fatalf("a %d-wide frame is narrow=%v", width, narrow)
			}

			// An edit reaches the hub's disk.
			edit := mark(" edited")
			typeAtEnd(b, edit)
			b.await(10*time.Second, "the edit on the hub's disk", func() bool { return strings.Contains(w.disk("Garden.md"), edit) })

			// An outside edit shows in the editor.
			outside := mark("Written outside")
			if err := os.WriteFile(filepath.Join(w.hub.Dir, "Garden.md"), []byte(w.disk("Garden.md")+outside+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			w.waitEditor(t, b, outside)

			// The quick switcher finds a note. A narrow window's bottom bar
			// shows once no editor has the focus, as after a tap elsewhere.
			leaveEditor(b)
			b.click(b.waitFind(`button[aria-label="Open quick switcher"]`, 10*time.Second))
			input := b.waitFind(`[role="dialog"] input`, 10*time.Second)
			b.typeInto(input, "Other"+keyEnter)
			w.waitEditor(t, b, "Pears by the wall.")

			// A new tab's Create new note makes a note on the hub's disk.
			before := notes(t, w.hub.Dir)
			leaveEditor(b)
			b.click(b.waitFind(`button[aria-label="New tab"]`, 10*time.Second))
			b.click(b.waitFind(`//button[starts-with(normalize-space(.), "Create new note")]`, 10*time.Second))
			created := mark("Created")
			b.await(10*time.Second, "a new note", func() bool { return len(notes(t, w.hub.Dir)) > len(before) })
			typeAtEnd(b, created)
			b.await(10*time.Second, "the new note's text on the hub's disk", func() bool {
				for _, n := range notes(t, w.hub.Dir) {
					if !slices.Contains(before, n) && strings.Contains(w.disk(n), created) {
						return true
					}
				}
				return false
			})

			// An image put into the note is uploaded and shows from lodge-content.
			w.openNote(t, b, "Garden.md", "Rows of quince.")
			b.click(b.waitFind(".cm-content", 10*time.Second))
			pasted := pastedImages(t, w.hub.Dir)
			// A paste where the browser keeps the file in a made clipboard
			// event; else a drop, which the editor takes the same way.
			var how string
			b.script(&how, `
				const bytes = Uint8Array.from(atob(arguments[0]), (c) => c.charCodeAt(0));
				const dt = new DataTransfer();
				dt.items.add(new File([bytes], "image.png", { type: "image/png" }));
				const target = document.querySelector(".cm-content");
				const paste = new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true });
				if (paste.clipboardData && paste.clipboardData.files.length > 0) {
					target.dispatchEvent(paste);
					return "paste";
				}
				const r = target.getBoundingClientRect();
				target.dispatchEvent(new DragEvent("drop", { dataTransfer: dt, bubbles: true, cancelable: true,
					clientX: r.left + 10, clientY: r.bottom - 5 }));
				return "drop";`, png)
			t.Logf("%s: the image went in by a %s", size.name, how)
			b.await(15*time.Second, "the pasted image on the hub's disk", func() bool {
				return len(pastedImages(t, w.hub.Dir)) > len(pasted)
			})
			// Away from the embed's line, live preview draws it.
			b.chord(keyControl, keyHome)
			b.await(15*time.Second, "the image drawn from lodge-content", func() bool {
				return b.truthy(`return [...document.querySelectorAll(".cm-content img")].some(
					(img) => img.src.startsWith(arguments[0]) && img.complete && img.naturalWidth > 0);`, w.content)
			})

			// A link to the shell's own origin opens through the shell, in notes.
			w.openNote(t, b, "Links.md", "See")
			b.click(b.waitFind(".cm-pv-link", 10*time.Second))
			b.top()
			b.frame(b.waitFind(`iframe.frame[title="Notes"]`, 10*time.Second))
			w.waitEditor(t, b, "Pears by the wall.")

			// The switcher's key reaches the shell from inside the frame.
			b.click(b.waitFind(".cm-content", 10*time.Second))
			b.chord(keyControl, "`")
			b.top()
			b.await(10*time.Second, "lodge's switcher", func() bool {
				return b.truthy(`const s = document.querySelector('[role="dialog"][aria-label="Switch app"]'); return !!s && !s.hidden;`)
			})
			b.chord(keyEscape)

			// notesd restarts: the page comes back and edits again.
			w.hub.Restart(w.t)
			w.openNote(t, b, "Garden.md", "Rows of quince.")
			again := mark(" after a restart")
			typeAtEnd(b, again)
			b.await(20*time.Second, "an edit after notesd restarted", func() bool { return strings.Contains(w.disk("Garden.md"), again) })
		})
	}
}

// notes lists dir's markdown files.
func notes(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	return out
}

// pastedImages lists dir's pasted images.
func pastedImages(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "Pasted image *.png"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
