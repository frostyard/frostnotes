//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/system"
	"github.com/frostyard/frostnotes/internal/desktop/vaults"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

const (
	settingsLabel = "Settings"

	createVault = "Create new vault"
	openVault   = "Open folder as vault"
	joinVault   = "Join a vault on a hub"
	linkVault   = "Sync this vault with a hub"
	pairAgain   = "Pair again"
	forgetVault = "Remove vault from list"
)

func isVaultAction(a string) bool {
	switch a {
	case createVault, openVault, joinVault, linkVault, pairAgain, forgetVault:
		return true
	}
	return false
}

// settingsButton is the way to Settings, at the navigation's foot.
func (w *workspace) settingsButton() []core.Node {
	return []core.Node{ui.IconButton{Label: settingsLabel, Icon: lucide.Settings, Selected: w.settings.Get, OnPress: func() { w.settings.Set(!w.settings.GetUntracked()) }}}
}

// bindSettingsKey binds mod+, to Settings (the kit's standard for an app,
// rule 7).
func (w *workspace) bindSettingsKey() {
	w.cx.OnKey("mod+,", func() { w.settings.Set(true) })
}

// vaultWelcome is what a device with no vault shows: the 3 ways to get
// one, the first of them the pane's one filled button.
func (w *workspace) vaultWelcome() core.Node {
	start := func(action string, variant ui.ButtonVariant) core.Node {
		return ui.Button{Text: action, Variant: variant, OnPress: func() { w.action.Set(action) }}
	}
	return core.View{TestID: "welcome", Style: ui.FillRest, Children: []core.Node{workspaceui.EmptyState{
		Icon: lucide.Library, Title: "Open a vault", Hint: "A vault is a folder of Markdown notes.",
		Actions: []core.Node{start(createVault, ui.ButtonPrimary), start(openVault, ui.ButtonSecondary), start(joinVault, ui.ButtonSecondary)}}}}
}

// settingsView is Settings: NSGo's settings view, shown in place of the
// vault. Its rows start the vault's forms, which are dialogs.
func (w *workspace) settingsView(cx *core.Scope) core.Node {
	background, hasBackground := system.GetBackground(os.Getenv)
	keep := core.NewSignal(cx, background.KeepRunning)
	category := core.NewSignal(cx, "settings")
	narrow := func() bool { return cx.Env().Window().Width < 720 }
	start := func(text, action string) core.Node {
		return ui.Button{Text: text, Label: action, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, OnPress: func() { w.action.Set(action) }}
	}
	return core.Dynamic(func(*core.Scope) core.Node {
		groups := []ui.SettingsGroup{{ID: "vaults", Title: "Vaults", Items: []ui.SettingsItem{
			{ID: "create", Title: "New vault", Description: "Make a folder on this device for a new vault.", Control: start("Create…", createVault)},
			{ID: "open", Title: "Existing folder", Description: "Use a folder of Markdown files as a vault.", Control: start("Open…", openVault)},
			{ID: "join", Title: "Vault on a hub", Description: "Pair this device with a hub and take a vault from it.", Control: start("Join…", joinVault)},
		}}}
		if w.vault.Get() != 0 {
			sync := ui.SettingsItem{ID: "sync", Title: "Sync", Description: "This vault is on this device alone.", Control: start("Set up…", linkVault)}
			if s := w.syncInfo.Get(); s != nil && s.State == api.SyncStateRevoked {
				sync.Description, sync.Control = "The hub no longer accepts this device.", start("Pair again…", pairAgain)
			} else if s != nil {
				sync.Description, sync.Control = fmt.Sprintf("%s on %s", s.HubVault, s.URL), nil
			}
			groups = append(groups, ui.SettingsGroup{ID: "vault", Title: w.title(), Items: []ui.SettingsItem{sync,
				{ID: "forget", Title: "Remove from this device", Description: "Takes the vault off this device’s list. Its folder and files stay on disk.", Control: start("Remove…", forgetVault)}}})
		}
		device := []ui.SettingsItem{{ID: "appearance", Title: "Appearance", Description: "The window’s light or dark look.", Control: workspaceui.AppearanceChoice{Value: w.appearance.Get, OnChange: w.saveAppearance}}}
		if hasBackground {
			device = append(device, ui.SettingsItem{ID: "background", Title: "Keep syncing when no window is open", Control: ui.Switch{Label: "Keep syncing when no window is open", Checked: keep.Get, OnChange: func(on bool) {
				b, err := system.SetBackground(os.Getenv, on)
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				keep.Set(b.KeepRunning)
			}}})
		}
		groups = append(groups, ui.SettingsGroup{ID: "device", Title: "This device", Items: device})
		return ui.Settings{TestID: "settings", Title: settingsLabel, Category: category, Narrow: narrow, HideSidebar: true, HideSearch: true, HideHeading: true,
			Categories: []ui.SettingsCategory{{ID: "settings", Title: settingsLabel, Groups: groups}}}
	})
}

func (w *workspace) connectionLabel() string {
	conn := w.connection.Get()
	if s := w.syncInfo.Get(); conn == "Connected" && s != nil {
		return fmt.Sprintf("%s · %s · %d pending", s.HubVault, s.State, s.Pending)
	}
	return conn
}
func (w *workspace) loadSync() {
	id, c := w.vault.GetUntracked(), w.session.Client()
	if id == 0 || c == nil {
		return
	}
	w.syncRequest++
	req := w.syncRequest
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		states, err := c.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &id})
		w.cx.Post(func() {
			if err != nil || req != w.syncRequest || id != w.vault.GetUntracked() || c != w.session.Client() {
				return
			}
			if len(states) > 0 {
				w.syncInfo.Set(&states[0])
			} else {
				w.syncInfo.Set(nil)
			}
		})
	}()
}

func (w *workspace) vaultDialog(cx *core.Scope, action string) core.Node {
	if !isVaultAction(action) {
		return nil
	}
	if action == forgetVault {
		return w.forgetVaultDialog(cx)
	}
	return w.vaultForm(cx, action)
}

func (w *workspace) vaultForm(cx *core.Scope, action string) core.Node {
	name, location := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	addr, device := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	status, problem := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	busy := core.NewSignal(cx, false)
	ctx, cancel := context.WithCancel(context.Background())
	cx.Cleanup(cancel)
	id := w.vault.GetUntracked()
	pairing := action == joinVault || action == linkVault || action == pairAgain
	folder := action == openVault || action == createVault || action == joinVault
	if action == pairAgain {
		if s := w.syncInfo.GetUntracked(); s != nil {
			addr.Set(s.URL)
		}
	}
	var fields []core.Node
	add := func(label string, s *core.Signal[string]) {
		fields = append(fields, field(label, label, s, busy.Get, nil))
	}
	if pairing {
		add("Hub URL", addr)
		add("Device name (optional)", device)
	}
	if action != openVault && action != pairAgain {
		label := "Vault name"
		if pairing {
			label = "Hub vault name"
		}
		add(label, name)
	}
	if folder {
		label := "Parent folder"
		if action == openVault {
			label = "Vault folder"
		}
		// The folder's field and the button that picks it share a row.
		path := field(label, label, location, busy.Get, nil)
		path.Style = core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}
		fields = append(fields, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{path,
			ui.Button{Text: "Browse…", Variant: ui.ButtonSecondary, Disabled: busy.Get, OnPress: func() {
				cx.PickDirectories(core.PickDirectoriesOptions{Title: label}, func(d []core.Directory, err error) {
					if err != nil {
						problem.Set(err.Error())
					} else if len(d) > 0 {
						location.Set(d[0].Path)
					}
				})
			}}}})
	}
	submit := func() {
		c := w.session.Client()
		if c == nil {
			problem.Set("Reconnect to notesd and try again.")
			return
		}
		if busy.GetUntracked() {
			return
		}
		n, p := strings.TrimSpace(name.GetUntracked()), strings.TrimSpace(location.GetUntracked())
		if folder && !filepath.IsAbs(p) {
			problem.Set("Choose an absolute folder path.")
			return
		}
		if action == createVault || action == joinVault {
			if msg := vaults.NameError(n); msg != "" {
				problem.Set(msg)
				return
			}
			p = vaults.Folder(p, n)
		}
		url := vaults.HubURL(addr.GetUntracked())
		if pairing && (url == "" || (action != pairAgain && n == "")) {
			problem.Set("Enter the hub URL and vault details.")
			return
		}
		problem.Set("")
		busy.Set(true)
		status.Set("Connecting…")
		run := vaults.Join{Client: c, Context: ctx, URL: url, Name: n, Device: strings.TrimSpace(device.GetUntracked()), VaultID: id, PairOnly: action == pairAgain,
			OnCode:    func(code string) { cx.Post(func() { status.Set("On the hub, run notesctl hub approve " + code) }) },
			OnJoining: func() { cx.Post(func() { status.Set("Joining the vault…") }) }}
		if action == joinVault {
			run.Folder = p
		}
		go func() {
			var v *api.Vault
			var err error
			if pairing {
				v, err = run.Run()
			} else {
				op, done := context.WithTimeout(context.Background(), 20*time.Second)
				defer done()
				create := action == createVault
				v, err = c.Vault().Add(op, &api.VaultAddParams{Path: p, Create: &create})
			}
			w.cx.Post(func() {
				if ctx.Err() != nil {
					var left *vaults.VaultLeft
					if errors.As(err, &left) {
						w.problem.Set(err.Error())
					}
					return
				}
				if w.session.Client() != c {
					w.action.Set("")
					w.loadVaults()
					return
				}
				if err != nil {
					busy.Set(false)
					status.Set("")
					problem.Set(err.Error())
					return
				}
				w.action.Set("")
				if v != nil {
					// The new vault is what the window shows next, not Settings.
					w.settings.Set(false)
					w.leave(func() { w.selectVault(v.ID); w.loadVaults() })
				} else {
					w.loadSync()
				}
			})
		}()
	}
	// What the pairing waits for, such as the code to approve on the hub.
	fields = append(fields, core.Dynamic(func(*core.Scope) core.Node {
		if text := status.Get(); text != "" {
			return workspaceui.Banner{Tone: workspaceui.Accent, Icon: lucide.Info, Text: text}
		}
		return nil
	}))
	// A pairing in progress can be cancelled, so busy does not hold the
	// dialog open.
	return w.form(cx, form{title: action, fields: fields, problem: problem.Get, ready: func() bool { return !busy.Get() }, confirm: "Continue", submit: submit})
}

func (w *workspace) forgetVaultDialog(cx *core.Scope) core.Node {
	id := w.vault.GetUntracked()
	name := ""
	for _, v := range w.vaults.GetUntracked() {
		if v.ID == id {
			name = v.Name
			break
		}
	}
	remove := func() {
		c := w.session.Client()
		if c == nil || id == 0 {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err := c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: id})
			w.cx.Post(func() {
				if err != nil {
					w.problem.Set(err.Error())
					return
				}
				w.loadVaults()
			})
		}()
	}
	return w.destroy(cx, fmt.Sprintf("Remove %q from this device?", name), "The vault leaves this device’s list. Its folder and files stay on disk.", "Remove vault", func() { w.leave(remove) })
}

func desktopLifetime() func() {
	finish, err := system.WindowLifetime(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FrostNotes window lock:", err)
		return func() {}
	}
	return finish
}
