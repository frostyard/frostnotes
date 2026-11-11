package window

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
)

// The join flow's phases.
const (
	joinForm    = "form"
	joinApprove = "approve"
	joinJoining = "joining"
)

// The join flow's modes: join a hub, or pair again where it refused the
// device's token.
const (
	modeJoin = "join"
	modePair = "pair"
)

var errCancelled = errors.New("the join was cancelled")

func forbiddenName() *regexp.Regexp { return regexp.MustCompile(`[\\/:*?"<>|#^\[\]]`) }

// vaultNameError says why name cannot name a new vault, "" when it can.
func vaultNameError(name string) string {
	t := strings.TrimSpace(name)
	switch {
	case t == "":
		return "A vault needs a name."
	case strings.HasPrefix(t, "."):
		return "A vault's name cannot start with a dot."
	case forbiddenName().MatchString(t):
		return `A vault's name cannot hold any of these: \ / : * ? " < > | # ^ [ ]`
	}
	return ""
}

func vaultFolder(location, name string) string {
	return strings.TrimRight(location, "/") + "/" + strings.TrimSpace(name)
}

func hubURL(url string) string { return strings.TrimRight(strings.TrimSpace(url), "/") }

// vaultManager is the vault manager (docs/specs/ui.md#vault-manager): a
// page while the window has no vault, a dialog over it otherwise.
type vaultManager struct {
	open     bool
	panel    string // "", create or join
	name     string
	location string
	openPath string
	err      string
	join     joinFlow
	canSync  *bool
	checked  bool
}

func (m *vaultManager) reset() {
	*m = vaultManager{open: m.open, canSync: m.canSync, checked: m.checked}
}

func (m *vaultManager) view(c *ui.Context, a *App, dialog bool) {
	if !m.checked {
		m.checked = true
		m.checkSync(a)
	}
	if !dialog {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Role(ui.RoleDialog).Label("Manage vaults").Children(func() { m.parts(c, a) })
		return
	}
	ui.Modal(c, &m.open, func() {
		ui.Row(c).Width(720).Height(480).AlignItems(ui.Stretch).Label("Manage vaults").Children(func() { m.parts(c, a) })
	})
	if !m.open {
		m.cancelJoin()
		m.reset()
	}
}

func (m *vaultManager) checkSync(a *App) {
	a.call(func(ctx context.Context, c *api.Client) error {
		_, err := c.Sync().Hubs(ctx, &api.SyncHubsParams{})
		return err
	}, func(err error) {
		can := !syncMissing(err)
		m.canSync = &can
	})
}

// syncMissing reports whether an answer says this notesd has no sync. Any
// other failure, such as a lost connection, says nothing about it.
func syncMissing(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && (e.Code == api.CodeUnavailable || e.Code == api.CodeMethodNotFound)
}

func (m *vaultManager) parts(c *ui.Context, a *App) {
	m.list(c, a)
	ui.Scroll(c).Grow(1).Padding(frostui.SpaceXL).Children(func() {
		ui.Column(c).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Children(func() {
			switch m.panel {
			case "create":
				m.createForm(c, a)
			case "join":
				m.join.view(c, a, func() { m.panel = "" })
			default:
				m.actions(c, a)
			}
		})
	})
}

func (m *vaultManager) list(c *ui.Context, a *App) {
	t := c.Theme()
	ui.Scroll(c).Width(280).Shrink(0).Padding(frostui.SpaceS).Background(frostui.Chrome(t)).Children(func() {
		if len(a.vaults) == 0 {
			frostui.Muted(c, "No vaults yet")
			return
		}
		ui.Column(c).Gap(2).AlignItems(ui.Stretch).Role(ui.RoleList).Label("Vaults").Children(func() {
			for i := range a.vaults {
				v := &a.vaults[i]
				row := ui.ButtonBase(c.Key(v.ID)).Column().Padding(8, 12).Radius(frostui.RadiusChip).AlignItems(ui.Stretch).Cursor(ui.CursorPointer).Label(v.Name)
				if a.vault != nil && a.vault.ID == v.ID {
					row.Background(t.Accent.Alpha(0.16))
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.ContextMenu(func(menu *ui.Menu) {
					if menu.Item("Remove from list").Chosen() {
						m.remove(a, v)
					}
				})
				if row.Clicked() {
					m.open = false
					m.cancelJoin()
					m.reset()
					a.showVault(v)
				}
				row.Children(func() {
					frostui.Strong(c, v.Name).SingleLine()
					frostui.Caption(c, v.Path).SingleLine()
				})
			}
		})
	})
}

func (m *vaultManager) remove(a *App, v *api.Vault) {
	id, name := v.ID, v.Name
	synced := false
	a.call(func(ctx context.Context, c *api.Client) error {
		// Not synced, or sync unavailable: nothing more to say.
		if statuses, err := c.Sync().Status(ctx, &api.SyncStatusParams{}); err == nil {
			synced = slices.ContainsFunc(statuses, func(s api.SyncStatus) bool { return s.VaultID == id })
		}
		return nil
	}, func(error) {
		forgets := "Frostnotes forgets its index and version history on this device."
		if synced {
			forgets = "Frostnotes forgets its index and version history on this device, and its place on the hub; open it again to sync it, after joining."
		}
		a.Dialogs.Ask(frostui.Confirm{Title: `Remove "` + name + `" from the list?`, Body: "Its folder and files stay where they are. " + forgets, Action: "Remove", Destructive: true,
			OnConfirm: func() {
				a.call(func(ctx context.Context, c *api.Client) error {
					return c.Vault().Remove(ctx, &api.VaultRemoveParams{ID: id})
				}, func(err error) {
					a.report(err)
					a.reloadAll()
				})
			}})
	})
}

func action(c *ui.Context, title, body string, fn func()) {
	ui.Row(c).Gap(frostui.SpaceM).Padding(frostui.SpaceM, 0).AlignItems(ui.Center).Label(title).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).AlignItems(ui.Stretch).Children(func() {
			frostui.Strong(c, title)
			frostui.Caption(c, body).Wrap()
		})
		fn()
	})
	frostui.RowDivider(c)
}

func (m *vaultManager) actions(c *ui.Context, a *App) {
	ui.Text(c, "Frostnotes").FontSize(20).Bold()
	if m.err != "" {
		frostui.Problem(c, m.err)
	}
	action(c.Key("create"), "Create new vault", "Make a new vault in a folder you choose.", func() {
		if ui.PrimaryButton(c, "Create").Clicked() {
			m.panel, m.err = "create", ""
		}
	})
	action(c.Key("open"), "Open folder as vault", "Choose a folder of Markdown files.", func() {
		if a.cfg.PickFolder == nil {
			ui.TextInput(c, &m.openPath).Width(240).Label("Folder path")
		}
		if ui.Button(c, "Open").Clicked() {
			m.openFolder(a)
		}
	})
	words := "Checking whether this notesd syncs…"
	if m.canSync != nil {
		words = "This notesd does not sync."
		if *m.canSync {
			words = "Sync a new vault with a vault on a hub. To sync a vault you have, open it, then Settings, Sync."
		}
	}
	action(c.Key("join"), "Join a vault on a hub", words, func() {
		if m.canSync != nil && *m.canSync && ui.Button(c, "Join").Clicked() {
			m.panel, m.err = "join", ""
			m.join = joinFlow{mode: modeJoin, phase: joinForm, makesVault: true}
		}
	})
}

func (m *vaultManager) openFolder(a *App) {
	p := strings.TrimSpace(m.openPath)
	if a.cfg.PickFolder != nil {
		p = a.cfg.PickFolder("Choose a folder to open as a vault")
	}
	if p != "" {
		m.add(a, p, false)
	}
}

// add registers a folder, then shows it.
func (m *vaultManager) add(a *App, p string, create bool) {
	var v *api.Vault
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		params := &api.VaultAddParams{Path: p}
		if create {
			params.Create = &create
		}
		v, err = c.Vault().Add(ctx, params)
		return err
	}, func(err error) {
		if err != nil {
			m.err = err.Error()
			return
		}
		a.listVault(*v)
		m.open = false
		m.cancelJoin()
		m.reset()
		a.showVault(v)
	})
}

func (m *vaultManager) createForm(c *ui.Context, a *App) {
	if ui.Button(c, "Back").Clicked() {
		m.panel, m.err = "", ""
	}
	ui.Text(c, "Create new vault").FontSize(18).Bold()
	frostui.Body(c, "Vault name")
	ui.TextInput(c, &m.name).Label("Vault name").AutoFocus()
	problem := ""
	if m.name != "" {
		problem = vaultNameError(m.name)
	}
	if problem != "" {
		frostui.Toned(c, problem, frostui.Danger).Wrap()
	}
	location(c, a, &m.location, "Choose where the new vault goes")
	if m.err != "" {
		frostui.Problem(c, m.err)
	}
	can := vaultNameError(m.name) == "" && strings.TrimSpace(m.location) != ""
	if ui.PrimaryButton(c, "Create").Disabled(!can).Clicked() && can {
		m.add(a, vaultFolder(m.location, m.name), true)
	}
}

func location(c *ui.Context, a *App, at *string, title string) {
	frostui.Body(c, "Location")
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c, at).Grow(1).Label("Location")
		if a.cfg.PickFolder != nil && ui.Button(c, "Browse").Clicked() {
			if p := a.cfg.PickFolder(title); p != "" {
				*at = p
			}
		}
	})
}

func (m *vaultManager) cancelJoin() { m.join.stop() }

// joinFlow is joining a hub or pairing again without a terminal
// (docs/design/sync.md#pairing-and-sessions): `notesctl hub join`'s steps.
type joinFlow struct {
	mode       string
	phase      string
	makesVault bool
	vaultID    int64
	url        string
	name       string
	device     string
	location   string
	edited     bool
	code       string
	err        string
	cancel     context.CancelFunc
	onDone     func()
}

func (j *joinFlow) stop() {
	if j.cancel != nil {
		j.cancel()
		j.cancel = nil
	}
}

func (j *joinFlow) nameProblem() string {
	if j.mode == modePair {
		return ""
	}
	if j.makesVault {
		return vaultNameError(j.name)
	}
	if strings.TrimSpace(j.name) == "" {
		return "A vault needs a name."
	}
	return ""
}

func (j *joinFlow) view(c *ui.Context, a *App, back func()) {
	title := "Join a vault on a hub"
	if j.mode == modePair {
		title = "Pair again"
	}
	if j.phase == joinForm && ui.Button(c, "Back").Clicked() {
		j.stop()
		back()
		return
	}
	ui.Text(c, title).FontSize(18).Bold()
	switch j.phase {
	case joinForm:
		j.form(c, a)
	case joinApprove:
		frostui.Body(c, "Approve this device on the hub's machine:").Wrap()
		ui.Text(c, "notesctl hub approve "+j.code).Font("monospace").Selectable()
		frostui.Muted(c, "Waiting for the approval…")
		if ui.Button(c, "Cancel").Clicked() {
			j.stop()
			back()
		}
	case joinJoining:
		frostui.Loading(c, "Joining…")
	}
}

func (j *joinFlow) form(c *ui.Context, a *App) {
	frostui.Body(c, "Hub URL")
	ui.TextInput(c, &j.url).Label("Hub URL").Placeholder("https://lodge.example.ts.net/device/notes").AutoFocus()
	if j.mode != modePair {
		frostui.Body(c, "Hub vault name")
		if ui.TextInput(c, &j.name).Label("Hub vault name").Changed() {
			j.edited = true
		}
		if p := j.nameProblem(); j.edited && p != "" {
			frostui.Toned(c, p, frostui.Danger).Wrap()
		}
	}
	frostui.Body(c, "Device name")
	ui.TextInput(c, &j.device).Label("Device name").Placeholder("The host name")
	if j.makesVault {
		location(c, a, &j.location, "Choose where the new vault goes")
	}
	if j.err != "" {
		frostui.Problem(c, j.err)
	}
	can := strings.TrimSpace(j.url) != "" && j.nameProblem() == "" && (!j.makesVault || strings.TrimSpace(j.location) != "")
	label := "Join"
	if j.mode == modePair {
		label = "Pair"
	}
	if ui.PrimaryButton(c, label).Disabled(!can).Clicked() && can {
		j.start(a)
	}
}

func (j *joinFlow) start(a *App) {
	client := a.Session.Client()
	if client == nil {
		j.err = errOffline.Error()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.stop()
	j.cancel, j.err, j.code, j.phase = cancel, "", "", joinJoining
	post := a.cfg.Env.Post
	flow := joinRun{c: client, ctx: ctx, url: hubURL(j.url), name: strings.TrimSpace(j.name), device: strings.TrimSpace(j.device),
		vaultID: j.vaultID, mode: j.mode, folder: "", onCode: func(code string) {
			post(func() {
				if ctx.Err() == nil {
					j.code, j.phase = code, joinApprove
				}
			})
		}, onJoining: func() {
			post(func() {
				if ctx.Err() == nil {
					j.phase = joinJoining
				}
			})
		}}
	if j.makesVault {
		flow.folder = vaultFolder(j.location, j.name)
	}
	go func() {
		defer cancel()
		v, err := flow.run()
		post(func() {
			if errors.Is(err, errCancelled) || errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				j.phase, j.err = joinForm, err.Error()
				return
			}
			a.joined(j, v)
		})
	}()
}

// joined ends a join that worked: a vault it made becomes the window's.
func (a *App) joined(j *joinFlow, v *api.Vault) {
	if v != nil {
		a.listVault(*v)
		a.vm.open = false
		a.vm.reset()
		a.showVault(v)
	} else {
		a.loadVaultData()
	}
	if j.onDone != nil {
		j.onDone()
	}
}

// joinRun is one run of the flow, off the UI thread.
type joinRun struct {
	c                 *api.Client
	ctx               context.Context
	url, name, device string
	vaultID           int64
	mode, folder      string
	onCode            func(string)
	onJoining         func()
}

func (r *joinRun) stopped() error {
	if r.ctx.Err() != nil {
		return errCancelled
	}
	return nil
}

// awaitApproval shows the pairing's code and waits: notesd long-polls up to
// 30 seconds for each sync.paired.
func (r *joinRun) awaitApproval(p *api.Pairing) error {
	if err := r.stopped(); err != nil {
		return err
	}
	r.onCode(p.Code)
	for {
		if err := r.stopped(); err != nil {
			return err
		}
		res, err := r.c.Sync().Paired(r.ctx, &api.SyncPairedParams{Pairing: p.Pairing})
		if err != nil {
			return err
		}
		switch res.State {
		case api.PairingStateApproved:
			return nil
		case api.PairingStateExpired:
			return errors.New("the pairing expired: the code was not approved within 10 minutes")
		}
	}
}

func (r *joinRun) pairNew() error {
	params := &api.SyncPairParams{URL: r.url}
	if r.device != "" {
		params.Name = &r.device
	}
	p, err := r.c.Sync().Pair(r.ctx, params)
	if err != nil {
		return err
	}
	return r.awaitApproval(p)
}

func (r *joinRun) paired() (bool, error) {
	hubs, err := r.c.Sync().Hubs(r.ctx, &api.SyncHubsParams{})
	if err != nil {
		return false, err
	}
	for _, h := range hubs {
		if h.URL == r.url {
			return true, nil
		}
	}
	return false, nil
}

func (r *joinRun) join(id int64, wasPaired bool) error {
	params := &api.SyncJoinParams{VaultID: id, URL: r.url, Name: r.name}
	_, err := r.c.Sync().Join(r.ctx, params)
	var e *api.Error
	if err == nil || !wasPaired || !errors.As(err, &e) || e.Code != api.CodeRevoked {
		return err
	}
	// The hub refuses a device it paired: pair again once, join again.
	if err := r.pairNew(); err != nil {
		return err
	}
	_, err = r.c.Sync().Join(r.ctx, params)
	return err
}

// registered reports whether notesd already lists the folder to be made.
func (r *joinRun) registered() bool {
	bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := r.c.Vault().List(bg, &api.VaultListParams{})
	return err != nil || slices.ContainsFunc(list, func(v api.Vault) bool { return v.Path == r.folder })
}

// forgetFolder removes the vault of the folder from the list, after a
// cancel that may have landed once notesd had added it.
func (r *joinRun) forgetFolder() {
	bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := r.c.Vault().List(bg, &api.VaultListParams{})
	if err != nil {
		return
	}
	for _, v := range list {
		if v.Path == r.folder {
			_ = r.c.Vault().Remove(bg, &api.VaultRemoveParams{ID: v.ID})
		}
	}
}

// cleanupFailed says a join failed and its vault stayed on the list. It
// does not wrap the join's error: a cancel would read as nothing to show.
func cleanupFailed(err error, name string, rerr error) error {
	return errors.New(err.Error() + `; could not remove the new vault "` + name + `" from the list: ` + rerr.Error() + ". Remove it in Manage vaults.")
}

// run returns the vault it made, nil when the vault existed.
func (r *joinRun) run() (*api.Vault, error) {
	if r.mode == modePair {
		return nil, r.pairNew()
	}
	wasPaired, err := r.paired()
	if err != nil {
		return nil, err
	}
	if !wasPaired {
		if err := r.pairNew(); err != nil {
			return nil, err
		}
	}
	r.onJoining()
	if r.folder == "" {
		return nil, r.join(r.vaultID, wasPaired)
	}
	create := true
	known := r.registered()
	v, err := r.c.Vault().Add(r.ctx, &api.VaultAddParams{Path: r.folder, Create: &create})
	if err != nil {
		// A cancel can land after notesd committed the vault.
		if errors.Is(err, context.Canceled) && !known {
			r.forgetFolder()
		}
		return nil, err
	}
	if err := r.join(v.ID, wasPaired); err != nil {
		// Forget the vault it made and did not join, even when cancelled.
		bg, cancel := context.WithTimeout(context.Background(), 10_000_000_000)
		defer cancel()
		if rerr := r.c.Vault().Remove(bg, &api.VaultRemoveParams{ID: v.ID}); rerr != nil {
			return nil, cleanupFailed(err, r.name, rerr)
		}
		return nil, err
	}
	return v, nil
}
