package main

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() { nsgo.Run(root, nsgo.WindowSize(1160, 760)) }

type environment struct{ *core.Scope }

func (e environment) After(d time.Duration, f func()) func() {
	stop := e.AfterFunc(d, f)
	return func() { stop() }
}

type workspace struct {
	cx                                       *core.Scope
	session                                  *session.Session
	vaults                                   *core.Signal[[]api.Vault]
	files                                    *core.Signal[[]api.NoteEntry]
	vault                                    *core.Signal[int64]
	doc                                      *core.Signal[*notes.Document]
	revision                                 *core.Signal[int]
	connection, problem                      *core.Signal[string]
	preview                                  *core.Signal[bool]
	catalogRequest, filesRequest, navigation int
}

func root(cx *core.Scope) core.Node {
	cx.SetTitle("FrostNotes")
	w := &workspace{
		cx:      cx,
		session: session.New(environment{cx}, platformDialer(), 0),
		vaults:  core.NewSignalFunc(cx, []api.Vault(nil), slices.Equal),
		files: core.NewSignalFunc(cx, []api.NoteEntry(nil), func(a, b []api.NoteEntry) bool {
			return slices.EqualFunc(a, b, func(x, y api.NoteEntry) bool { return x.Path == y.Path && x.Title == y.Title })
		}),
		vault:      core.NewSignal(cx, int64(0)),
		doc:        core.NewSignal(cx, (*notes.Document)(nil)),
		revision:   core.NewSignal(cx, 0),
		connection: core.NewSignal(cx, "Connecting to notesd…"),
		problem:    core.NewSignal(cx, ""),
		preview:    core.NewSignal(cx, false),
	}
	w.session.OnChange(func() {
		if w.session.State() == session.Connected {
			w.connection.Set("Connected")
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
	w.session.Start()
	explorer := workspaceui.FileExplorer{
		TreeWidth: 230, Tree: ui.Sidebar{SectionsFunc: w.noteSections, Selected: core.NewMemo(cx, w.selectedPath).Get, OnSelect: w.open},
		Preview: w.editor(), HasPreview: w.preview.Get, BackToTree: func() { w.preview.Set(false) },
	}
	shell := workspaceui.Shell{
		Brand: "FrostNotes", Header: []core.Node{core.Text{ContentFunc: w.title, Style: ui.TypeTitle}},
		Actions:       []core.Node{ui.IconButton{Label: "Refresh vaults and notes", Icon: lucide.RefreshCw, OnPress: w.loadVaults}},
		SidebarFooter: []core.Node{core.Text{ContentFunc: w.connection.Get, Style: ui.TypeCaption.Merge(core.Style{Padding: core.All(12)})}},
		Sections:      w.vaultSections, Selected: func() string { return strconv.FormatInt(w.vault.Get(), 10) },
		Navigate: func(key string) {
			id, err := strconv.ParseInt(key, 10, 64)
			if err == nil && id != w.vault.GetUntracked() {
				w.leave(func() { w.selectVault(id) })
			}
		},
		Content: []core.Node{explorer},
	}
	return core.View{Style: ui.FillRest, Children: []core.Node{shell, core.Dynamic(w.notice)}}
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
		section.Items = append(section.Items, ui.SidebarItem{ID: strconv.FormatInt(v.ID, 10), Text: v.Name, Icon: lucide.Library})
	}
	if len(section.Items) == 0 && w.connection.Get() == "Connected" {
		section.Note = core.Text{Content: "No vaults available"}
	}
	return []ui.SidebarSection{section}
}

func (w *workspace) noteSections() []ui.SidebarSection {
	var sections []ui.SidebarSection
	indices := map[string]int{}
	for _, f := range w.files.Get() {
		dir := path.Dir(f.Path)
		i, ok := indices[dir]
		if !ok {
			title := dir
			if dir == "." || dir == "/" {
				title = "Notes"
			}
			i = len(sections)
			indices[dir] = i
			sections = append(sections, ui.SidebarSection{Title: title, Collapsible: true})
		}
		sections[i].Items = append(sections[i].Items, ui.SidebarItem{ID: f.Path, Text: f.Title, Icon: lucide.FileText})
	}
	if len(sections) == 0 {
		return []ui.SidebarSection{{Title: "Notes", Note: core.Text{Content: "No notes available"}}}
	}
	return sections
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
	w.vault.Set(id)
	w.files.Set(nil)
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
		})
	}()
}

func (w *workspace) onEvent(_ int64, ev api.Event) {
	id := w.vault.GetUntracked()
	switch e := ev.(type) {
	case api.VaultAdded, api.VaultRemoved:
		w.loadVaults()
	case api.NoteChanged:
		if e.VaultID == id && !slices.ContainsFunc(w.files.GetUntracked(), func(f api.NoteEntry) bool { return f.Path == e.Path }) {
			w.loadFiles()
		}
	case api.VaultScan:
		if e.Status.VaultID == id && e.Status.State == api.ScanStateIdle {
			w.loadFiles()
		}
	case api.NoteMoved:
		if e.VaultID == id {
			w.loadFiles()
		}
	case api.NoteRemoved:
		if e.VaultID == id {
			w.loadFiles()
		}
	}
}

func (w *workspace) editor() core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		d := w.doc.Get()
		if d == nil {
			return workspaceui.Page(core.Text{Content: "Select a note to edit", Style: ui.TypeTitle})
		}
		status := func() string {
			w.revision.Get()
			if reason := d.ReadOnly(); reason != "" {
				return reason
			}
			return [...]string{"Loading…", "Note is missing", "Saved", "Unsaved changes", "Saving…"}[d.Status()]
		}
		return core.View{Style: ui.FillRest.Merge(core.Style{Padding: core.All(16), Gap: 10}), Children: []core.Node{
			workspaceui.Row(core.Text{ContentFunc: func() string { w.revision.Get(); return d.Path() }, Style: ui.TypeTitle}, core.Text{ContentFunc: status, Style: ui.TypeCaption}, ui.Button{Text: "Save", Icon: lucide.Save, Disabled: func() bool { w.revision.Get(); return !d.Unsaved() }, OnPress: d.RetryNow}),
			core.Show{When: func() bool { w.revision.Get(); return needsRecovery(d) }, Then: func(*core.Scope) core.Node {
				return ui.Input{Label: "Recovered draft", Multiline: true, Readonly: true, Style: ui.FillRest, DefaultValue: d.Notice().Text}
			}, Else: func(*core.Scope) core.Node {
				return ui.Input{Label: "Markdown source", Multiline: true, NoAutoCorrect: true, Style: ui.FillRest, Value: func() string { w.revision.Get(); return d.Text() }, Disabled: func() bool {
					w.revision.Get()
					return !d.Loaded() || d.Status() == notes.StatusMissing || d.ReadOnly() != ""
				}, OnChange: func(value string) { d.Local(notes.Diff(d.Text(), value)) }}
			}},
		}}
	})
}

func (w *workspace) notice(*core.Scope) core.Node {
	w.revision.Get()
	if d := w.doc.Get(); d != nil {
		if n := d.Notice(); n != nil {
			title, detail := "Could not save", n.Reason
			switch n.Kind {
			case notes.NoticeConflict:
				title = "Edits overlapped"
				detail = fmt.Sprintf("The overlapping lines from %s are in the note. Your draft is in version %d.", n.By, n.Version)
			case notes.NoticeRefused:
				title = "Save base is unavailable"
				detail = "Copy the recovered draft from the editor before dismissing this message. Dismiss to return to the stored note."
			case notes.NoticeDeleted:
				title = "Note deleted"
			case notes.NoticeLoadFailed:
				title = "Could not open note"
			}
			return workspaceui.Notice{Kind: ui.ToastWarning, Title: title, Detail: detail, OnDismiss: d.Dismiss}
		}
	}
	if msg := w.problem.Get(); msg != "" {
		return workspaceui.Notice{Kind: ui.ToastWarning, Title: "Workspace", Detail: msg, OnDismiss: func() { w.problem.Set("") }}
	}
	return nil
}
