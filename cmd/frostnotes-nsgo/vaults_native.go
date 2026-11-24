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
	manageVaults = "Manage vaults"
	createVault  = "Create new vault"
	openVault    = "Open folder as vault"
	joinVault    = "Join a vault on a hub"
	linkVault    = "Sync this vault with a hub"
	pairAgain    = "Pair again"
	forgetVault  = "Remove vault from list"
)

func isVaultAction(a string) bool {
	switch a {
	case manageVaults, createVault, openVault, joinVault, linkVault, pairAgain, forgetVault:
		return true
	}
	return false
}

func (w *workspace) vaultActions() []core.Node {
	return []core.Node{ui.IconButton{Label: manageVaults, Icon: lucide.Library, OnPress: func() { w.action.Set(manageVaults) }}}
}
func (w *workspace) vaultButtons() []core.Node {
	var nodes []core.Node
	for _, action := range []string{createVault, openVault, joinVault} {
		nodes = append(nodes, ui.Button{Text: action, OnPress: func() { w.action.Set(action) }})
	}
	return nodes
}
func (w *workspace) vaultWelcome() core.Node {
	return workspaceui.Page(append([]core.Node{core.Text{Content: "Open a vault", Style: ui.TypeTitle}}, w.vaultButtons()...)...)
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
	if action != manageVaults {
		return w.vaultForm(cx, action)
	}
	nodes := w.vaultButtons()
	if w.vault.Get() != 0 {
		nodes = append(nodes, ui.Button{Text: forgetVault, OnPress: func() { w.action.Set(forgetVault) }})
		if s := w.syncInfo.Get(); s == nil {
			nodes = append(nodes, ui.Button{Text: linkVault, OnPress: func() { w.action.Set(linkVault) }})
		} else if s.State == api.SyncStateRevoked {
			nodes = append(nodes, ui.Button{Text: pairAgain, OnPress: func() { w.action.Set(pairAgain) }})
		}
	}
	if b, ok := system.GetBackground(os.Getenv); ok {
		keep := core.NewSignal(cx, b.KeepRunning)
		nodes = append(nodes, ui.Checkbox{Text: "Keep syncing when no window is open", Checked: keep.Get, OnChange: func(on bool) {
			b, err := system.SetBackground(os.Getenv, on)
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			keep.Set(b.KeepRunning)
		}})
	}
	return ui.Dialog{Title: manageVaults, Open: func() bool { return true }, OnOpenChange: w.closeDialog, Children: nodes}
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
	field := func(label string, s *core.Signal[string]) {
		fields = append(fields, ui.Input{Label: label, Placeholder: label, Value: s.Get, OnChange: s.Set, Disabled: busy.Get})
	}
	if pairing {
		field("Hub URL", addr)
		field("Device name (optional)", device)
	}
	if action != openVault && action != pairAgain {
		label := "Vault name"
		if pairing {
			label = "Hub vault name"
		}
		field(label, name)
	}
	if folder {
		label := "Parent folder"
		if action == openVault {
			label = "Vault folder"
		}
		field(label, location)
		fields = append(fields, ui.Button{Text: "Browse…", Disabled: busy.Get, OnPress: func() {
			cx.PickDirectories(core.PickDirectoriesOptions{Title: label}, func(d []core.Directory, err error) {
				if err != nil {
					problem.Set(err.Error())
				} else if len(d) > 0 {
					location.Set(d[0].Path)
				}
			})
		}})
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
					w.leave(func() { w.selectVault(v.ID); w.loadVaults() })
				} else {
					w.loadSync()
				}
			})
		}()
	}
	fields = append(fields, core.Text{ContentFunc: status.Get}, core.Text{ContentFunc: problem.Get})
	return ui.Dialog{Open: func() bool { return true }, Title: action, OnOpenChange: w.closeDialog, Children: fields, Footer: []core.Node{ui.Button{Text: "Cancel", OnPress: func() { w.action.Set("") }}, ui.Button{Text: "Continue", Disabled: busy.Get, OnPress: submit}}}
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
	busy := core.NewSignal(cx, false)
	problem := core.NewSignal(cx, "")
	remove := func() {
		c := w.session.Client()
		if c == nil || id == 0 || busy.GetUntracked() {
			return
		}
		busy.Set(true)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			err := c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: id})
			cx.Post(func() {
				if err != nil {
					busy.Set(false)
					problem.Set(err.Error())
					return
				}
				w.action.Set("")
				w.loadVaults()
			})
		}()
	}
	return ui.Dialog{Open: func() bool { return true }, Title: forgetVault, Description: fmt.Sprintf("Remove %q from this device’s vault list? Its folder and files stay on disk.", name), OnOpenChange: w.closeDialog, Children: []core.Node{core.Text{ContentFunc: problem.Get}}, Footer: []core.Node{ui.Button{Text: "Cancel", OnPress: func() { w.action.Set("") }}, ui.Button{Text: "Remove", Disabled: busy.Get, OnPress: func() { w.leave(remove) }}}}
}

func desktopLifetime() func() {
	finish, err := system.WindowLifetime(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FrostNotes window lock:", err)
		return func() {}
	}
	return finish
}
