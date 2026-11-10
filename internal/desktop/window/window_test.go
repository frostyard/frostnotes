package window

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
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
	if items := f.a.statusStart(); len(items) == 0 || !strings.HasSuffix(items[0].Text, "Local") {
		t.Fatalf("status starts %+v", items)
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
	if !f.a.Palette.Open || f.a.mode != "" {
		t.Fatalf("Ctrl K: open %v, mode %q", f.a.Palette.Open, f.a.mode)
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyO)
	f.tt.Frame()
	if f.a.mode != modeNotes {
		t.Fatalf("Ctrl O: mode %q", f.a.mode)
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
	if want := "Reconnecting to notesd… (" + errOffline.Error() + ")"; !tt.HasText(want) {
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
	if items := f.a.statusStart(); items[0].Tone == 0 {
		t.Fatalf("the status bar shows no warning: %+v", items[0])
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
	f.a.sets.section = "sync"
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
	if !strings.HasSuffix(f.a.statusStart()[0].Text, hostOf(hub.HubURL)) {
		t.Fatalf("the status bar shows %+v", f.a.statusStart()[0])
	}
}

// TestEveryScreenDraws writes each screen's picture, light and dark:
// FROSTNOTES_SCREENS keeps them in a directory.
func TestEveryScreenDraws(t *testing.T) {
	dir := os.Getenv("FROSTNOTES_SCREENS")
	if dir == "" {
		dir = t.TempDir()
	}
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
		f.a.Palette.Open = false
		f.tt.Frame()
		f.a.run("app:open-settings")
		save("settings")
		f.a.sets.open = false
		f.tt.Frame()
		f.a.Session.Close()
	}
}
