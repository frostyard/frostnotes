// Package window is the desktop app's window (plan 0009): it joins the
// session, the workspace, the panes and the notes' views under frostui's
// chrome. It is the only desktop package that knows all of them.
package window

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/commands"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/panes"
	"github.com/frostyard/frostnotes/internal/desktop/prefs"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostnotes/internal/desktop/system"
	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// RefreshDelay is how long after a change in the vault the panes read
// again.
const RefreshDelay = 300 * time.Millisecond

// Config is what the window needs of its surroundings.
type Config struct {
	Env      notes.Env
	Dial     session.Dialer
	Getenv   system.Getenv
	Blobs    string // notesd's blob store
	Version  string
	Seq      int64
	ConfigAt string // the preferences' directory
	StateAt  string // the state's directory
	// SetTitle sets the window's title; Close closes it; Quit ends the app.
	SetTitle func(string)
	Close    func()
	Quit     func()
	// PickFolder asks for a folder, "" when cancelled; OpenURL and OpenFile
	// open a web link and a vault file outside the app; Copy puts text on
	// the clipboard.
	PickFolder func(title string) string
	OpenURL    func(url string) error
	OpenFile   func(root, path string) error
	Copy       func(text string)
	Clipboard  editor.Clipboard
	PickFiles  func() []string
}

// App is the window's state. UI thread only.
type App struct {
	cfg     Config
	Session *session.Session
	Prefs   prefs.Prefs
	State   prefs.State
	Store   *workspace.Store

	vaults   []api.Vault
	listed   bool
	vault    *api.Vault
	Catalog  *catalog.Catalog
	settings *api.VaultSettings
	syncInfo *api.SyncStatus
	conflict []api.SyncConflict

	env        *panes.Env
	files      *panes.Files
	search     *panes.Search
	backlinks  *panes.Backlinks
	outgoing   *panes.Outgoing
	tagsPane   *panes.Tags
	properties *panes.Properties
	outline    *panes.Outline
	tasks      *panes.Tasks
	refresh    func()
	lastActive string

	tabs     map[string]*tabContent
	bindings map[*notes.Document]*binding
	split    uikit.SplitState
	dragging bool
	wasDrag  bool

	Shell        frostui.PaneShell
	Dialogs      frostui.Dialogs
	panes        frostui.Panes
	cmds         *frostui.CommandTable
	moving       *moveTarget
	pending      *pendingLine
	held         map[string]heldMerge
	previews     map[string]*preview
	lastMod      ui.Modifiers
	table        []commands.Spec
	actions      map[string]func()
	toasts       []ui.Toast
	actionToasts []frostui.ActionToast
	counts       textCounts
	vm           vaultManager
	sets         settingsDialog
}

type binding struct {
	b    *editor.Binding
	refs int
}

// New makes the window's state; Start connects it.
func New(cfg Config) *App {
	a := &App{cfg: cfg, tabs: map[string]*tabContent{}, bindings: map[*notes.Document]*binding{}, held: map[string]heldMerge{}, previews: map[string]*preview{}, table: commands.Table()}
	a.Prefs = prefs.Load(filepath.Join(cfg.ConfigAt, "prefs.json"))
	a.State = prefs.LoadState(filepath.Join(cfg.StateAt, "state.json"))
	a.Store = workspace.NewStore(cfg.Env, filepath.Join(cfg.StateAt, "workspace"))
	a.Session = session.New(cfg.Env, cfg.Dial, a.State.Seq)
	a.Session.OnSeq = func(seq int64) { a.State.Seq = seq }
	a.Session.OnEvent = a.event
	a.Session.OnResync = a.reloadAll
	a.Session.OnChange(a.connectionChanged)
	a.actions = a.commandActions()
	a.Shell.Dialogs = &a.Dialogs
	a.cmds = a.commandTable()
	a.Shell.Palette.Sources = a.paletteSources()
	return a
}

// Start connects to notesd.
func (a *App) Start() { a.Session.Start() }

// RequestClose saves what is unsaved, and calls done once the window may
// close: at once when every note reached notesd, else when the user says
// so (docs/design/app.md#operational-notes).
func (a *App) RequestClose(done func()) {
	a.Store.Flush()
	a.saveState()
	a.Session.Flush(func() {
		if !a.Session.Unsaved() {
			a.Session.Close()
			done()
			return
		}
		a.Dialogs.Ask(frostui.Confirm{Title: "Close anyway?", Body: "Some notes are not saved yet; what was typed in them is lost.",
			Action: "Close", Destructive: true, OnConfirm: func() {
				a.Session.Close()
				done()
			}})
	})
}

func (a *App) savePrefs() { a.report(prefs.Save(filepath.Join(a.cfg.ConfigAt, "prefs.json"), a.Prefs)) }
func (a *App) saveState() {
	a.report(prefs.SaveState(filepath.Join(a.cfg.StateAt, "state.json"), a.State))
}

func (a *App) report(err error) {
	if err != nil {
		a.notify(err.Error())
	}
}

func (a *App) notify(text string) {
	a.toasts = append(a.toasts, ui.Toast{Title: text})
}

func (a *App) call(f func(ctx context.Context, c *api.Client) error, done func(error)) {
	c := a.Session.Client()
	if c == nil {
		if done != nil {
			done(errOffline)
		}
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := f(ctx, c)
		a.cfg.Env.Post(func() {
			if done != nil {
				done(err)
			}
		})
	}()
}

func (a *App) connectionChanged() {
	if a.Session.State() == session.Connected {
		a.reloadAll()
	}
}

// reloadAll reads the vault list and everything of the window's vault.
func (a *App) reloadAll() {
	var list []api.Vault
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		list, err = c.Vault().List(ctx, &api.VaultListParams{})
		return err
	}, func(err error) {
		if err != nil {
			a.report(err)
			return
		}
		a.vaults, a.listed = list, true
		switch {
		case a.vault != nil && slices.ContainsFunc(list, func(v api.Vault) bool { return v.ID == a.vault.ID }):
			a.loadVaultData()
		case a.vault != nil:
			a.showVault(nil)
		case a.Prefs.LastVault != "":
			for i := range list {
				if list[i].Path == a.Prefs.LastVault {
					a.showVault(&list[i])
				}
			}
		}
	})
}

// showVault makes v the window's vault, nil for the vault manager.
func (a *App) showVault(v *api.Vault) {
	a.releaseAllTabs()
	clear(a.previews)
	a.vault = v
	if v == nil {
		a.Store.Load("")
		a.Catalog, a.env = nil, nil
		a.setTitle()
		return
	}
	a.Prefs.LastVault = v.Path
	a.savePrefs()
	a.Store.Load(v.Path)
	a.Catalog = catalog.New(a.cfg.Env, a.catalogAPI, v.ID)
	a.Catalog.OnChange(a.scheduleRefresh)
	a.Catalog.OnChange(a.catalogChanged)
	a.makePanes(v)
	a.loadVaultData()
	a.setTitle()
}

func (a *App) catalogAPI() catalog.API {
	if c := a.Session.Client(); c != nil {
		n := c.Note()
		return &n
	}
	return nil
}

func (a *App) panesAPI() panes.API {
	if c := a.Session.Client(); c != nil {
		n := c.Note()
		return &n
	}
	return nil
}

func (a *App) makePanes(v *api.Vault) {
	a.env = &panes.Env{Loop: a.cfg.Env, API: a.panesAPI, Vault: v.ID, Catalog: a.Catalog,
		Active: a.activeFile, ActiveText: a.activeText,
		Where: func(m ui.Modifiers) string { return workspace.WhereFor(m&ui.Cmd != 0, m&ui.Alt != 0, 0) },
		Actions: panes.Actions{Open: a.open, OpenAtLine: a.openAtLine, OpenOrCreate: a.openOrCreate,
			SearchFor: a.searchFor, JumpToLine: a.jumpToLine, Report: a.report}}
	a.files = &panes.Files{Env: a.env, Store: a.Store, ShowUnsupported: a.Prefs.ShowUnsupported, Actions: a.fileActions()}
	a.search = &panes.Search{Env: a.env}
	a.backlinks = &panes.Backlinks{Env: a.env}
	a.outgoing = &panes.Outgoing{Env: a.env}
	a.tagsPane = &panes.Tags{Env: a.env}
	a.properties = &panes.Properties{Env: a.env}
	a.outline = &panes.Outline{Env: a.env, CurrentLine: a.caretLine}
	a.tasks = &panes.Tasks{Env: a.env}
}

func (a *App) loadVaultData() {
	if a.vault == nil {
		return
	}
	id := a.vault.ID
	a.Catalog.Reload()
	a.refreshPanes()
	var settings *api.VaultSettings
	var statuses []api.SyncStatus
	var conflicts []api.SyncConflict
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		settings, err = c.Vault().Settings(ctx, &api.VaultSettingsParams{ID: id})
		if err != nil {
			return err
		}
		statuses, _ = c.Sync().Status(ctx, &api.SyncStatusParams{})
		conflicts, _ = c.Sync().Conflicts(ctx, &api.SyncConflictsParams{VaultID: id})
		return nil
	}, func(err error) {
		if err != nil || a.vault == nil || a.vault.ID != id {
			return
		}
		a.settings = settings
		a.Catalog.LinkFormat = string(settings.LinkFormat)
		a.Catalog.AttachmentFolder = settings.AttachmentFolder
		a.Catalog.Wikilinks = settings.Wikilinks
		a.syncInfo = nil
		for i := range statuses {
			if statuses[i].VaultID == id {
				a.syncInfo = &statuses[i]
			}
		}
		a.conflict = conflicts
	})
}

func (a *App) scheduleRefresh() {
	if a.refresh != nil {
		a.refresh()
	}
	a.refresh = a.cfg.Env.After(RefreshDelay, func() {
		a.refresh = nil
		a.refreshPanes()
	})
}

func (a *App) refreshPanes() {
	if a.env == nil {
		return
	}
	a.backlinks.Refresh()
	a.outgoing.Refresh()
	a.tagsPane.Refresh()
	a.properties.Refresh()
	a.tasks.Refresh()
}

func (a *App) vaultPath(id int64) string {
	for _, v := range a.vaults {
		if v.ID == id {
			return v.Path
		}
	}
	return ""
}

// event follows notesd: the catalog, the workspaces and the vault list.
func (a *App) event(_ int64, ev api.Event) {
	if a.Catalog != nil {
		a.Catalog.Event(ev)
	}
	switch e := ev.(type) {
	case api.NoteChanged:
		clear(a.previews)
		a.blocksChanged()
	case api.NoteMoved:
		a.Store.Follow(a.vaultPath(e.VaultID), func(w *workspace.Workspace) { w.ApplyMove(e.From, e.To) })
		a.blocksChanged()
	case api.NoteRemoved:
		a.Store.Follow(a.vaultPath(e.VaultID), func(w *workspace.Workspace) { w.ApplyDelete(e.Path) })
		a.blocksChanged()
	case api.VaultAdded, api.VaultRemoved:
		a.reloadAll()
	case api.VaultConfigured:
		a.loadVaultData()
	case api.SyncChanged:
		a.syncChanged(e.Status)
	}
}

// blocksChanged has live preview's rendered blocks read the vault again.
func (a *App) blocksChanged() {
	for _, tc := range a.tabs {
		if tc.ed != nil {
			tc.ed.Editor.Rendered.Changed()
		}
	}
}

// catalogChanged has live preview look again at what its embeds name.
func (a *App) catalogChanged() {
	for _, tc := range a.tabs {
		if tc.ed != nil {
			tc.ed.Editor.RefreshLive()
		}
	}
}

// syncChanged follows the window's vault's sync state, and reads the
// conflicts again when their number changes.
func (a *App) syncChanged(s api.SyncStatus) {
	if a.vault == nil || s.VaultID != a.vault.ID {
		return
	}
	stale := a.syncInfo == nil || a.syncInfo.Conflicts != s.Conflicts
	a.syncInfo = &s
	if !stale {
		return
	}
	id := s.VaultID
	var conflicts []api.SyncConflict
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		conflicts, err = c.Sync().Conflicts(ctx, &api.SyncConflictsParams{VaultID: id})
		return err
	}, func(err error) {
		if err == nil && a.vault != nil && a.vault.ID == id {
			a.conflict = conflicts
		}
	})
}

func (a *App) setTitle() {
	if a.cfg.SetTitle == nil {
		return
	}
	parts := []string{"Frostnotes"}
	if a.vault != nil {
		parts = append([]string{a.vault.Name}, parts...)
		if t := a.Store.Workspace().ActiveTab().View; t.Type != workspace.ViewEmpty {
			parts = append([]string{t.Title()}, parts...)
		}
	}
	a.cfg.SetTitle(strings.Join(parts, " - "))
}

// View draws the window.
func (a *App) View(c *ui.Context) {
	// Before anything below reads the state a palette command changes, or
	// what it does shows a frame late.
	a.Shell.Palette.RunPending()
	a.lastMod = c.Modifiers()
	a.applyAppearance(c)
	for _, t := range a.toasts {
		c.AddToast(t)
	}
	a.toasts = nil
	for _, t := range a.actionToasts {
		frostui.ShowActionToast(c, t)
	}
	a.actionToasts = nil
	a.watchActive()
	a.frame(c)
}

// watchActive reads the panes again when the active file changes.
func (a *App) watchActive() {
	if a.env == nil {
		return
	}
	if p := a.activeFile(); p != a.lastActive {
		a.lastActive = p
		a.backlinks.Refresh()
		a.outgoing.Refresh()
		a.setTitle()
	}
}
