package window

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/reading"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

type fixture struct {
	*desktoptest.Notesd
	t    *testing.T
	loop *desktoptest.Loop
	a    *App
	tt   *ui.Tester
}

// start draws the window on a real notesd, at a desktop size.
func start(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{Notesd: desktoptest.StartNotesd(t), t: t, loop: desktoptest.NewLoop()}
	f.a = New(Config{Env: f.loop, Version: "test",
		Dial: func(ctx context.Context) (*api.Client, *api.Hello, error) {
			return api.Dial(ctx, f.Srv.Socket, session.ClientName)
		},
		Blobs: f.Blobs, ConfigAt: t.TempDir(), StateAt: t.TempDir()})
	f.tt = ui.NewTester(func(c *ui.Context) { f.a.View(c) }, 1200, 800)
	f.a.Start()
	t.Cleanup(f.a.Session.Close)
	f.until("the connection", func() bool { return f.a.Session.State() == session.Connected })
	f.until("the vault list", func() bool { return f.a.listed })
	return f
}

// until draws frames until ok.
func (f *fixture) until(what string, ok func() bool) {
	f.t.Helper()
	f.loop.Until(f.t, what, func() bool {
		f.tt.Frame()
		return ok()
	})
	f.tt.Frame()
}

func (f *fixture) has(text string) bool { return f.tt.HasText(text) }

func (f *fixture) waitText(text string) {
	f.t.Helper()
	f.until(text, func() bool { return f.has(text) })
}

// openVault opens the fixture's vault from the vault manager.
func (f *fixture) openVault() {
	f.t.Helper()
	f.waitText("Create new vault")
	if err := f.tt.Click(f.a.vaults[0].Name); err != nil {
		f.t.Fatal(err)
	}
	f.until("the vault", func() bool { return f.a.vault != nil && f.a.settings != nil })
}

func TestAWindowWithoutAVaultShowsTheVaultManager(t *testing.T) {
	f := start(t)
	f.waitText("Create new vault")
	for _, want := range []string{"Open folder as vault", "Join a vault on a hub", "Frostnotes"} {
		if !f.has(want) {
			t.Errorf("no %q in %q", want, f.tt.Texts())
		}
	}
}

func TestOpeningAVaultRemembersIt(t *testing.T) {
	f := start(t)
	f.openVault()
	if f.a.Prefs.LastVault != f.Dir {
		t.Fatalf("last vault %q", f.a.Prefs.LastVault)
	}
}

func TestTheStatusBarLeadsWithTheConnection(t *testing.T) {
	f := start(t)
	f.openVault()
	if !f.has("Local") {
		t.Fatalf("no connection in %q", f.tt.Texts())
	}
}

func TestCreatingANoteAndTypingInIt(t *testing.T) {
	f := start(t)
	f.openVault()
	f.tt.Key(ui.Cmd, ui.KeyN)
	f.until("the new note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	if p := f.a.activeFile(); p != "Untitled.md" {
		t.Fatalf("the new note is %q", p)
	}
	if err := f.tt.Click("Note"); err != nil {
		t.Fatal(err)
	}
	f.tt.Type("hello")
	f.tt.Frame()
	f.loop.Advance(notes.SaveDelay)
	d := f.a.activeEditor().Binding().Document()
	f.until("the save", func() bool { return d.Status() == notes.StatusClean && !d.Unsaved() })
	if got := f.Read("Untitled.md").Content; got != "hello" {
		t.Fatalf("the file holds %q", got)
	}
}

func TestThePaletteAndQuickSwitcher(t *testing.T) {
	f := start(t)
	f.Create("Garden plan.md", "x\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("Garden plan.md") })
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	if !f.a.Shell.Palette.Open || f.a.Shell.Palette.ShownSource() != "" {
		t.Fatalf("Ctrl K: open %v, source %q", f.a.Shell.Palette.Open, f.a.Shell.Palette.ShownSource())
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyO)
	f.tt.Frame()
	if f.a.Shell.Palette.ShownSource() != sourceNotes {
		t.Fatalf("Ctrl O: source %q", f.a.Shell.Palette.ShownSource())
	}
	f.tt.Type("gard")
	f.tt.Frame()
	if !f.has("Garden plan") {
		t.Fatalf("no match in %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEnter)
	f.until("the note", func() bool { return f.a.activeFile() == "Garden plan.md" })
}

func TestTabsCloseAndReopen(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.openVault()
	f.a.open("a.md", workspace.Here)
	f.until("the note", func() bool { return f.a.activeEditor() != nil })
	f.tt.Key(ui.Cmd, ui.KeyW)
	f.until("the close", func() bool { return f.a.activeFile() == "" })
	if n := len(f.a.tabs); n > 1 {
		t.Fatalf("%d tab contents kept", n)
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
	f.until("the reopen", func() bool { return f.a.activeFile() == "a.md" })
}

func TestEveryCommandHasAnAction(t *testing.T) {
	f := start(t)
	for _, s := range f.a.table {
		if f.a.actions[s.ID] == nil {
			t.Errorf("%s does nothing", s.ID)
		}
	}
}

func TestSettingsChangeAPreferenceAndAVaultSetting(t *testing.T) {
	f := start(t)
	f.openVault()
	f.tt.Key(ui.Cmd, ui.KeyComma)
	f.waitText("Show inline title")
	f.a.settingChanged("pref:inlineTitle", false)
	if f.a.Prefs.InlineTitle {
		t.Fatal("the preference did not change")
	}
	f.a.settingChanged("vault:dailyFolder", "Journal")
	f.until("the vault's setting", func() bool { return f.a.settings.DailyFolder == "Journal" })
}

func TestAMovedNoteFollowsInItsTab(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.openVault()
	f.a.open("a.md", workspace.Here)
	f.until("the note", func() bool { return f.a.activeEditor() != nil })
	if _, err := f.Other.Note().Move(t.Context(), &api.NoteMoveParams{VaultID: f.Vault, Path: "a.md", To: "b.md"}); err != nil {
		t.Fatal(err)
	}
	f.until("the move", func() bool { return f.a.activeFile() == "b.md" })
}

func TestTheFileTabOfAnImage(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.open("missing.png", workspace.Here)
	f.until("the file tab", func() bool { return f.has("This file is not in the vault.") })
}

func TestConnectingShowsUntilNotesdAnswers(t *testing.T) {
	loop := desktoptest.NewLoop()
	a := New(Config{Env: loop, Version: "test", ConfigAt: t.TempDir(), StateAt: t.TempDir(),
		Dial: func(context.Context) (*api.Client, *api.Hello, error) { return nil, nil, errOffline }})
	tt := ui.NewTester(func(c *ui.Context) { a.View(c) }, 1200, 800)
	if !tt.HasText("Connecting to notesd…") {
		t.Fatalf("before the first try: %q", tt.Texts())
	}
	a.Start()
	t.Cleanup(a.Session.Close)
	loop.Until(t, "the failure", func() bool {
		tt.Frame()
		return a.Session.Reason() != ""
	})
	tt.Frame()
	if !tt.HasText("Connecting to notesd…") || !tt.HasText(errOffline.Error()) {
		t.Fatalf("after a failure: %q", tt.Texts())
	}
}

func TestAConnectionLostKeepsTheWorkspace(t *testing.T) {
	f := start(t)
	f.openVault()
	f.Srv.Stop()
	f.until("the loss", func() bool { return f.a.Session.State() != session.Connected })
	if !f.has("Files") {
		t.Fatalf("the workspace went: %q", f.tt.Texts())
	}
	if conn := f.a.connection(); conn.State != frostui.Reconnecting {
		t.Fatalf("the connection is %+v", conn)
	}
}

func TestAVaultTheEventListedFirstIsListedOnce(t *testing.T) {
	f := start(t)
	f.waitText("Create new vault")
	f.a.listVault(f.a.vaults[0]) // the answer to vault.add, after the event's reload
	f.tt.Frame()                 // two rows with one key would panic
	if len(f.a.vaults) != 1 {
		t.Fatalf("%d vaults", len(f.a.vaults))
	}
}

func TestCreatingOpeningAndRemovingVaults(t *testing.T) {
	f := start(t)
	f.waitText("Create new vault")
	parent := t.TempDir()
	if err := f.tt.Click("Create"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.a.vm.name, f.a.vm.location = "Fresh", parent
	f.tt.Frame()
	if err := f.tt.Click("Create"); err != nil {
		t.Fatal(err)
	}
	f.until("the new vault", func() bool { return f.a.vault != nil && f.a.vault.Name == "Fresh" })

	f.a.run("app:open-vault")
	f.waitText("Open folder as vault")
	if !f.a.vm.open {
		t.Fatal("the manager is not a dialog over the vault")
	}
	other := f.a.vaults[0]
	if other.Name == "Fresh" {
		other = f.a.vaults[1]
	}
	if err := f.tt.Click(other.Name); err != nil {
		t.Fatal(err)
	}
	f.until("the switch", func() bool { return f.a.vault != nil && f.a.vault.ID == other.ID })

	var fresh *api.Vault
	for i := range f.a.vaults {
		if f.a.vaults[i].Name == "Fresh" {
			fresh = &f.a.vaults[i]
		}
	}
	f.a.vm.remove(f.a, fresh)
	f.waitText("Its folder and files stay where they are.")
	if err := f.tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	f.until("the removal", func() bool { return len(f.a.vaults) == 1 })
}

func TestJoiningALoopbackHub(t *testing.T) {
	hub := rpctest.StartWith(t, rpctest.Options{HubListen: true})
	hc := hub.Dial(t)
	hv, err := hc.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir(), Name: new("Shared")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hc.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: hv.ID}); err != nil {
		t.Fatal(err)
	}
	f := start(t)
	f.openVault()
	f.a.run("app:open-settings")
	f.waitText("Show inline title")
	f.a.sets.layout.Selected = "sync"
	f.waitText("Sync with a hub")
	f.a.settingChanged("sync:join", true)
	f.tt.Frame()
	j := f.a.sets.joining
	j.url, j.name = hub.HubURL, "Shared"
	f.tt.Frame()
	if err := f.tt.Click("Join"); err != nil {
		t.Fatal(err)
	}
	f.until("the code", func() bool { return j.phase == joinApprove && j.code != "" })
	if !f.has("notesctl hub approve " + j.code) {
		t.Fatalf("no code shown: %q", f.tt.Texts())
	}
	if _, err := hc.Hub().Approve(t.Context(), &api.HubApproveParams{Code: j.code}); err != nil {
		t.Fatal(err)
	}
	f.until("the join", func() bool { return f.a.syncInfo != nil && f.a.sets.joining == nil && f.a.syncInfo.URL == hub.HubURL })
	if conn := f.a.connection(); conn.Host != hostOf(hub.HubURL) {
		t.Fatalf("the status bar shows %+v", conn)
	}
}

// TestEveryScreenDraws writes each screen's picture, light and dark:
// FROSTNOTES_SCREENS keeps them in a directory.
func TestEveryScreenDraws(t *testing.T) {
	dir := os.Getenv("FROSTNOTES_SCREENS")
	if dir == "" {
		dir = t.TempDir()
	}
	frostui.SetIcons(frostui.NewIcons(nil))
	for _, dark := range []bool{false, true} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		f := start(t)
		f.tt.SetDark(dark)
		f.Create("Garden plan.md", "---\ntags: [a]\n---\n# Garden\n\nSome **bold** text and a [[Link]].\n\n- [ ] task\n")
		f.Create("Ideas/Other.md", "x\n")
		save := func(name string) {
			t.Helper()
			f.tt.Frame()
			out, err := os.Create(filepath.Join(dir, name+"-"+mode+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = out.Close() }()
			if err := png.Encode(out, f.tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		f.waitText("Create new vault")
		save("vaults")
		f.openVault()
		f.until("the catalog", func() bool { return f.a.hasFile("Garden plan.md") })
		f.a.open("Garden plan.md", workspace.Here)
		f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
		save("editor")
		f.a.run("markdown:toggle-preview")
		f.until("the reading view", func() bool { return f.has("Garden") && f.a.activeEditor() == nil })
		f.loop.Advance(time.Second)
		f.until("the rendering", func() bool { return f.a.activeContent().read.Markdown() != "" })
		save("reading")
		f.a.run("command-palette:open")
		save("palette")
		f.a.Shell.Palette.Open = false
		f.tt.Frame()
		f.a.run("app:open-settings")
		save("settings")
		f.a.sets.layout.Open = false
		f.tt.Frame()
		f.a.Session.Close()
	}
}

func TestTheSidebarsFollowTheWorkspaceAndTheToggle(t *testing.T) {
	f := start(t)
	f.openVault()
	f.tt.Frame()
	if !f.a.panes.Shown(paneRight) || !f.a.panes.Shown(paneLeft) {
		t.Fatal("a sidebar starts hidden")
	}
	f.a.run("app:toggle-right-sidebar")
	f.tt.Frame()
	f.tt.Frame()
	if f.a.panes.Shown(paneRight) || f.a.Store.Workspace().Right.Open {
		t.Fatal("the right sidebar did not hide")
	}
	if err := f.tt.Click("Sidebar"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.tt.Frame()
	if f.a.Store.Workspace().Left.Open {
		t.Fatal("the bar's toggle did not reach the workspace")
	}
	f.a.run("app:toggle-left-sidebar")
	f.tt.Frame()
	f.tt.Frame()
	if !f.a.panes.Shown(paneLeft) {
		t.Fatal("the left sidebar did not come back")
	}
}

func TestARestoredImageTabShowsOnceTheFilesAreRead(t *testing.T) {
	f := start(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Other.Note().Upload(t.Context(), &api.NoteUploadParams{VaultID: f.Vault, Path: "a.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())}); err != nil {
		t.Fatal(err)
	}
	f.a.Store.Load(f.Dir)
	f.a.Store.Update(func(w *workspace.Workspace) {
		w.Open(workspace.View{Type: workspace.ViewFile, Path: "a.png"}, workspace.Here)
	})
	f.a.Store.Flush()
	f.openVault()
	f.until("the image", func() bool {
		tc := f.a.activeContent()
		return tc != nil && tc.img != nil
	})
	if f.has("This file is not in the vault.") {
		t.Fatalf("%q", f.tt.Texts())
	}
}

func TestLivePreviewInTheWindow(t *testing.T) {
	f := start(t)
	f.Create("a.md", "see [[b|the other note]]\n\n![[b]]\n\nlast\n")
	f.Create("b.md", "b's own text\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("b.md") })
	f.a.open("a.md", workspace.NewTab)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	e := f.a.activeEditor()
	end := len(e.Binding().Document().Text())
	e.State.Select(end, end)
	f.waitText("b's own text")
	if !f.has("Live Preview") {
		t.Fatalf("no mode in %q", f.tt.Texts())
	}
	if err := f.tt.Click("the other note"); err != nil {
		t.Fatal(err)
	}
	f.until("the link's note", func() bool { return f.a.activeFile() == "b.md" })
}

func TestSourceModeInTheWindow(t *testing.T) {
	f := start(t)
	f.Create("a.md", "**bold**\n\nlast\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("a.md") })
	f.a.Prefs.EditingMode = "source"
	f.a.open("a.md", workspace.NewTab)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	e := f.a.activeEditor()
	e.State.Select(11, 11)
	f.tt.Frame()
	if got := e.State.Replacements(); len(got) != 0 {
		t.Fatalf("source mode replaced %+v", got)
	}
	if !f.has("Source mode") {
		t.Fatalf("no mode in %q", f.tt.Texts())
	}
	f.a.Prefs.EditingMode = "live"
	f.a.prefsChanged()
	f.tt.Frame()
	f.tt.Frame()
	if got := e.State.Replacements(); len(got) == 0 {
		t.Fatal("live preview replaced nothing after the preference changed")
	}
}

func TestDeletingAFileCanBeUndone(t *testing.T) {
	f := start(t)
	f.Create("a.md", "text\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("a.md") })
	f.a.Prefs.ConfirmDelete = false
	f.a.deleteFile("a.md", false)
	f.waitText("Deleted a.")
	if _, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "a.md"}); err == nil {
		t.Fatal("the file is still there")
	}
	if err := f.tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	f.loop.Until(t, "the restore", func() bool {
		f.tt.Frame()
		_, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "a.md"})
		f.loop.Post(func() {})
		return err == nil
	})
	if got := f.Read("a.md").Content; got != "text\n" {
		t.Fatalf("restored %q", got)
	}
}

func TestTheBreadcrumbRevealsAFolder(t *testing.T) {
	f := start(t)
	f.Create("Projects/Garden/Plan.md", "x\n")
	f.openVault()
	f.a.open("Projects/Garden/Plan.md", workspace.Here)
	f.until("the note", func() bool { return f.a.activeEditor() != nil })
	f.a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "search") })
	f.tt.Frame()
	if err := f.tt.Click("Reveal Garden"); err != nil {
		t.Fatalf("%v in %q", err, f.tt.Texts())
	}
	f.tt.Frame()
	w := f.a.Store.Workspace()
	if w.Left.Panes[w.Left.Active] != "files" || !slices.Contains(w.Files.Expanded, "Projects") {
		t.Fatalf("pane %q, expanded %q", w.Left.Panes[w.Left.Active], w.Files.Expanded)
	}
}

// heldUploads answers an upload when the test says.
type heldUploads struct{ done func(string, error) }

func (u *heldUploads) Upload(_ []editor.File, done func(string, error)) { u.done = done }

func (f *fixture) openNoteTab(p string) *editor.NoteEditor {
	f.t.Helper()
	f.until("the catalog", func() bool { return f.a.hasFile(p) })
	f.a.open(p, workspace.NewTab)
	f.until("the note", func() bool {
		e := f.a.activeEditor()
		return f.a.activeFile() == p && e != nil && e.Binding().Document().Loaded()
	})
	return f.a.activeEditor()
}

func TestAnUploadOutlastsAModeSwitch(t *testing.T) {
	f := start(t)
	f.Create("a.md", "text\n")
	f.openVault()
	e := f.openNoteTab("a.md")
	up := &heldUploads{}
	e.Uploads = up
	e.Attach([]editor.File{{Name: "p.png", Data: []byte("x")}})
	f.a.toggleMode() // the only tab of the note leaves edit mode
	f.until("the reading view", func() bool { return f.a.activeEditor() == nil })
	up.done("![[p.png]]", nil)
	f.tt.Frame()
	f.loop.Advance(notes.SaveDelay)
	f.loop.Until(t, "the save", func() bool {
		f.tt.Frame()
		f.loop.Post(func() {})
		return strings.Contains(f.Read("a.md").Content, "![[p.png]]")
	})
}

func TestClosingCommitsAFocusedProperty(t *testing.T) {
	f := start(t)
	f.Create("a.md", "---\nstatus: draft\n---\nbody\n")
	f.openVault()
	f.openNoteTab("a.md")
	f.tt.Frame()
	if err := f.tt.Click("status"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("final")
	f.tt.Frame()
	closed := false
	f.a.RequestClose(func() { closed = true })
	f.until("the close", func() bool { return closed })
	if got := f.Read("a.md").Content; !strings.Contains(got, "status: final") {
		t.Fatalf("file holds %q", got)
	}
}

func TestClosingSeesWhatWasTypedDuringTheFlush(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.Create("b.md", "b\n")
	f.openVault()
	a := f.openNoteTab("a.md")
	b := f.openNoteTab("b.md")
	b.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "typed "})
	b.Binding().Sync()
	closed := false
	f.a.RequestClose(func() { closed = true })
	a.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "late "}) // while b saves
	f.waitText("Close anyway?")
	if closed {
		t.Fatal("closed with what was typed unsaved")
	}
}

func TestAPagePreviewsLinkFollowsTheOpenRules(t *testing.T) {
	f := start(t)
	f.openVault()
	var opened []string
	f.a.cfg.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	f.a.previews["a.md\x00b"] = &preview{text: "see [mail](mailto:a@e.test?attach=/etc/passwd) here", done: true}
	draw := f.a.linkPreview(func() string { return "a.md" })
	tt := ui.NewTester(func(c *ui.Context) { draw(c, editor.Link{Target: "b"}) }, 400, 300)
	if err := tt.Click("mail"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 {
		t.Fatalf("the link did not go through the app's rules: %q", opened)
	}
}

func TestALinkResolvedAfterAVaultSwitchIsDropped(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	other, err := f.Other.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("a.md") })
	f.a.followLink(editor.Link{Target: "New note"}, "a.md", editor.WhereHere)
	before := f.a.vault
	f.a.vault = other  // the answer comes after the switch
	for i := range 3 { // the resolve's answer, and a create that would follow it
		done := false
		f.a.call(func(ctx context.Context, c *api.Client) error {
			_, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: other.ID, Path: "Sentinel " + strconv.Itoa(i) + ".md"})
			return err
		}, func(error) { done = true })
		f.loop.Until(t, "a round trip", func() bool { return done })
	}
	f.a.vault = before
	files, err := f.Other.Note().Files(t.Context(), &api.NoteFilesParams{VaultID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(files.Files, func(v api.VaultFile) bool { return v.Path == "New note.md" }) {
		t.Fatalf("the note went to the vault switched to: %+v", files.Files)
	}
}

func TestDeletingADirtyNoteStaysDeleted(t *testing.T) {
	for _, tc := range []struct{ name, del string }{{"the note", "Dir/a.md"}, {"its folder", "Dir"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := start(t)
			f.Create("Dir/a.md", "text\n")
			f.openVault()
			e := f.openNoteTab("Dir/a.md")
			e.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "typed "})
			e.Binding().Sync()
			f.a.Prefs.ConfirmDelete = false
			f.a.deleteFile(tc.del, tc.del == "Dir")
			f.loop.Until(t, "the delete", func() bool {
				f.tt.Frame()
				f.loop.Post(func() {})
				_, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "Dir/a.md"})
				return err != nil
			})
			d := e.Binding().Document()
			f.until("the deleted notice", func() bool { return d.Notice() != nil && d.Notice().Kind == notes.NoticeDeleted })
			if _, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "Dir/a.md"}); err == nil {
				t.Fatal("the note came back")
			}
		})
	}
}

func TestClosingATabCommitsAFocusedProperty(t *testing.T) {
	f := start(t)
	f.Create("a.md", "---\nstatus: draft\n---\nbody\n")
	f.openVault()
	f.openNoteTab("a.md")
	f.tt.Frame()
	if err := f.tt.Click("status"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyA)
	f.tt.Type("final")
	f.tt.Frame()
	id := f.a.Store.Workspace().ActiveTab().ID
	f.a.Store.Update(func(w *workspace.Workspace) { w.Close(id) })
	f.until("the save", func() bool { return strings.Contains(f.Read("a.md").Content, "status: final") })
}

func TestClosingCommitsAPropertyTypedDuringTheFlush(t *testing.T) {
	f := start(t)
	f.Create("a.md", "---\nstatus: draft\n---\nbody\n")
	f.Create("b.md", "b\n")
	f.openVault()
	b := f.openNoteTab("b.md")
	a := f.openNoteTab("a.md")
	f.tt.Frame()
	if err := f.tt.Click("status"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	b.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "typed "})
	b.Binding().Sync()
	closed := false
	f.a.RequestClose(func() { closed = true })
	f.tt.Key(ui.Cmd, ui.KeyA) // while b saves
	f.tt.Type("final")
	f.tt.Frame()
	f.waitText("Close anyway?")
	if closed || !strings.Contains(a.Binding().Document().Text(), "status: final") {
		t.Fatalf("closed %v, the note holds %q", closed, a.Binding().Document().Text())
	}
}

func TestACreateAnsweredAfterAVaultSwitchOpensNothing(t *testing.T) {
	f := start(t)
	other, err := f.Other.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f.openVault()
	f.a.createAndOpen("New note.md", workspace.Here, nil)
	before := f.a.vault
	f.a.vault = other // the answer comes after the switch
	f.loop.Until(t, "the create", func() bool {
		_, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: before.ID, Path: "New note.md"})
		f.loop.Post(func() {})
		return err == nil
	})
	for range 3 {
		done := false
		f.a.call(func(ctx context.Context, c *api.Client) error {
			_, err := c.Note().Files(ctx, &api.NoteFilesParams{VaultID: other.ID})
			return err
		}, func(error) { done = true })
		f.loop.Until(t, "a round trip", func() bool { return done })
	}
	if p := f.a.Store.Workspace().ActiveTab().View.Path; p == "New note.md" {
		t.Fatal("the note opened in the vault switched to")
	}
	f.a.vault = before
}

func TestADeleteStopsWhenANoteCannotBeSaved(t *testing.T) {
	f := start(t)
	f.Create("Dir/a.md", "text\n")
	f.openVault()
	e := f.openNoteTab("Dir/a.md")
	dir := filepath.Join(f.Dir, "Dir")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	e.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "typed "})
	e.Binding().Sync()
	f.a.Prefs.ConfirmDelete = false
	f.a.deleteFile("Dir/a.md", false)
	f.loop.Until(t, "the refusal", func() bool { return len(f.a.toasts) > 0 }) // drawn frames take them
	if got := f.a.toasts[0].Title; got != "Cannot delete a: some of its text could not be saved." {
		t.Fatalf("it said %q", got)
	}
	d := e.Binding().Document()
	if d.ReadOnly() != "" {
		t.Fatalf("the note stays read-only: %q", d.ReadOnly())
	}
	if _, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "Dir/a.md"}); err != nil {
		t.Fatalf("the note is gone: %v", err)
	}
}

func TestANoteTakesNoEditsWhileItIsBeingDeleted(t *testing.T) {
	f := start(t)
	f.Create("a.md", "text\n")
	f.openVault()
	e := f.openNoteTab("a.md")
	e.Binding().Buffer().Edit(code.Edit{Start: 0, End: 0, Text: "typed "})
	e.Binding().Sync()
	f.a.Prefs.ConfirmDelete = false
	f.a.deleteFile("a.md", false)
	d := e.Binding().Document()
	if d.ReadOnly() == "" {
		t.Fatal("the note can be edited during its delete")
	}
	f.tt.Frame()
	if !e.State.ReadOnly {
		t.Fatal("the editor takes typing during the delete")
	}
	f.until("the deleted notice", func() bool { return d.Notice() != nil && d.Notice().Kind == notes.NoticeDeleted })
	f.until("the answer", func() bool { return d.ReadOnly() == "" })
	if _, err := f.Other.Note().Read(t.Context(), &api.NoteReadParams{VaultID: f.Vault, Path: "a.md"}); err == nil {
		t.Fatal("the note came back")
	}
}

func TestANewNoteWaitsForTheVaultsSettings(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.settingChanged("vault:newNoteLocation", "folder")
	f.until("the location", func() bool { return f.a.settings.NewNoteLocation == api.NewNoteLocationFolder })
	f.a.settingChanged("vault:newNoteFolder", "Inbox")
	f.until("the folder", func() bool { return f.a.settings.NewNoteFolder == "Inbox" })
	v := *f.a.vault
	f.a.showVault(&v)
	if f.a.settings != nil {
		t.Fatal("the settings are already here")
	}
	f.a.createNote(workspace.Here)
	f.until("the new note", func() bool { return f.a.activeFile() != "" })
	if p := f.a.activeFile(); p != "Inbox/Untitled.md" {
		t.Fatalf("the new note is %q", p)
	}
}

func TestAnOversizedImageBlobIsRefused(t *testing.T) {
	f := start(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Other.Note().Upload(t.Context(), &api.NoteUploadParams{VaultID: f.Vault, Path: "a.png", Data: base64.StdEncoding.EncodeToString(data.Bytes())}); err != nil {
		t.Fatal(err)
	}
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("a.png") })
	hash := f.a.fileHash("a.png")
	blob := filepath.Join(f.Blobs, hash[:2], hash)
	if err := os.Chmod(blob, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(blob, reading.MaxImageFile+1); err != nil { // sparse: takes no disk
		t.Fatal(err)
	}
	f.a.Store.Update(func(w *workspace.Workspace) {
		w.Open(workspace.View{Type: workspace.ViewFile, Path: "a.png"}, workspace.Here)
	})
	f.until("the refusal", func() bool {
		tc := f.a.activeContent()
		return tc != nil && strings.Contains(tc.err, "larger than")
	})
}

func TestANewNoteAsksAgainWhenTheSettingsRequestFailed(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.settings, f.a.settingsAsked = nil, false // the first request failed
	f.a.createNote(workspace.Here)
	f.until("the new note", func() bool { return f.a.activeFile() == "Untitled.md" })
}

func TestAddingAPropertyWaitsOutAFreeze(t *testing.T) {
	f := start(t)
	f.Create("a.md", "text\n")
	f.openVault()
	e := f.openNoteTab("a.md")
	d := e.Binding().Document()
	thaw := d.Freeze("Busy.")
	defer thaw()
	f.a.addFileProperty()
	if d.Text() != "text\n" {
		t.Fatalf("the note holds %q", d.Text())
	}
	if len(f.a.toasts) != 1 || f.a.toasts[0].Title != "Busy." {
		t.Fatalf("it said %+v", f.a.toasts)
	}
}

func TestAReadingViewLinkScrollsToItsHeading(t *testing.T) {
	f := start(t)
	f.Create("b.md", "text\n\n## The Goals\n\nmore\n")
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("b.md") })
	f.a.Prefs.DefaultView = workspace.ModeRead
	f.a.followNote("b.md", "#The Goals", false)
	f.until("the heading", func() bool {
		tc := f.a.activeContent()
		return tc != nil && tc.read != nil && f.a.pending == nil && tc.read.Options.Anchor == "the-goals"
	})
}

func TestANewVaultForgetsTheOldSettings(t *testing.T) {
	f := start(t)
	f.openVault()
	v := *f.a.vault
	f.a.showVault(&v)
	if f.a.settings != nil {
		t.Fatal("the old settings stayed for the new vault")
	}
	f.until("the settings", func() bool { return f.a.settings != nil })
}

func TestTheRightSidebarButtonWorksInAFrame(t *testing.T) {
	f := start(t)
	f.openVault()
	f.tt.Frame()
	if !f.a.Store.Workspace().Right.Open {
		t.Fatal("it starts closed")
	}
	if err := f.tt.Click("Toggle right sidebar"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.tt.Frame()
	if f.a.Store.Workspace().Right.Open {
		t.Fatal("the click was undone")
	}
}

func TestAPressInAGroupMakesItActive(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.Create("b.md", "b\n")
	f.openVault()
	f.openNoteTab("a.md")
	f.until("the second note", func() bool { return f.a.hasFile("b.md") })
	f.a.open("b.md", workspace.ToRight)
	f.until("b", func() bool {
		e := f.a.activeEditor()
		return f.a.activeFile() == "b.md" && e != nil && e.Binding().Document().Loaded()
	})
	f.tt.Frame()
	first := func() ui.Rect {
		var out ui.Rect
		for _, n := range f.tt.AccessAll() {
			if n.Name == "Note" && (out.W == 0 || n.Bounds.X < out.X) {
				out = n.Bounds
			}
		}
		return out
	}()
	if first.W == 0 {
		t.Fatal("no editor")
	}
	f.tt.ClickAt(first.X+first.W/2, first.Y+first.H/2)
	f.tt.Frame()
	f.tt.Frame()
	if f.a.activeFile() != "a.md" {
		t.Fatalf("a click in a's editor left %q active", f.a.activeFile())
	}
	// A press on the other group's shown tab makes it active too.
	var tab ui.Rect
	for _, n := range f.tt.AccessAll() {
		if n.Role == ui.RoleTab && n.Name == "b" {
			tab = n.Bounds
		}
	}
	f.tt.ClickAt(tab.X+16, tab.Y+tab.H/2)
	f.tt.Frame()
	if f.a.activeFile() != "b.md" {
		t.Fatalf("a press on b's tab left %q active", f.a.activeFile())
	}
}

func TestRemovingAVaultSaysWhatIsForgotten(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.vm.remove(f.a, &f.a.vaults[0])
	f.waitText("Its folder and files stay where they are. Frostnotes forgets its index and version history on this device.")
}

func TestResetLeavesNoHiddenEditor(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.openVault()
	f.a.Store.Update(func(w *workspace.Workspace) { w.Open(workspace.Note("a.md", workspace.ModeEdit), workspace.Here) })
	f.until("the note", func() bool { e := f.a.activeEditor(); return e != nil && e.Binding().Document().Loaded() })
	f.a.run("frostnotes:reset-workspace-layout")
	f.tt.Frame()
	if f.a.activeEditor() != nil {
		t.Fatal("the old editor is still the active one")
	}
}

func TestALinkOpensAtItsHeading(t *testing.T) {
	f := start(t)
	f.Create("a.md", "see [[b#Two]] and [[#Local]]\n\n# Local\n")
	f.Create("b.md", "# One\n\ntext\n\n# Two\n")
	f.openVault()
	e := f.openNoteTab("a.md")
	f.a.followLink(editor.Link{Target: "", Subpath: "#Local"}, "a.md", editor.WhereHere)
	f.tt.Frame()
	f.tt.Frame()
	if got := e.Binding().Buffer().Pos(e.State.Cursor()).Line; got != 2 {
		t.Fatalf("own heading: line %d", got)
	}
	f.a.followLink(editor.Link{Target: "b", Subpath: "#Two"}, "a.md", editor.WhereTab)
	f.until("b at its heading", func() bool {
		b := f.a.activeEditor()
		return f.a.activeFile() == "b.md" && b != nil && b.Binding().Document().Loaded() && f.a.pending == nil && b.Binding().Buffer().Pos(b.State.Cursor()).Line == 4
	})
}

func TestAnEventBeforeTheVaultListIsNotLost(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.openVault()
	f.openNoteTab("a.md")
	f.a.listed = false
	seen := f.a.State.Seq
	f.a.Session.OnSeq(seen + 5)
	f.a.event(1, api.NoteMoved{VaultID: f.Vault, From: "a.md", To: "z.md"})
	if len(f.a.early) != 1 {
		t.Fatal("the event was dropped")
	}
	if f.a.State.Seq != seen {
		t.Fatal("a close now would never replay the held event")
	}
	f.a.reloadAll()
	f.until("the replay", func() bool { return f.a.listed && len(f.a.early) == 0 })
	if f.a.State.Seq != seen+5 {
		t.Fatalf("sequence %d after the replay", f.a.State.Seq)
	}
	if f.a.Store.Workspace().ActiveTab().View.Path != "z.md" {
		t.Fatalf("not followed: %+v", f.a.Store.Workspace().ActiveTab().View)
	}
}

func TestSyncMissingOnlyForThatAnswer(t *testing.T) {
	if !syncMissing(&api.Error{Code: api.CodeMethodNotFound}) || !syncMissing(&api.Error{Code: api.CodeUnavailable}) {
		t.Error("a notesd without sync was not recognized")
	}
	if syncMissing(errors.New("connection lost")) || syncMissing(nil) {
		t.Error("a lost connection read as no sync")
	}
}

func TestACleanupFailureIsNotACancel(t *testing.T) {
	err := cleanupFailed(context.Canceled, "Notes", errors.New("gone"))
	if errors.Is(err, context.Canceled) {
		t.Fatal("the cleanup failure would be dropped as a cancel")
	}
	if !strings.Contains(err.Error(), "Notes") {
		t.Fatalf("%v", err)
	}
}

func TestACancelDoesNotCutTheAddShort(t *testing.T) {
	f := start(t)
	dir := filepath.Join(t.TempDir(), "new")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := joinRun{c: f.Other, ctx: ctx, folder: dir}
	v, err := r.addVault()
	if err != nil || v == nil {
		t.Fatalf("vault %v, err %v", v, err)
	}
}

func TestStatusEditingModeMenuChoosesEachMode(t *testing.T) {
	f := start(t)
	f.Create("a.md", "text\n")
	f.openVault()
	f.a.open("a.md", workspace.NewTab)
	f.until("the note", func() bool { return f.a.activeEditor() != nil && f.a.activeEditor().Binding().Document().Loaded() })
	for _, name := range []string{"Reading view", "Source mode", "Live Preview"} {
		if err := f.tt.Click("Change the editing mode"); err != nil {
			t.Fatal(err)
		}
		if err := f.tt.ChooseMenuItem(name); err != nil {
			t.Fatal(err)
		}
		f.tt.Frame()
		if got := f.a.modeName(f.a.Store.Workspace().ActiveTab().View); got != name {
			t.Fatalf("got %q, want %q", got, name)
		}
	}
}

func TestHistoryRestoresCaretAndScroll(t *testing.T) {
	f := start(t)
	f.Create("a.md", strings.Repeat("A long line of text\n", 150))
	f.Create("b.md", "second\n")
	f.openVault()
	e := f.openNoteTab("a.md")
	e.State.Select(1200, 1200)
	e.State.SetScrollY(500)
	f.tt.Frame()
	scroll := e.State.ScrollY()
	f.a.open("b.md", workspace.Here)
	f.until("b", func() bool {
		e := f.a.activeEditor()
		return e != nil && e.Binding().Document().Loaded() && f.a.activeFile() == "b.md"
	})
	f.a.updateWorkspace(func(w *workspace.Workspace) { w.Back() })
	f.until("restored caret", func() bool { e := f.a.activeEditor(); return e != nil && e.State.Cursor() == 1200 })
	if got := f.a.activeEditor().State.ScrollY(); got != scroll {
		t.Fatalf("scroll %v, want %v", got, scroll)
	}
	f.a.updateWorkspace(func(w *workspace.Workspace) { w.Forward() })
	f.tt.Frame()
	if f.a.activeFile() != "b.md" {
		t.Fatal("forward did not return to b")
	}
}

func TestLineJumpTargetsTheNewTab(t *testing.T) {
	f := start(t)
	f.Create("a.md", "one\ntwo\nthree\n")
	f.openVault()
	first := f.openNoteTab("a.md")
	before := first.State.Cursor()
	f.a.openAtLine("a.md", 3, workspace.ToRight)
	f.until("the jump", func() bool { return f.a.pending == nil })
	if first.State.Cursor() != before {
		t.Fatal("the earlier tab took the jump")
	}
	if f.a.activeEditor().State.CursorPos().Line != 2 {
		t.Fatal("new tab missed the jump")
	}
}

func TestHiddenPropertiesHideTheSource(t *testing.T) {
	f := start(t)
	f.Create("a.md", "---\na: b\n---\nbody\n")
	f.openVault()
	f.a.Prefs.Properties = "hidden"
	e := f.openNoteTab("a.md")
	f.tt.Frame()
	if !e.HideFront {
		t.Fatal("hidden properties left YAML visible")
	}
	tc := f.a.tabs[f.a.Store.Workspace().ActiveTab().ID]
	if tc.ed.ShowProperties {
		t.Fatal("hidden properties showed the panel")
	}
	f.a.setPref("properties", "source")
	f.tt.Frame()
	if e.HideFront {
		t.Fatal("source mode hid YAML")
	}
}

func TestClosingSettingsCommitsDraft(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.sets.show("daily")
	f.tt.Frame()
	f.a.sets.drafts["vault:dailyFolder"] = "Journal"
	f.a.sets.layout.Open = false
	f.tt.Frame()
	f.until("the draft to reach notesd", func() bool { return f.a.settings.DailyFolder == "Journal" })
}

func TestHeldEventsAtZeroSurviveRestart(t *testing.T) {
	f := start(t)
	f.Create("a.md", "a\n")
	f.openVault()
	f.openNoteTab("a.md")
	f.a.Store.Flush()
	f.a.State.Seq, f.a.listed = 0, false
	f.a.Session.OnSeq(7)
	f.a.event(7, api.NoteMoved{VaultID: f.Vault, From: "a.md", To: "z.md"})
	f.a.event(8, api.NoteRemoved{VaultID: f.Vault, Path: "gone.md"})
	f.a.Session.OnSeq(8)
	f.a.saveState()
	next := New(f.a.cfg)
	if len(next.early) != 2 || next.Session.Seq() != 8 {
		t.Fatalf("held %v, resume %d", next.early, next.Session.Seq())
	}
	next.vaults = f.a.vaults
	next.Store.Load(f.Dir)
	for _, ev := range next.early {
		next.followEarly(ev)
	}
	if got := next.Store.Workspace().ActiveTab().View.Path; got != "z.md" {
		t.Fatalf("restored path %q", got)
	}
}

func TestLostAddAnswerExplainsPossibleVault(t *testing.T) {
	f := start(t)
	if err := f.Other.Close(); err != nil {
		t.Fatal(err)
	}
	r := joinRun{c: f.Other, folder: "/made-up/vault"}
	_, err := r.addVault()
	if err == nil || !strings.Contains(err.Error(), "Manage vaults") || !strings.Contains(err.Error(), r.folder) {
		t.Fatalf("lost answer: %v", err)
	}
}

type heldJoinLoop struct {
	*desktoptest.Loop
	posted chan func()
}

func (l *heldJoinLoop) Post(fn func()) { l.posted <- fn }

func TestAnOldJoinCannotFinishTheNewRun(t *testing.T) {
	f := start(t)
	loop := &heldJoinLoop{Loop: f.loop, posted: make(chan func(), 10)}
	f.a.cfg.Env = loop
	j := &joinFlow{mode: modePair, url: ":invalid"}
	j.start(f.a)
	var old func()
	select {
	case old = <-loop.posted:
	case <-t.Context().Done():
		t.Fatal("no join answer")
	}
	j.start(f.a)
	old()
	if j.phase != joinJoining || j.err != "" {
		t.Fatal("old answer replaced the new join")
	}
	select {
	case fn := <-loop.posted:
		fn()
	case <-t.Context().Done():
		t.Fatal("no second answer")
	}
	if j.phase != joinForm || j.err == "" {
		t.Fatal("current error not shown")
	}
}
