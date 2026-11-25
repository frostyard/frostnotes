package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() {
	finish := desktopLifetime()
	defer finish()
	nsgo.Run(root, nsgo.WindowSize(1160, 760), nsgo.WindowConstraints(core.WindowConstraints{MinWidth: minWidth, MinHeight: minHeight}))
}

type environment struct{ *core.Scope }

func (e environment) After(d time.Duration, f func()) func() {
	// AfterFunc belongs to the currently executing NSGo view. Arm it under
	// the workspace so replacing a line or switching editor modes cannot
	// cancel a pending document save. The session runs on the UI thread.
	var stop func() bool
	cancelled := false
	e.Post(func() {
		if !cancelled {
			stop = e.AfterFunc(d, f)
		}
	})
	return func() {
		cancelled = true
		if stop != nil {
			stop()
		}
	}
}

type workspace struct {
	removed     map[int64][]string
	syncInfo    *core.Signal[*api.SyncStatus]
	syncRequest int

	cx                                       *core.Scope
	session                                  *session.Session
	vaults                                   *core.Signal[[]api.Vault]
	files                                    *core.Signal[[]api.NoteEntry]
	vault                                    *core.Signal[int64]
	doc                                      *core.Signal[*notes.Document]
	revision                                 *core.Signal[int]
	connection, problem                      *core.Signal[string]
	action                                   *core.Signal[string]
	preview, live                            *core.Signal[bool]
	appearance                               *core.Signal[core.Appearance]
	catalogRequest, filesRequest, navigation int
	// chosen is the notes tree's choice.
	chosen *ui.Selection
	// listed says the shown vault's notes have been read, so an empty tree
	// is a vault with no notes and not one still being read.
	listed *core.Signal[bool]
	// settings shows Settings in place of the vault, where the app has
	// Settings: hasSettings.
	settings    *core.Signal[bool]
	hasSettings bool
	// contentWidth is the width of the pane under the toolbar.
	contentWidth *core.Signal[core.Points]
	// filter is what the field over the notes tree holds, and filterInput
	// the field, which mod+F focuses.
	filter      *core.Signal[string]
	filterInput *core.Ref
}

// connected is what connection holds while notesd answers.
const connected = "Connected"

func root(cx *core.Scope) core.Node {
	cx.SetTitle("FrostNotes")
	// The Frostyard look and the bundled Inter. A font the platform refuses
	// leaves the text in the system's face.
	_ = workspaceui.Apply(cx)
	appearance := core.AppearanceSystem
	if ownAppearance {
		// Lodge sets the web build's appearance; a desktop keeps its own.
		appearance = savedAppearance(cx)
		cx.SetAppearance(appearance)
	}
	w := &workspace{
		syncInfo: core.NewSignal(cx, (*api.SyncStatus)(nil)),
		removed:  make(map[int64][]string),
		cx:       cx,
		session:  session.New(environment{cx}, platformDialer(), 0),
		vaults:   core.NewSignalFunc(cx, []api.Vault(nil), slices.Equal),
		files: core.NewSignalFunc(cx, []api.NoteEntry(nil), func(a, b []api.NoteEntry) bool {
			return slices.EqualFunc(a, b, func(x, y api.NoteEntry) bool { return x.Path == y.Path && x.Title == y.Title })
		}),
		vault:      core.NewSignal(cx, int64(0)),
		doc:        core.NewSignal(cx, (*notes.Document)(nil)),
		revision:   core.NewSignal(cx, 0),
		connection: core.NewSignal(cx, "Connecting to notesd…"),
		problem:    core.NewSignal(cx, ""),
		action:     core.NewSignal(cx, ""),
		preview:    core.NewSignal(cx, false),
		live:       core.NewSignal(cx, false),
		appearance: core.NewSignal(cx, appearance),
		listed:     core.NewSignal(cx, false),
		settings:   core.NewSignal(cx, false),

		contentWidth: core.NewSignal(cx, core.Points(0)),
		filter:       core.NewSignal(cx, ""),
		filterInput:  &core.Ref{},
	}
	w.session.OnChange(func() {
		if w.session.State() == session.Connected {
			w.connection.Set(connected)
			w.loadVaults()
			return
		}
		w.connection.Set("Connecting to notesd · " + w.session.Reason())
	})
	w.session.OnEvent = w.onEvent
	cx.Cleanup(w.session.Close)
	cx.OnQuitRequest(func(q *core.QuitRequest) {
		d := w.doc.GetUntracked()
		if needsRecovery(d) {
			q.Cancel()
			return
		}
		if d != nil && d.Unsaved() {
			answer := q.Defer()
			d.Flush(func() { answer(!d.Unsaved() && !needsRecovery(d)) })
		}
	})
	guardBrowserDraft(cx, func() bool { w.revision.Get(); d := w.doc.Get(); return d != nil && (d.Unsaved() || needsRecovery(d)) })
	cx.OnKey("mod+s", func() {
		if d := w.doc.GetUntracked(); d != nil && d.Unsaved() {
			d.RetryNow()
		}
	})
	w.bindNoteActions()
	w.bindSettingsKey()
	w.keepWindow(cx)
	w.session.Start()
	// The explorer outlives the views shown in its place, so a look at
	// Settings leaves the tree, the editor and its history as they were.
	explorer := core.NewHolder(cx, func(cx *core.Scope) core.Node {
		return workspaceui.FileExplorer{
			TreeWidth: 230, Tree: w.notesTree(cx),
			// Back at the tree, no row is chosen, so a press of the note
			// that was open opens it again.
			Preview: w.editor(), HasPreview: w.preview.Get, BackToTree: func() { w.preview.Set(false); w.chosen.Clear() },
		}
	})
	settings := w.settingsView(cx)
	w.hasSettings = settings != nil
	shell := workspaceui.Shell{
		Widths: w.paneWidths(), OnWidthsChange: w.savePanes,
		Brand: "FrostNotes", Header: []core.Node{w.header()}, Actions: []core.Node{w.actions(cx)},
		SidebarFooter: []core.Node{w.sidebarFoot()},
		Sections:      w.vaultSections, Selected: func() string { return strconv.FormatInt(w.vault.Get(), 10) },
		Navigate: func(key string) {
			id, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				return
			}
			// A press of a vault's row leaves Settings, the shown vault's too.
			w.settings.Set(false)
			if id != w.vault.GetUntracked() {
				w.leave(func() { w.selectVault(id) })
			}
		},
		Content: []core.Node{core.View{Style: ui.FillRest, OnLayout: func(b core.Bounds) { w.contentWidth.Set(b.Size.Width) }, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if settings != nil && w.settings.Get() {
				return settings
			}
			if w.vault.Get() == 0 {
				if welcome := w.vaultWelcome(); welcome != nil {
					return welcome
				}
			}
			return explorer.Place()
		})}}},
	}
	return workspaceui.Frame(shell, core.Dynamic(w.notice), core.Dynamic(w.noteDialog))
}

func (w *workspace) title() string {
	for _, v := range w.vaults.Get() {
		if v.ID == w.vault.Get() {
			return v.Name
		}
	}
	return "Vaults"
}

func (w *workspace) vaultSections() []ui.SidebarSection {
	section := ui.SidebarSection{Title: "Vaults"}
	for _, v := range w.vaults.Get() {
		section.Items = append(section.Items, ui.SidebarItem{ID: strconv.FormatInt(v.ID, 10), Text: v.Name, Content: workspaceui.NavLabel{Icon: lucide.Library, Text: v.Name}})
	}
	if len(section.Items) == 0 && w.connection.Get() == connected {
		section.Note = core.Text{Content: "No vaults available"}
	}
	return []ui.SidebarSection{section}
}

func (w *workspace) selectedPath() string {
	w.revision.Get()
	if d := w.doc.Get(); d != nil {
		return d.Path()
	}
	return ""
}

func needsRecovery(d *notes.Document) bool {
	return d != nil && d.Notice() != nil && d.Notice().Kind == notes.NoticeRefused
}

// Flush before releasing the current note. If its save returns a conflict,
// keep the recovery notice in view. A later click supersedes a pending one.
func (w *workspace) leave(next func()) {
	w.navigation++
	request := w.navigation
	d := w.doc.GetUntracked()
	if needsRecovery(d) {
		return
	}
	if d == nil {
		next()
		return
	}
	finish := func() {
		if request != w.navigation || w.doc.GetUntracked() != d || d.Unsaved() || needsRecovery(d) {
			return
		}
		w.session.Release(d)
		w.doc.Set(nil)
		w.preview.Set(false)
		next()
	}
	if d.Unsaved() {
		d.Flush(func() {
			if n := d.Notice(); n == nil || n.Kind != notes.NoticeConflict {
				finish()
			}
		})
		return
	}
	finish()
}

func (w *workspace) selectVault(id int64) {
	if !isVaultAction(w.action.GetUntracked()) {
		w.action.Set("")
	}
	w.syncInfo.Set(nil)
	w.vault.Set(id)
	w.loadSync()
	w.files.Set(nil)
	w.listed.Set(false)
	w.loadFiles()
}

func (w *workspace) open(p string) {
	if d := w.doc.GetUntracked(); d != nil && d.Path() == p {
		w.preview.Set(true)
		return
	}
	w.leave(func() {
		d := w.session.Open(w.vault.GetUntracked(), p)
		w.doc.Set(d)
		w.preview.Set(true)
		w.problem.Set("")
		d.OnChange(func() {
			if w.doc.GetUntracked() == d {
				w.revision.Set(w.revision.GetUntracked() + 1)
			}
		})
	})
}

func (w *workspace) loadVaults() {
	c := w.session.Client()
	if c == nil {
		return
	}
	w.catalogRequest++
	request := w.catalogRequest
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		vs, err := c.Vault().List(ctx, &api.VaultListParams{})
		w.cx.Post(func() {
			if request != w.catalogRequest || c != w.session.Client() {
				return
			}
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			w.vaults.Set(vs)
			if slices.ContainsFunc(vs, func(v api.Vault) bool { return v.ID == w.vault.GetUntracked() }) {
				w.loadFiles()
				w.loadSync()
				return
			}

			var id int64
			if len(vs) > 0 {
				id = vs[0].ID
			}
			w.leave(func() { w.selectVault(id) })
		})
	}()
}

func (w *workspace) loadFiles() {
	c := w.session.Client()
	id := w.vault.GetUntracked()
	if c == nil || id == 0 {
		return
	}
	w.filesRequest++
	request := w.filesRequest
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		fs, err := c.Note().List(ctx, &api.NoteListParams{VaultID: id})
		w.cx.Post(func() {
			if request != w.filesRequest || w.vault.GetUntracked() != id || w.session.Client() != c {
				return
			}
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			w.files.Set(fs)
			w.listed.Set(true)
		})
	}()
}

func (w *workspace) onEvent(_ int64, ev api.Event) {
	id := w.vault.GetUntracked()
	switch e := ev.(type) {
	case api.SyncChanged:
		if id == e.Status.VaultID {
			w.syncRequest++
			w.syncInfo.Set(&e.Status)
		}
	case api.VaultAdded, api.VaultRemoved:
		w.loadVaults()
	case api.NoteChanged:
		w.forgetRemoved(e.VaultID, e.Path)
		if e.VaultID == id && !slices.ContainsFunc(w.files.GetUntracked(), func(f api.NoteEntry) bool { return f.Path == e.Path }) {
			w.loadFiles()
		}
	case api.VaultScan:
		if e.Status.VaultID == id && e.Status.State == api.ScanStateIdle {
			w.loadFiles()
		}
	case api.NoteMoved:
		w.forgetRemoved(e.VaultID, e.To)
		if e.VaultID == id {
			w.loadFiles()
		}
	case api.NoteRemoved:
		w.rememberRemoved(e.VaultID, e.Path)
		if d := w.doc.GetUntracked(); d != nil && d.Vault() == e.VaultID && d.Path() == e.Path && !d.Unsaved() && d.Notice() != nil && d.Notice().Kind == notes.NoticeDeleted {
			if a := w.action.GetUntracked(); a == trashNote || a == moveNote {
				w.action.Set("")
			}
			w.leave(func() {})
		}
		if e.VaultID == id {
			w.loadFiles()
		}
	}
}

// editor is the pane of the open note: what notesd has to say about it,
// as a banner, over its text. The note's path, its state and its actions
// are in the window's toolbar.
func (w *workspace) editor() core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		d := w.doc.Get()
		if d == nil {
			return workspaceui.EmptyState{Icon: lucide.FileText, Title: "Select a note to edit", Hint: "Choose a note in the tree, or make a new one."}
		}
		return core.View{Style: ui.FillRest.Merge(core.Style{Padding: core.All(workspaceui.SpaceL), Gap: 10}), Children: []core.Node{
			core.Dynamic(func(*core.Scope) core.Node { return w.noteBanner(d) }),
			core.Show{When: func() bool { w.revision.Get(); return needsRecovery(d) }, Then: func(*core.Scope) core.Node {
				return ui.Input{Label: "Recovered draft", Multiline: true, Readonly: true, Style: ui.FillRest, DefaultValue: d.Notice().Text}
			}, Else: func(*core.Scope) core.Node {
				return core.Show{When: func() bool {
					w.revision.Get()
					return !d.Loaded() && d.Status() != notes.StatusMissing && d.Notice() == nil
				}, Then: func(*core.Scope) core.Node {
					return ui.Skeleton{Lines: 6, Label: "Loading note"}
				}, Else: func(*core.Scope) core.Node {
					return core.Show{When: w.live.Get, Then: func(cx *core.Scope) core.Node { return w.inlineEditor(cx, d) }, Else: func(*core.Scope) core.Node {
						return ui.Input{Label: "Markdown source", Multiline: true, NoAutoCorrect: true, Style: ui.FillRest, Value: func() string { w.revision.Get(); return d.Text() }, Disabled: func() bool {
							w.revision.Get()
							return !d.Loaded() || d.Status() == notes.StatusMissing || d.ReadOnly() != ""
						}, OnChange: func(value string) { d.Local(notes.Diff(d.Text(), value)) }}
					}}
				}}
			}},
		}}
	})
}

// noteBanner is what notesd has to say about the open note, in the tone
// of what happened, and nothing while it has nothing to say.
func (w *workspace) noteBanner(d *notes.Document) core.Node {
	w.revision.Get()
	n := d.Notice()
	if n == nil {
		return nil
	}
	tone, icon, text := workspaceui.Danger, lucide.CircleAlert, "Could not save. "+n.Reason
	switch n.Kind {
	case notes.NoticeConflict:
		tone, icon = workspaceui.Warning, lucide.TriangleAlert
		text = fmt.Sprintf("Edits overlapped. The overlapping lines from %s are in the note. Your draft is in version %d.", n.By, n.Version)
	case notes.NoticeRefused:
		text = "Save base is unavailable. Copy the recovered draft below before dismissing this message. Dismiss to return to the stored note."
	case notes.NoticeDeleted:
		tone, icon = workspaceui.Warning, lucide.TriangleAlert
		text = strings.TrimSpace("Note deleted. " + n.Reason)
	case notes.NoticeLoadFailed:
		text = "Could not open note. " + n.Reason
	}
	return workspaceui.Banner{Tone: tone, Icon: icon, Text: text, Action: "Dismiss", OnAction: d.Dismiss}
}

// notice is what went wrong in the app itself, over the window.
func (w *workspace) notice(*core.Scope) core.Node {
	if msg := w.problem.Get(); msg != "" {
		return workspaceui.Notice{Kind: ui.ToastError, Title: "Something went wrong", Detail: msg, OnDismiss: func() { w.problem.Set("") }}
	}
	return nil
}
