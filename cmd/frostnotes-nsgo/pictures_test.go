//go:build linux && integration

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The picture tours name a picture at each main view of the client on a
// hand-written vault. A headless run keeps no pixels; on a target
// (make nsgo-live) each name is a PNG, and comparison/linux holds the last
// set a person kept (make nsgo-pictures). No test compares pixels.

// pictureVault is the tours' vault: made-up notes in two folders, with the
// marks live preview draws.
var pictureVault = map[string]string{
	"Welcome.md":        "# Welcome\n\nA small vault for the pictures of the client.\n\nStart with [[Quince]] or the [[Seed list]].\n",
	"Seed list.md":      "# Seed list\n\n- [x] Borage\n- [ ] Chervil\n- [ ] Sorrel\n",
	"Plants/Quince.md":  "# Quince\n\nA scarlet quince grows by the gate.\n\n## Care\n\n- [x] Mulch in autumn\n- [ ] Prune in March\n- [ ] Thin the fruit\n\nIt shares a bed with [[Mint]] and [[Plants/Fig|the fig]].\n\n```\nrows: 3\nspacing: 4 m\n```\n",
	"Plants/Mint.md":    "# Mint\n\nKeep it in a pot, or it takes the bed.\n",
	"Plants/Fig.md":     "# Fig\n\nBrown Turkey, against the south wall.\n",
	"Journal/Monday.md": "# Monday\n\nPlanted the quince. Rain by noon.\n",
}

// The size of the app's own window (main.go), and a width under both compact
// limits: the shell's 720 and the explorer's 620.
const (
	pictureWidth  core.Points = 1160
	pictureHeight core.Points = 760
	narrowWidth   core.Points = 600
)

// pictureApp starts the client on files in light or dark. On a target NSGo
// takes the appearance from GTK's settings, which follow the host, unless the
// test pins it, and the pictures must not follow the host.
func pictureApp(t *testing.T, files map[string]string, width core.Points, dark bool) *nsgotest.App {
	t.Helper()
	app, _ := startVault(t, files, width, pictureHeight, nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1, Dark: dark}))
	return app
}

func pictureWorkspace(t *testing.T, width core.Points, dark bool) *nsgotest.App {
	t.Helper()
	app := pictureApp(t, pictureVault, width, dark)
	app.WaitFor("label:Quince", 10*time.Second)
	return app
}

func TestPicturesOfANote(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, false)
	app.Screenshot("empty")
	app.Press("label:Quince")
	app.WaitFor("text:Saved", 10*time.Second)
	app.Screenshot("note-source")
	app.Press("label:Live preview")
	app.WaitFor("label:Edit line 1", 5*time.Second)
	app.Screenshot("note-live-preview")
	app.Press("label:Edit line 9")
	app.WaitFor("label:Markdown line", 5*time.Second)
	app.Screenshot("note-live-preview-line")
}

func TestPicturesOfDialogs(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, false)
	app.Press("label:Quince")
	app.WaitFor("text:Saved", 10*time.Second)
	app.Press("label:New note")
	app.WaitFor("label:Note path", 5*time.Second)
	app.Type("label:Note path", "Plants/Medlar")
	app.Screenshot("dialog-new-note")
	app.Press("label:Cancel")
	app.Press("label:Rename or move note")
	app.WaitFor("label:Destination path", 5*time.Second)
	app.Screenshot("dialog-move-note")
	app.Press("label:Cancel")
	app.Press("label:Move to trash")
	app.WaitFor("label:Trash note", 5*time.Second)
	app.Screenshot("dialog-trash-note")
	app.Press("label:Cancel")
	app.Press("label:Restore deleted note")
	app.WaitFor("label:Deleted note path", 5*time.Second)
	app.Screenshot("dialog-restore-note")
}

func TestPicturesOfQuickOpen(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, false)
	app.Key("mod+o")
	app.WaitFor("label:Find note", 5*time.Second)
	app.Screenshot("quick-open")
	app.Type("label:Find note", "plants")
	app.Screenshot("quick-open-query")
}

func TestPicturesOfVaultSearch(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, false)
	app.Key("mod+shift+f")
	app.WaitFor("label:Find note", 5*time.Second)
	app.Type("label:Find note", "scarlet")
	// The search waits for the app's debounce timer.
	waitTimers(t, app, 5*time.Second, func() bool { return strings.Contains(app.Dump(), "A scarlet quince grows") })
	app.Screenshot("vault-search")
}

func TestPicturesOfVaults(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, false)
	app.Press("label:Manage vaults")
	app.WaitFor("label:Create new vault", 5*time.Second)
	app.Screenshot("vault-manager")
	app.Press("label:Join a vault on a hub")
	app.WaitFor("label:Hub URL", 5*time.Second)
	app.Type("label:Hub URL", "https://hub.example.test")
	app.Type("label:Hub vault name", "Orchard")
	app.Screenshot("vault-join")
}

// A notesd with no vault: what a first start shows.
func TestPicturesOfNoVault(t *testing.T) {
	app := pictureApp(t, nil, pictureWidth, false)
	app.WaitFor("label:Create new vault", 10*time.Second)
	app.WaitFor("text:No vaults available", 5*time.Second)
	app.Screenshot("no-vault")
}

// A compact window shows one pane: the tree, or the note with a way back,
// or the vaults behind the Navigation button.
func TestPicturesOfANarrowWindow(t *testing.T) {
	app := pictureWorkspace(t, narrowWidth, false)
	app.Screenshot("narrow-tree")
	app.Press("label:Quince")
	app.WaitFor("text:Saved", 10*time.Second)
	app.Screenshot("narrow-note")
	app.Press("label:Navigation")
	app.WaitFor("label:Vault", 5*time.Second)
	app.Screenshot("narrow-navigation")
}

// The same views in Frostyard, the dark theme: a note in source and in live
// preview, a dialog with a field, and the vault manager, which holds the
// choice of appearance.
func TestPicturesInDark(t *testing.T) {
	app := pictureWorkspace(t, pictureWidth, true)
	app.Press("label:Quince")
	app.WaitFor("text:Saved", 10*time.Second)
	app.Screenshot("dark-note-source")
	app.Press("label:Live preview")
	app.WaitFor("label:Edit line 1", 5*time.Second)
	app.Screenshot("dark-note-live-preview")
	app.Press("label:Rename or move note")
	app.WaitFor("label:Destination path", 5*time.Second)
	app.Screenshot("dark-dialog-move-note")
	app.Press("label:Cancel")
	app.Press("label:Manage vaults")
	app.WaitFor("label:Create new vault", 5*time.Second)
	app.Screenshot("dark-vault-manager")
}

func TestPicturesOfANarrowWindowInDark(t *testing.T) {
	app := pictureWorkspace(t, narrowWidth, true)
	app.Press("label:Quince")
	app.WaitFor("text:Saved", 10*time.Second)
	app.Screenshot("dark-narrow-note")
}
