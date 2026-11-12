package window

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/system"
)

// BackgroundPoll is how often the open dialog reads the desktop's answer
// again (ADR-0033).
const BackgroundPoll = 2 * time.Second

// A setting's control.
type controlKind int

const (
	toggleControl controlKind = iota
	selectControl
	textControl
	numberControl
	keysControl
	infoControl
	buttonControl
)

type option struct{ value, label string }

// settingRow is one setting: its id says where it goes: "pref:<key>",
// "vault:<field>", "sync:<action>", "hub:revoke:<device>" or "bg:<key>".
type settingRow struct {
	id, name, description string
	kind                  controlKind
	on                    bool
	value                 string
	options               []option
	placeholder           string
	number, min, max      int
	keys                  []string
	text                  string
	label                 string
	danger                bool
	disabled              bool
}

type settingsSection struct {
	id, name string
	rows     []settingRow
}

// hubView is this notesd's hub role, for the Hub section.
type hubView struct {
	status  *api.HubStatus
	devices []api.HubDevice
	pending []api.PendingPairing
}

// settingsDialog is the settings dialog (docs/specs/ui.md#settings-batch-2)
// on frostui's settings layout: the preferences, the vault's settings, the
// keyboard map, sync, the hub, and in the Flatpak the background setting.
type settingsDialog struct {
	layout     frostui.Settings
	wasOpen    bool
	wantShow   string
	drafts     map[string]string
	hub        *hubView
	background *system.Background
	awaited    *uint64
	poll       func()
	joining    *joinFlow
}

func (s *settingsDialog) show(section string) { s.wantShow = section }

func (s *settingsDialog) view(c *ui.Context, a *App) {
	if s.wantShow != "" {
		s.layout.Search = true
		s.layout.Show(s.wantShow)
		s.wantShow = ""
		s.drafts = map[string]string{}
		s.load(a)
	}
	if s.layout.Open {
		s.wasOpen = true
		s.layout.Dialog(c, "Settings", a.settingsPages(c))
		if s.joining != nil {
			s.joinDialog(c, a)
		}
	}
	if s.wasOpen && !s.layout.Open {
		s.wasOpen = false
		s.commit(a)
		s.closed()
	}
}

func (s *settingsDialog) closed() {
	s.awaited = nil
	if s.poll != nil {
		s.poll()
		s.poll = nil
	}
	if s.joining != nil {
		s.joining.stop()
		s.joining = nil
	}
}

func (s *settingsDialog) joinDialog(c *ui.Context, a *App) {
	open := true
	j := s.joining
	done := func() {
		j.stop()
		s.joining = nil
	}
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(480).Gap(frostui.SpaceM).AlignItems(ui.Stretch).Label("Join a hub").Children(func() { j.view(c, a, done) })
	})
	if !open {
		done()
	}
}

func (s *settingsDialog) load(a *App) {
	var hub *hubView
	a.call(func(ctx context.Context, c *api.Client) error {
		status, err := c.Hub().Status(ctx, &api.HubStatusParams{})
		if err != nil || status.Hub == "" {
			return nil
		}
		devices, err := c.Hub().Devices(ctx, &api.HubDevicesParams{})
		if err != nil {
			return nil
		}
		pending, err := c.Hub().Pending(ctx, &api.HubPendingParams{})
		if err != nil {
			return nil
		}
		hub = &hubView{status, devices, pending}
		return nil
	}, func(error) { s.hub = hub })
	if a.cfg.Getenv == nil {
		return
	}
	if b, ok := system.GetBackground(a.cfg.Getenv); ok {
		s.background = &b
		s.schedulePoll(a)
	}
}

func (s *settingsDialog) schedulePoll(a *App) {
	s.poll = a.cfg.Env.After(BackgroundPoll, func() {
		if !s.layout.Open {
			return
		}
		if b, ok := system.GetBackground(a.cfg.Getenv); ok {
			s.background = &b
			if s.awaited != nil && b.Portal != nil && b.Portal.Generation != nil && *b.Portal.Generation >= *s.awaited {
				s.awaited = nil
			}
		}
		s.schedulePoll(a)
	})
}

// settingOf draws a row with frostui's layout.
func (s *settingsDialog) settingOf(a *App, r settingRow) frostui.Setting {
	out := frostui.Setting{Title: r.name, Subtitle: r.description, Disabled: r.disabled}
	switch r.kind {
	case toggleControl:
		on := r.on
		out.Switch = &on
		out.OnChange = func() { a.settingChanged(r.id, on) }
	case selectControl:
		selected := r.value
		for _, o := range r.options {
			out.Options = append(out.Options, o.label)
			if o.value == r.value {
				selected = o.label
			}
		}
		out.Choice = &selected
		out.OnChange = func() { a.settingChanged(r.id, r.options[slices.Index(out.Options, selected)].value) }
	case textControl:
		out.Control = func(c *ui.Context) { s.textField(c, a, r) }
	case numberControl:
		v := float64(r.number)
		out.Number, out.Min, out.Max, out.Step = &v, float64(r.min), float64(r.max), 1
		out.OnChange = func() { a.settingChanged(r.id, int(v)) }
	case keysControl:
		out.Control = func(c *ui.Context) { frostui.Shortcut(c, r.keys...) }
	case infoControl:
		out.Value = r.text
	case buttonControl:
		out.Control = func(c *ui.Context) {
			b := ui.Button(c, r.label).Label(r.label + " " + r.name).Disabled(r.disabled)
			if r.danger {
				b.TextColor(c.Theme().Danger)
			}
			if b.Clicked() && !r.disabled {
				a.settingChanged(r.id, true)
			}
		}
	}
	return out
}

// settingsPages are the dialog's pages.
func (a *App) settingsPages(*ui.Context) []frostui.SettingsPage {
	var pages []frostui.SettingsPage
	for _, sec := range a.settingsSections() {
		g := frostui.SettingsGroup{}
		for _, r := range sec.rows {
			g.Settings = append(g.Settings, a.sets.settingOf(a, r))
		}
		pages = append(pages, frostui.SettingsPage{ID: sec.id, Title: sec.name, Icon: sectionIcon(sec.id), Groups: []frostui.SettingsGroup{g}})
	}
	return pages
}

func sectionIcon(id string) string {
	switch id {
	case "editor":
		return "document-edit"
	case "files":
		return "folder"
	case "appearance":
		return "view-grid"
	case "hotkeys":
		return "utilities-terminal"
	case "daily":
		return "x-office-calendar"
	case "templates":
		return "text-x-generic"
	case "sync":
		return "view-refresh"
	case "hub":
		return "network-server"
	}
	return "go-bottom"
}

// textField keeps a draft while it has the focus, and commits it on Enter
// or when it loses it.
func (s *settingsDialog) textField(c *ui.Context, a *App, r settingRow) {
	draft, editing := s.drafts[r.id]
	if !editing {
		draft = r.value
	}
	in := ui.TextInput(c, &draft).Width(240).Label(r.name).Placeholder(r.placeholder).Disabled(r.disabled)
	if in.Changed() {
		s.drafts[r.id] = draft
	}
	if (in.Submitted() || (editing && !in.Focused())) && !r.disabled {
		delete(s.drafts, r.id)
		if draft != r.value {
			a.settingChanged(r.id, draft)
		}
	}
}

// commit keeps unfinished text fields when Settings closes.
func (s *settingsDialog) commit(a *App) {
	for id, draft := range s.drafts {
		delete(s.drafts, id)
		a.settingChanged(id, draft)
	}
}

// settingChanged applies a change a row made.
func (a *App) settingChanged(id string, value any) {
	where, key, _ := strings.Cut(id, ":")
	switch where {
	case "pref":
		a.setPref(key, value)
	case "vault":
		a.configure(key, value)
	case "bg":
		a.setBackground(value.(bool))
	case "sync":
		a.syncAction(key)
	case "hub":
		if dev, ok := strings.CutPrefix(key, "revoke:"); ok {
			a.revokeDevice(dev)
		}
	}
}

// setPref sets a preference by its JSON name.
func (a *App) setPref(key string, value any) {
	v := reflect.ValueOf(&a.Prefs).Elem()
	t := v.Type()
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == key {
			v.Field(i).Set(reflect.ValueOf(value))
		}
	}
	a.savePrefs()
	a.prefsChanged()
}

// prefsChanged applies the preferences to what is open.
func (a *App) prefsChanged() {
	if a.files != nil {
		a.files.ShowUnsupported = a.Prefs.ShowUnsupported
	}
	for _, tc := range a.tabs {
		if tc.ed != nil {
			tc.ed.Editor.Prefs = a.editorPrefs()
			tc.ed.Editor.Rendered.SetRemoteImages(a.Prefs.LoadRemoteImages)
			tc.ed.Editor.Rendered.Changed()
			tc.ed.Editor.RefreshLive()
		}
		if tc.read != nil {
			tc.read.SetRemoteImages(a.Prefs.LoadRemoteImages)
		}
	}
}

// applyAppearance applies the theme and the zoom. The theme is frostui's
// Frostyard or Frostyard light (frostui ADR-0011): the one the Theme
// setting picks, or for System the one of the desktop's light or dark, with
// the desktop's high contrast and text size but never its accent. It is
// set before anything reads the theme, so frostui.Adapt keeps it.
func (a *App) applyAppearance(c *ui.Context) {
	c.SetTheme(appearance(c, a.Prefs.Theme))
	c.SetZoom(float32(a.Prefs.Zoom))
}

// appearance is the theme the Theme setting asks for: "light", "dark", or
// anything else for the desktop's.
func appearance(c *ui.Context, setting string) *ui.Theme {
	dark := c.Theme().Dark // the desktop's, as MyGo resets it each frame
	switch setting {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	p := c.Preferences()
	p.Accent = ui.Color{}
	return frostui.FrostyardTheme(dark).WithPreferences(p)
}

func (a *App) configure(field string, value any) {
	if a.vault == nil {
		return
	}
	p := &api.VaultConfigureParams{ID: a.vault.ID}
	str := func() *string { s, _ := value.(string); return &s }
	flag := func() *bool { b, _ := value.(bool); return &b }
	switch field {
	case "dailyFolder":
		p.DailyFolder = str()
	case "dailyFormat":
		p.DailyFormat = str()
	case "dailyTemplate":
		p.DailyTemplate = str()
	case "templateFolder":
		p.TemplateFolder = str()
	case "templateDateFormat":
		p.TemplateDateFormat = str()
	case "templateTimeFormat":
		p.TemplateTimeFormat = str()
	case "newNoteLocation":
		l := api.NewNoteLocation(*str())
		p.NewNoteLocation = &l
	case "newNoteFolder":
		p.NewNoteFolder = str()
	case "attachmentFolder":
		p.AttachmentFolder = str()
	case "linkFormat":
		l := api.LinkFormat(*str())
		p.LinkFormat = &l
	case "wikilinks":
		p.Wikilinks = flag()
	case "updateLinks":
		p.UpdateLinks = flag()
	case "ignore":
		p.Ignore = []string{}
		for part := range strings.SplitSeq(*str(), ",") {
			if t := strings.TrimSpace(part); t != "" {
				p.Ignore = append(p.Ignore, t)
			}
		}
	}
	a.call(func(ctx context.Context, c *api.Client) error {
		_, err := c.Vault().Configure(ctx, p)
		return err
	}, func(err error) {
		// A change notesd made comes back as vault.configured, which loads
		// the vault's data (event): a load here too would do it twice. A
		// refused change loads it, so the rows show what holds.
		if err != nil {
			a.report(err)
			a.loadVaultData()
		}
	})
}

func (a *App) setBackground(keep bool) {
	b, err := system.SetBackground(a.cfg.Getenv, keep)
	if err != nil {
		a.report(err)
		return
	}
	a.sets.background = &b
	g := b.Generation
	a.sets.awaited = &g
}

func (a *App) syncAction(key string) {
	if a.vault == nil {
		return
	}
	switch key {
	case "join":
		a.sets.joining = &joinFlow{mode: modeJoin, phase: joinForm, vaultID: a.vault.ID}
		a.sets.joining.onDone = a.closeJoining
	case "pair":
		j := &joinFlow{mode: modePair, phase: joinForm, vaultID: a.vault.ID, onDone: a.closeJoining}
		if a.syncInfo != nil {
			j.url = a.syncInfo.URL
		}
		a.sets.joining = j
	case "leave":
		a.leaveHub()
	}
}

func (a *App) closeJoining() { a.sets.joining = nil }

func (a *App) leaveHub() {
	id, name := a.vault.ID, a.vault.Name
	a.Dialogs.Ask(frostui.Confirm{Title: `Stop syncing "` + name + `"?`, Body: "This vault's files stay on this device, and the hub keeps its copy.", Action: "Leave", Destructive: true,
		OnConfirm: func() {
			a.call(func(ctx context.Context, c *api.Client) error {
				return c.Sync().Leave(ctx, &api.SyncLeaveParams{VaultID: id})
			}, func(err error) {
				a.report(err)
				a.loadVaultData()
			})
		}})
}

func (a *App) revokeDevice(device string) {
	n, err := strconv.ParseInt(device, 10, 64)
	if err != nil || a.sets.hub == nil {
		return
	}
	name := "this device"
	for _, d := range a.sets.hub.devices {
		if d.ID == n {
			name = d.Name
		}
	}
	a.Dialogs.Ask(frostui.Confirm{Title: "Revoke " + name + "?", Body: "Its token stops working at once, and it must pair again to sync.", Action: "Revoke", Destructive: true,
		OnConfirm: func() {
			a.call(func(ctx context.Context, c *api.Client) error {
				return c.Hub().Revoke(ctx, &api.HubRevokeParams{Device: n})
			}, func(err error) {
				a.report(err)
				a.sets.load(a)
			})
		}})
}

func stamp(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

func portalText(p *system.PortalAnswer) string {
	if p == nil {
		return "Not asked yet."
	}
	ok := p.Response != nil && *p.Response == 0
	if ok && p.Background != nil && *p.Background {
		if p.Autostart != nil && *p.Autostart {
			return "Allowed; notesd starts at login."
		}
		return "Allowed; notesd does not start at login."
	}
	if (p.Response != nil && *p.Response == 1) || (ok && p.Background != nil && !*p.Background) {
		return "Refused. The desktop stops notesd about 45 seconds after the last window closes, and at login. " +
			"Allow it in GNOME Settings, Apps, Frostnotes, Run in Background, then open Frostnotes again."
	}
	if p.Error != nil {
		return "Not known: the desktop did not answer (" + *p.Error + ")."
	}
	return "Not known: the desktop did not answer."
}

func toggle(key, name, description string, on bool) settingRow {
	return settingRow{id: "pref:" + key, name: name, description: description, kind: toggleControl, on: on}
}

func choice(id, name, value string, options ...option) settingRow {
	return settingRow{id: id, name: name, kind: selectControl, value: value, options: options}
}

// settingsSections builds the dialog's sections.
func (a *App) settingsSections() []settingsSection {
	p := a.Prefs
	v := a.settings
	text := func(field, name, description, placeholder, value string) settingRow {
		return settingRow{id: "vault:" + field, name: name, description: description, kind: textControl, value: value, placeholder: placeholder, disabled: a.settings == nil}
	}
	disabled := v == nil
	if v == nil {
		v = &api.VaultSettings{}
	}
	sections := []settingsSection{
		{"editor", "Editor", []settingRow{
			choice("pref:defaultView", "Default view for new tabs", p.DefaultView, option{"edit", "Editing view"}, option{"read", "Reading view"}),
			choice("pref:editingMode", "Default editing mode", p.EditingMode, option{"live", "Live Preview"}, option{"source", "Source mode"}),
			toggle("inlineTitle", "Show inline title", "The file's name as an editable title above the note.", p.InlineTitle),
			toggle("viewHeader", "Show view header", "The bar above a tab's view.", p.ViewHeader),
			toggle("readableLineLength", "Readable line length", "Limit a note's width to 700 pixels.", p.ReadableLineLength),
			choice("pref:properties", "Properties in document", p.Properties, option{"visible", "Visible"}, option{"hidden", "Hidden"}, option{"source", "Source"}),
			toggle("lineNumbers", "Show line numbers", "", p.LineNumbers),
			toggle("indentGuides", "Show indentation guides", "", p.IndentGuides),
			toggle("foldHeadings", "Fold headings", "", p.FoldHeadings),
			toggle("foldIndent", "Fold indent", "", p.FoldIndent),
			toggle("autoPairBrackets", "Auto-pair brackets", "", p.AutoPairBrackets),
			toggle("autoPairMarkdown", "Auto-pair Markdown syntax", "", p.AutoPairMarkdown),
			toggle("smartLists", "Smart lists", "", p.SmartLists),
			toggle("indentWithTabs", "Indent using tabs", "", p.IndentWithTabs),
			{id: "pref:tabSize", name: "Tab size", kind: numberControl, number: p.TabSize, min: 1, max: 8},
			toggle("confirmDelete", "Confirm file deletion", "Ask before a file or folder is deleted.", p.ConfirmDelete),
			toggle("loadRemoteImages", "Load remote images", "Show images from the web in notes.", p.LoadRemoteImages),
		}},
		{"files", "Files and links", []settingRow{
			withDisabled(choice("vault:newNoteLocation", "Default location for new notes", string(v.NewNoteLocation), option{"root", "Vault folder"}, option{"current", "Same folder as current file"}, option{"folder", "In the folder specified below"}), disabled),
			withDisabled(text("newNoteFolder", "Folder to create new notes in", "", "Example: folder 1/folder 2", v.NewNoteFolder), v.NewNoteLocation != api.NewNoteLocationFolder),
			withDisabled(choice("vault:linkFormat", "New link format", string(v.LinkFormat), option{"shortest", "Shortest path when possible"}, option{"relative", "Relative path to file"}, option{"absolute", "Absolute path in vault"}), disabled),
			{id: "vault:wikilinks", name: "Use [[Wikilinks]]", kind: toggleControl, on: v.Wikilinks, disabled: disabled},
			{id: "vault:updateLinks", name: "Automatically update internal links", description: "Off: asked on each move.", kind: toggleControl, on: v.UpdateLinks, disabled: disabled},
			text("attachmentFolder", "Default location for new attachments", `"/" is the vault folder.`, "/", v.AttachmentFolder),
			toggle("showUnsupported", "Show unsupported files", "Files of other types in the Files pane.", p.ShowUnsupported),
			text("ignore", "Excluded files", "Patterns, separated by commas.", "", strings.Join(v.Ignore, ", ")),
		}},
		{"appearance", "Appearance", []settingRow{
			choice("pref:theme", "Theme", p.Theme, option{"system", "System"}, option{"light", "Light"}, option{"dark", "Dark"}),
			{id: "pref:fontSize", name: "Font size", kind: numberControl, number: p.FontSize, min: 10, max: 30},
		}},
		{"hotkeys", "Hotkeys", a.hotkeyRows()},
		{"daily", "Daily notes", []settingRow{
			text("dailyFormat", "Date format", "Moment.js syntax; '/' makes folders.", "YYYY-MM-DD", v.DailyFormat),
			text("dailyFolder", "New file location", "Empty is the vault folder.", "", v.DailyFolder),
			text("dailyTemplate", "Template file location", "", "", v.DailyTemplate),
		}},
		{"templates", "Templates", []settingRow{
			text("templateFolder", "Template folder location", "", "", v.TemplateFolder),
			text("templateDateFormat", "Date format", "{{date}}'s moment.js format.", "YYYY-MM-DD", v.TemplateDateFormat),
			text("templateTimeFormat", "Time format", "{{time}}'s moment.js format.", "HH:mm", v.TemplateTimeFormat),
		}},
		{"sync", "Sync", a.syncRows()},
	}
	if a.sets.hub != nil {
		sections = append(sections, hubSection(a.sets.hub))
	}
	if b := a.sets.background; b != nil {
		text := portalText(b.Portal)
		if a.sets.awaited != nil {
			text = "Asking the desktop…"
		}
		sections = append(sections, settingsSection{"background", "Background", []settingRow{
			{id: "bg:keep", name: "Keep syncing when no window is open", kind: toggleControl, on: b.KeepRunning,
				description: "On: notesd keeps your notes in sync after the last window closes, and starts when you log in. Off: notesd stops when the last window closes and does not start at login; notes sync only while a Frostnotes window is open."},
			{id: "bg:desktop", name: "Desktop permission", description: "The desktop's own switch. Off there wins over On here.", kind: infoControl, text: text},
		}})
	}
	return sections
}

func withDisabled(r settingRow, disabled bool) settingRow {
	r.disabled = disabled
	return r
}

func (a *App) hotkeyRows() []settingRow {
	var rows []settingRow
	for _, s := range a.table {
		if len(s.Keys) == 0 {
			continue
		}
		var caps []string
		for i, k := range s.Keys {
			if i > 0 {
				caps = append(caps, "or")
			}
			caps = append(caps, keyCaps(k)...)
		}
		rows = append(rows, settingRow{id: "hotkey:" + s.ID, name: s.Title, kind: keysControl, keys: caps})
	}
	return rows
}

func (a *App) syncRows() []settingRow {
	s := a.syncInfo
	if s == nil {
		if a.vm.canSync != nil && !*a.vm.canSync {
			return []settingRow{{id: "sync:none", name: "Sync", kind: infoControl, text: "This notesd does not sync."}}
		}
		return []settingRow{{id: "sync:join", name: "Sync with a hub", description: "Join a vault on a hub. Notes then sync on every device that joins it.", kind: buttonControl, label: "Join…"}}
	}
	rows := []settingRow{
		{id: "sync:hub", name: "Hub", kind: infoControl, text: s.URL},
		{id: "sync:vault", name: "Hub vault", kind: infoControl, text: s.HubVault},
		{id: "sync:state", name: "State", kind: infoControl, text: syncText(s)},
		{id: "sync:conflicts", name: "Conflicts", kind: infoControl, text: strconv.FormatInt(s.Conflicts, 10)},
	}
	if s.State == api.SyncStateRevoked {
		rows = append(rows, settingRow{id: "sync:pair", name: "Pair again", description: "The hub refused this device's token. Pair again with a new code to sync again.", kind: buttonControl, label: "Pair again…"})
	}
	return append(rows, settingRow{id: "sync:leave", name: "Stop syncing", description: "This vault's files stay on this device, and the hub keeps its copy.", kind: buttonControl, label: "Leave", danger: true})
}

func hubSection(h *hubView) settingsSection {
	rows := []settingRow{{id: "hub:id", name: "Hub", kind: infoControl, text: h.status.Hub}}
	if h.status.URL != nil {
		rows = append(rows, settingRow{id: "hub:url", name: "Devices' URL", kind: infoControl, text: *h.status.URL})
	}
	for _, d := range h.devices {
		seen := ", never seen"
		if d.SeenAt != nil {
			seen = ", last seen " + stamp(*d.SeenAt)
		}
		connected := ""
		if d.Connected {
			connected = ", connected"
		}
		r := settingRow{id: "hub:revoke:" + strconv.FormatInt(d.ID, 10), name: d.Name, description: "Paired " + stamp(d.PairedAt) + seen + connected, kind: buttonControl, label: "Revoke", danger: true}
		if d.Revoked {
			r.kind, r.text = infoControl, "Revoked"
		}
		rows = append(rows, r)
	}
	if len(h.devices) == 0 {
		rows = append(rows, settingRow{id: "hub:none", name: "Devices", kind: infoControl, text: "No device has paired."})
	}
	for _, p := range h.pending {
		from := p.Addr
		if from == "" {
			from = "a node lodge did not name"
		}
		r := settingRow{id: "hub:pending:" + p.Code, name: p.Name + " (pending)", description: "From " + from + "; expires " + stamp(p.ExpiresAt), kind: infoControl, text: "notesctl hub approve " + p.Code}
		if p.Approved {
			r.text = "Approved"
		}
		rows = append(rows, r)
	}
	return settingsSection{"hub", "Hub", rows}
}

// notesdCause says why notesd is not running: in the Flatpak, the portal's
// fix, or else notesd.log's last line.
func notesdCause(getenv system.Getenv) string {
	log, ok := system.ReadNotesdLog(getenv)
	if !ok {
		return ""
	}
	if log.PortalRefused {
		return "notesd did not start, because the Flatpak portal did not start it. Run flatpak override --user --show org.frostyard.Frostnotes: it must not list a blocked org.freedesktop.portal.Flatpak. " +
			"Remove the overrides with flatpak override --user --reset org.frostyard.Frostnotes, if it does, and check the portal with systemctl --user status flatpak-portal.service. See " + log.Path + "."
	}
	if log.LastLine != "" {
		return log.LastLine + " (" + log.Path + ")"
	}
	return ""
}
