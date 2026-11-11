package window

import (
	"context"
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/commands"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/panes"
	"github.com/frostyard/frostnotes/internal/desktop/prefs"
	"github.com/frostyard/frostnotes/internal/desktop/system"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
	"github.com/frostyard/frostnotes/internal/render"
)

// The palette's sources besides the commands.
const (
	sourceNotes     = "notes"
	sourceFolders   = "folders"
	sourceTemplates = "templates"
	sourceHeadings  = "headings"
)

// moveTarget is what the folder chooser moves.
type moveTarget struct {
	path   string
	folder bool
}

// heldMerge is the text a merge view compares with, which no version holds.
type heldMerge struct{ text, title string }

// goToSubpath has the note at p show a heading or block once its editor
// has its text.
func (a *App) goToSubpath(p, sub string) {
	if sub != "" {
		a.pending = &pendingLine{tabID: a.Store.Workspace().ActiveTab().ID, path: p, subpath: sub}
	}
}

// pendingLine is where a note opens once its editor has its text: a line,
// or a link's "#Heading" or "#^block".
type pendingLine struct {
	tabID   string
	subpath string
	path    string
	line    int
}

func whereOf(w editor.Where) string {
	switch w {
	case editor.WhereTab, editor.WhereWindow: // a new window is a new tab until the app has more windows
		return workspace.NewTab
	case editor.WhereRight:
		return workspace.ToRight
	}
	return workspace.Here
}

func nameOf(p string) string { return strings.TrimSuffix(path.Base(p), ".md") }

func parentOf(p string) string {
	if d := path.Dir(p); d != "." {
		return d
	}
	return ""
}

func join(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

func (a *App) viewOf(p string) workspace.View {
	if strings.HasSuffix(p, ".md") {
		return workspace.Note(p, a.Prefs.DefaultView)
	}
	return workspace.View{Type: workspace.ViewFile, Path: p}
}

func (a *App) open(p, where string) {
	a.updateWorkspace(func(w *workspace.Workspace) { w.Open(a.viewOf(p), where) })
}

func (a *App) openAtLine(p string, line int, where string) {
	a.updateWorkspace(func(w *workspace.Workspace) { w.Open(workspace.Note(p, workspace.ModeEdit), where) })
	a.pending = &pendingLine{tabID: a.Store.Workspace().ActiveTab().ID, path: p, line: line}
}

// jumpToLine puts the active editor's caret on line, from 1.
func (a *App) jumpToLine(line int) {
	if e := a.activeEditor(); e != nil {
		goToLine(e, line)
	}
}

func goToLine(e *editor.NoteEditor, line int) {
	if line < 1 {
		line = 1
	}
	e.State.GoTo(code.Pos{Line: line - 1})
	e.State.ScrollTo(line - 1)
}

func (a *App) hasFile(p string) bool {
	_, ok := a.Catalog.File(p)
	return ok
}

func (a *App) tagNames() []string {
	if a.Catalog == nil {
		return nil
	}
	return a.Catalog.Sources(func() string { return "" }, a.openText).Tags()
}

// newNoteFolder is the folder a new note goes in, by the vault's settings
// (Obsidian's Default location for new notes).
func (a *App) newNoteFolder() string {
	if a.settings == nil {
		return ""
	}
	switch a.settings.NewNoteLocation {
	case api.NewNoteLocationFolder:
		return a.settings.NewNoteFolder
	case api.NewNoteLocationCurrent:
		if v := a.Store.Workspace().ActiveTab().View; v.Type != workspace.ViewEmpty {
			return parentOf(v.Path)
		}
	}
	return ""
}

// newNotePath is where a new note goes: "Untitled.md", or "Untitled N.md"
// when taken.
func newNotePath(taken func(string) bool, folder string) string {
	for n := 0; ; n++ {
		name := "Untitled.md"
		if n > 0 {
			name = "Untitled " + strconv.Itoa(n) + ".md"
		}
		if p := join(folder, name); !taken(p) {
			return p
		}
	}
}

func (a *App) createNote(where string) {
	a.withSettings(func() { a.createNoteAt(newNotePath(a.hasFile, a.newNoteFolder()), where) })
}

func (a *App) createNoteIn(folder string) {
	p := newNotePath(a.hasFile, folder)
	a.createAndOpen(p, workspace.Here, func(w *workspace.Workspace) { w.SetFolderOpen(folder, true) })
}

func (a *App) createNoteAt(p, where string) { a.createAndOpen(p, where, nil) }

func (a *App) createAndOpen(p, where string, then func(*workspace.Workspace)) {
	id := a.vault.ID
	a.call(func(ctx context.Context, c *api.Client) error {
		_, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: id, Path: p})
		return err
	}, func(err error) {
		if a.vault == nil || a.vault.ID != id {
			return // the window moved to another vault meanwhile
		}
		if err != nil {
			a.notify("Cannot create " + p + ": " + err.Error())
			return
		}
		a.updateWorkspace(func(w *workspace.Workspace) {
			if then != nil {
				then(w)
			}
			w.Open(workspace.Note(p, workspace.ModeEdit), where)
		})
	})
}

// openOrCreate opens the note named target (as typed, with or without
// ".md"; folders it names are kept), creating it where new notes go when it
// does not exist.
func (a *App) openOrCreate(target, where string) {
	bare := strings.TrimLeft(strings.TrimSpace(target), "/")
	if bare == "" {
		return
	}
	file := bare
	if !strings.HasSuffix(strings.ToLower(bare), ".md") {
		file = bare + ".md"
	}
	a.withSettings(func() {
		p := file
		if folder := a.newNoteFolder(); !strings.Contains(file, "/") && folder != "" {
			p = join(folder, file)
		}
		if a.hasFile(p) {
			a.open(p, where)
			return
		}
		a.createNoteAt(p, where)
	})
}

func (a *App) searchFor(query string) {
	if a.search == nil {
		return
	}
	a.search.Ask(query)
	a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "search") })
}

// followNote follows a link the reading view drew, which notesd resolved.
func (a *App) followNote(p, sub string, newTab bool) {
	where := workspace.Here
	if newTab {
		where = workspace.NewTab
	}
	if a.hasFile(p) {
		a.open(p, where)
		a.goToSubpath(p, sub)
		return
	}
	a.openOrCreate(p, where)
}

// followLink follows a link in an editor: a vault file opens, a web link
// opens outside the app, and an unresolved note link creates the note, as
// Obsidian does: under its own folders when it names any, else where new
// notes go.
func (a *App) followLink(l editor.Link, from string, w editor.Where) {
	if l.URL != "" {
		a.report(a.openExternal(l.URL))
		return
	}
	if l.Target == "" { // a heading or block of the note itself
		a.goToSubpath(from, l.Subpath)
		return
	}
	where, id := whereOf(w), a.vault.ID
	var found []api.Resolution
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		found, err = c.Note().Resolve(ctx, &api.NoteResolveParams{VaultID: id, From: from, Targets: []string{l.Target}})
		return err
	}, func(err error) {
		if a.vault == nil || a.vault.ID != id {
			return // the window moved to another vault meanwhile
		}
		if err != nil {
			a.report(err)
			return
		}
		if len(found) > 0 && found[0].Path != nil {
			view := a.viewOf(*found[0].Path)
			if view.Type == workspace.ViewNote {
				view.Mode = workspace.ModeEdit
			}
			a.updateWorkspace(func(w *workspace.Workspace) { w.Open(view, where) })
			a.goToSubpath(*found[0].Path, l.Subpath)
			return
		}
		a.openOrCreate(l.Target, where)
	})
}

// preview is a link's page preview, read once.
type preview struct {
	text   string
	done   bool
	failed bool
}

// previewLines is how much of a note its page preview shows.
const previewLines = 12

// linkPreview draws the page preview of a link in the note at from.
func (a *App) linkPreview(from func() string) func(*ui.Context, editor.Link) {
	return func(c *ui.Context, l editor.Link) {
		key := from() + "\x00" + l.Target
		p := a.previews[key]
		if p == nil {
			p = &preview{}
			a.previews[key] = p
			a.loadPreview(p, from(), l.Target)
		}
		col := ui.Column(c).MaxWidth(360).Padding(frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch)
		col.Children(func() {
			switch {
			case !p.done:
				frostui.Loading(c, "Reading the note…")
			case p.failed:
				frostui.Muted(c, "This note does not exist yet.")
			default:
				frostui.Strong(c, nameOf(l.Target))
				markdown.View(c, p.text, &markdown.Options{OnLink: a.previewLink})
			}
		})
	}
}

// previewLink follows a link in a page preview, as the reading view does:
// a web link goes through the rules of what the app opens.
func (a *App) previewLink(href string) {
	links := a.readingLinks()
	if p, sub, tag, ok := render.NoteLink(href); ok {
		if tag != "" {
			links.Tag(tag)
		} else if p != "" {
			links.Note(p, sub, false)
		}
		return
	}
	links.URL(href)
}

func (a *App) loadPreview(p *preview, from, target string) {
	id := a.vault.ID
	var text string
	a.call(func(ctx context.Context, c *api.Client) error {
		found, err := c.Note().Resolve(ctx, &api.NoteResolveParams{VaultID: id, From: from, Targets: []string{target}})
		if err != nil {
			return err
		}
		if len(found) == 0 || found[0].Path == nil {
			return errNoNote
		}
		note, err := c.Note().RenderMarkdown(ctx, &api.NoteRenderMarkdownParams{VaultID: id, Path: *found[0].Path})
		if err != nil {
			return err
		}
		text = note.Markdown
		return nil
	}, func(err error) {
		p.done, p.failed = true, err != nil
		lines := strings.Split(strings.TrimSpace(editor.Body(text)), "\n")
		p.text = strings.Join(lines[:min(len(lines), previewLines)], "\n")
	})
}

// openExternal opens a web link outside the app, as system.WebURL allows
// it: every link the editor, the reading view and a page preview follow
// comes here.
func (a *App) openExternal(raw string) error {
	target, err := system.WebURL(raw)
	if err != nil {
		return err
	}
	if a.cfg.OpenURL == nil {
		return nil
	}
	return a.cfg.OpenURL(target)
}

func (a *App) openVaultFile(p string) error {
	if a.cfg.OpenFile == nil || a.vault == nil {
		return nil
	}
	return a.cfg.OpenFile(a.vault.Path, p)
}

func (a *App) openHistory(p string) {
	a.updateWorkspace(func(w *workspace.Workspace) {
		w.Open(workspace.View{Type: workspace.ViewHistory, Path: p}, workspace.NewTab)
	})
}

func (a *App) openMerge(p, text, title string) {
	a.held[p] = heldMerge{text, title}
	a.openMergeSide(p, &workspace.MergeSide{Kind: "held"})
}

func (a *App) openMergeSide(p string, other *workspace.MergeSide) {
	a.updateWorkspace(func(w *workspace.Workspace) {
		w.Open(workspace.View{Type: workspace.ViewMerge, Path: p, Other: other}, workspace.NewTab)
	})
}

// askLinks says whether a move rewrites links: the vault's setting, or the
// user's answer.
func (a *App) askLinks(name string, then func(bool)) {
	if a.settings != nil && a.settings.UpdateLinks {
		then(true)
		return
	}
	a.Dialogs.Ask(frostui.Confirm{Title: "Update links?", Body: `Update the links to "` + name + `" so they still lead to it?`,
		Action: "Update", Cancel: "Don't update", OnConfirm: func() { then(true) }, OnCancel: func() { then(false) }})
}

// moveTo moves a file or folder, its links updated as the vault's setting
// says.
func (a *App) moveTo(from, to string, folder bool) {
	if from == to {
		return
	}
	if !folder && a.hasFile(to) {
		a.notify(to + " exists already.")
		return
	}
	id := a.vault.ID
	a.askLinks(nameOf(from), func(links bool) {
		a.call(func(ctx context.Context, c *api.Client) error {
			var err error
			if folder {
				_, err = c.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: id, Path: from, To: to, UpdateLinks: &links})
			} else {
				_, err = c.Note().Move(ctx, &api.NoteMoveParams{VaultID: id, Path: from, To: to, UpdateLinks: &links})
			}
			return err
		}, func(err error) {
			if err != nil {
				a.notify("Cannot move " + from + ": " + err.Error())
				return
			}
			if folder {
				a.Catalog.Reload()
			}
		})
	})
}

// renameFile renames a file or folder where it is: a file keeps its
// extension.
func (a *App) renameFile(p, name string, folder bool) {
	ext := ""
	if base := path.Base(p); !folder {
		if dot := strings.LastIndexByte(base, '.'); dot > 0 {
			ext = base[dot:]
		}
	}
	a.moveTo(p, join(parentOf(p), name+ext), folder)
}

func (a *App) moveInto(p, folder string, isFolder bool) {
	if isFolder && (folder == p || strings.HasPrefix(folder, p+"/")) {
		return
	}
	a.moveTo(p, join(folder, path.Base(p)), isFolder)
}

func (a *App) chooseFolder(p string, folder bool) {
	a.moving = &moveTarget{p, folder}
	a.Shell.Palette.ShowSource(sourceFolders)
}

func (a *App) confirmDelete(name, body string, run func()) {
	if !a.Prefs.ConfirmDelete {
		run()
		return
	}
	a.Dialogs.Ask(frostui.Confirm{Title: `Delete "` + name + `"?`, Body: body, Action: "Delete", Destructive: true, OnConfirm: run})
}

// deleteFile deletes a file or folder with its base, asking first while
// the Confirm file deletion preference is on; notesd keeps its versions.
func (a *App) deleteFile(p string, folder bool) {
	if folder {
		a.deleteFolder(p)
		return
	}
	hash := a.fileHash(p)
	if hash == "" {
		return
	}
	name, id := nameOf(p), a.vault.ID
	a.confirmDelete(name, "You can restore it from its version history.", func() {
		a.saveUnder(id, p, name, func(bases map[string]string, thaw func()) {
			if b, ok := bases[p]; ok {
				hash = b
			}
			a.call(func(ctx context.Context, c *api.Client) error {
				return c.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: id, Path: p, Base: hash})
			}, func(err error) {
				thaw()
				if err != nil {
					a.notify("Cannot delete " + name + ": " + err.Error())
					return
				}
				a.actionToasts = append(a.actionToasts, frostui.ActionToast{Title: "Deleted " + name + ".", Action: "Undo", OnAction: func() { a.restoreFile(id, p) }})
			})
		})
	})
}

// saveUnder saves the open notes at or inside p, then gives their bases: a
// delete comes after, or the save's unsaved text would make the note again.
// What was typed stays in the deleted note's version history. The notes take
// no edits until next calls thaw, once the delete has answered; a note that
// could not be saved stops the delete, which next then never sees.
func (a *App) saveUnder(vault int64, p, name string, next func(bases map[string]string, thaw func())) {
	docs := a.Session.Under(vault, p)
	a.commitDrafts()
	thaws := make([]func(), 0, len(docs))
	for _, d := range docs {
		thaws = append(thaws, d.Freeze("Deleting this note."))
	}
	thaw := func() {
		for _, fn := range thaws {
			fn()
		}
	}
	left := len(docs) + 1
	done := func() {
		if left--; left > 0 {
			return
		}
		if slices.ContainsFunc(docs, (*notes.Document).Unsaved) {
			thaw()
			a.notify("Cannot delete " + name + ": some of its text could not be saved.")
			return
		}
		bases := make(map[string]string, len(docs))
		for _, d := range docs {
			bases[d.Path()] = d.Base()
		}
		next(bases, thaw)
	}
	for _, d := range docs {
		d.Flush(done)
	}
	done()
}

// restoreFile brings a deleted file back from its newest version.
func (a *App) restoreFile(vault int64, p string) {
	a.call(func(ctx context.Context, c *api.Client) error {
		versions, err := c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: vault, Path: p})
		if err != nil {
			return err
		}
		i := slices.IndexFunc(versions, func(v api.Version) bool { return !v.Deleted })
		if i < 0 {
			return errors.New("it has no version to restore")
		}
		_, err = c.Note().Revert(ctx, &api.NoteRevertParams{VaultID: vault, Path: p, Version: versions[i].ID})
		return err
	}, func(err error) {
		if err != nil {
			a.notify("Cannot restore " + nameOf(p) + ": " + err.Error())
		}
	})
}

func (a *App) deleteFolder(p string) {
	var files []api.FileBase
	for _, f := range a.Catalog.Files() {
		if strings.HasPrefix(f.Path, p+"/") {
			files = append(files, api.FileBase{Path: f.Path, Base: f.Hash})
		}
	}
	name, id := path.Base(p), a.vault.ID
	a.confirmDelete(name, "Its "+strconv.Itoa(len(files))+" files can be restored from their version history.", func() {
		a.saveUnder(id, p, name, func(bases map[string]string, thaw func()) {
			for i := range files {
				if b, ok := bases[files[i].Path]; ok {
					files[i].Base = b
				}
			}
			var res *api.FolderDeleteResult
			a.call(func(ctx context.Context, c *api.Client) (err error) {
				res, err = c.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: id, Path: p, Files: files})
				return err
			}, func(err error) {
				thaw()
				if err != nil {
					a.notify("Cannot delete " + name + ": " + err.Error())
					return
				}
				a.Catalog.Reload()
				if len(res.Kept) > 0 {
					a.notify("Deleted " + name + ", but kept " + strconv.Itoa(len(res.Kept)) + " changed files.")
				} else {
					a.notify("Deleted " + name + ".")
				}
			})
		})
	})
}

func (a *App) newFolder(parent string) {
	p := join(parent, "Untitled")
	for n := 1; slices.Contains(a.Catalog.Folders(), p); n++ {
		p = join(parent, "Untitled "+strconv.Itoa(n))
	}
	id := a.vault.ID
	a.call(func(ctx context.Context, c *api.Client) error {
		return c.Note().Mkdir(ctx, &api.NoteMkdirParams{VaultID: id, Path: p})
	}, func(err error) {
		if err != nil {
			a.notify("Cannot create the folder: " + err.Error())
			return
		}
		a.Catalog.Reload()
		a.files.Rename(p)
	})
}

func (a *App) fileActions() panes.FileActions {
	return panes.FileActions{
		NewNote: a.createNoteIn, NewFolder: a.newFolder, Rename: a.renameFile, Move: a.moveInto, MoveTo: a.chooseFolder, Delete: a.deleteFile,
		CopyPath: func(p string) {
			if a.cfg.Copy != nil {
				a.cfg.Copy(p)
				a.notify("Copied the path.")
			}
		},
		OpenDefault: func(p string) { a.report(a.openVaultFile(p)) },
		History:     a.openHistory,
	}
}

// insertTemplate inserts a template at the active editor's cursor, as
// Obsidian's Insert template does: notesd expands it for the note's title
// and now; its properties the note lacks join the note's, and its body
// replaces the selection.
func (a *App) insertTemplate(template string) {
	e := a.activeEditor()
	if e == nil {
		return
	}
	d := e.Binding().Document()
	title, id := nameOf(d.Path()), a.vault.ID
	var content string
	a.call(func(ctx context.Context, c *api.Client) error {
		t, err := c.Note().Template(ctx, &api.NoteTemplateParams{VaultID: id, Template: template, Title: &title})
		if err == nil {
			content = t.Content
		}
		return err
	}, func(err error) {
		if err != nil {
			a.notify("Cannot insert the template: " + err.Error())
			return
		}
		if reason := d.ReadOnly(); reason != "" {
			a.notify(reason)
			return
		}
		a.applyTemplate(e, content)
	})
}

func (a *App) applyTemplate(e *editor.NoteEditor, content string) {
	e.Binding().Sync()
	d := e.Binding().Document()
	if fm := editor.ReadFrontmatter(content); fm.Exists && len(fm.Properties) > 0 {
		mine := editor.ReadFrontmatter(d.Text())
		add := map[string]any{}
		for _, p := range fm.Properties {
			if !slices.ContainsFunc(mine.Properties, func(q editor.Property) bool { return q.Key == p.Key }) {
				add[p.Key] = p.Value
			}
		}
		if len(add) > 0 {
			next, err := editor.SetProperties(d.Text(), add)
			if err != nil {
				a.notify("Cannot add the template's properties: " + err.Error())
			} else {
				d.Edit(notes.Diff(d.Text(), next))
			}
		}
	}
	e.State.Insert(editor.Body(content))
}

func (a *App) addFileProperty() {
	tc := a.activeContent()
	if tc == nil || tc.ed == nil {
		return
	}
	e := tc.ed.Editor
	e.Binding().Sync()
	d := e.Binding().Document()
	if reason := d.ReadOnly(); reason != "" {
		a.notify(reason)
		return
	}
	if !editor.ReadFrontmatter(d.Text()).Exists {
		at := editor.FrontmatterStart(d.Text())
		cs, err := notes.ChangesOf(len(d.Text()), notes.Change{From: at, To: at, Insert: "---\n---\n"})
		if err != nil {
			a.report(err)
			return
		}
		d.Edit(cs)
	}
	if a.Prefs.Properties == "visible" {
		tc.ed.Properties.Add()
		return
	}
	e.State.GoTo(code.Pos{Line: editor.FrontmatterLastLine(d.Text())})
	e.State.Insert("\n")
}

func (a *App) openDaily() {
	id := a.vault.ID
	var day *api.DailyNote
	a.call(func(ctx context.Context, c *api.Client) (err error) {
		day, err = c.Note().Daily(ctx, &api.NoteDailyParams{VaultID: id})
		return err
	}, func(err error) {
		if err != nil {
			a.notify("Cannot open today's daily note: " + err.Error())
			return
		}
		a.open(day.Note.Path, workspace.Here)
	})
}

func (a *App) setZoom(z float64) {
	a.Prefs.Zoom = z
	a.savePrefs()
}

func (a *App) renameActive() {
	v := a.Store.Workspace().ActiveTab().View
	if v.Type != workspace.ViewNote && v.Type != workspace.ViewFile {
		return
	}
	a.files.Reveal(v.Path)
	a.files.Rename(v.Path)
	a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "files") })
}

func (a *App) searchAll() {
	q := a.search.Query
	if e := a.activeEditor(); e != nil {
		if s := strings.TrimSpace(e.State.SelectedText()); s != "" {
			q = s
			if strings.ContainsAny(s, " \t\n") {
				q = `"` + s + `"`
			}
		}
	}
	a.searchFor(q)
}

func (a *App) followCaret(w editor.Where) {
	e := a.activeEditor()
	if e == nil {
		return
	}
	e.Binding().Sync()
	if l, ok := editor.LinkAt(e.Binding().Document().Text(), e.State.Cursor()); ok {
		a.followLink(l, e.Binding().Document().Path(), w)
	}
}

func (a *App) insertTemplateChooser() {
	folder := ""
	if a.settings != nil {
		folder = strings.Trim(a.settings.TemplateFolder, "/")
	}
	if folder == "" {
		a.notify("Choose a template folder in Settings, under Templates.")
		return
	}
	if len(a.templates(folder)) == 0 {
		a.notify("There are no templates in " + folder + ".")
		return
	}
	a.Shell.Palette.ShowSource(sourceTemplates)
}

func (a *App) templates(folder string) []string {
	var out []string
	for _, f := range a.Catalog.Files() {
		if strings.HasPrefix(f.Path, folder+"/") && strings.HasSuffix(f.Path, ".md") {
			out = append(out, f.Path)
		}
	}
	return out
}

func (a *App) attachFile() {
	e := a.activeEditor()
	if e == nil || a.cfg.PickFiles == nil {
		return
	}
	if paths := a.cfg.PickFiles(); len(paths) > 0 {
		e.AttachPaths(paths)
	}
}

// commandActions gives each command of the table what it does.
func (a *App) commandActions() map[string]func() {
	up := func(op func(*workspace.Workspace)) func() { return func() { a.updateWorkspace(op) } }
	onActive := func(op func(*workspace.Workspace, string)) func() {
		return up(func(w *workspace.Workspace) { op(w, w.ActiveTab().ID) })
	}
	format := func(cmd func(string, []editor.Sel) editor.Edit) func() {
		return func() {
			if e := a.activeEditor(); e != nil {
				e.Run(cmd)
			}
		}
	}
	mark := func(m string) func() {
		return format(func(t string, s []editor.Sel) editor.Edit { return editor.ToggleMark(t, s, m) })
	}
	pair := func(before, after string) func() {
		return format(func(t string, s []editor.Sel) editor.Edit { return editor.InsertPair(t, s, before, after) })
	}
	run := func(cmd code.Command) func() {
		return func() {
			if e := a.activeEditor(); e != nil {
				e.State.Run(cmd)
			}
		}
	}
	m := map[string]func(){
		"switcher:open":                      func() { a.Shell.Palette.ShowSource(sourceNotes) },
		"command-palette:open":               func() { a.Shell.Palette.ShowSource(frostui.PaletteCommands) },
		"file-explorer:new-file":             func() { a.createNote(workspace.Here) },
		"file-explorer:new-file-in-new-pane": func() { a.createNote(workspace.ToRight) },
		"daily-notes":                        a.openDaily,
		"insert-template":                    a.insertTemplateChooser,
		"workspace:new-tab":                  up(func(w *workspace.Workspace) { w.Open(workspace.Empty(), workspace.NewTab) }),
		"workspace:close":                    onActive((*workspace.Workspace).Close),
		"workspace:close-others":             onActive((*workspace.Workspace).CloseOthers),
		"workspace:close-window":             func() { a.closeWindow() },
		"workspace:undo-close-pane":          up((*workspace.Workspace).ReopenClosed),
		"workspace:next-tab":                 up(func(w *workspace.Workspace) { w.Cycle(1) }),
		"workspace:previous-tab":             up(func(w *workspace.Workspace) { w.Cycle(-1) }),
		"workspace:goto-last-tab":            up((*workspace.Workspace).GotoLastTab),
		"workspace:split-vertical":           onActive(func(w *workspace.Workspace, id string) { w.Split(id, workspace.ToRight) }),
		"workspace:split-horizontal":         onActive(func(w *workspace.Workspace, id string) { w.Split(id, workspace.Below) }),
		"workspace:toggle-pin":               onActive(func(w *workspace.Workspace, id string) { w.Pin(id, !w.ActiveTab().Pinned) }),
		"workspace:edit-file-title":          a.renameActive,
		"app:go-back":                        up((*workspace.Workspace).Back),
		"app:go-forward":                     up((*workspace.Workspace).Forward),
		"app:toggle-left-sidebar":            up(func(w *workspace.Workspace) { w.ToggleSidebar(workspace.SideLeft) }),
		"app:toggle-right-sidebar":           up(func(w *workspace.Workspace) { w.ToggleSidebar(workspace.SideRight) }),
		"app:open-settings":                  func() { a.sets.show("editor") },
		"app:open-vault":                     func() { a.vm.open = true },
		"app:open-help":                      func() { a.sets.show("hotkeys") },
		"frostnotes:reset-workspace-layout":  a.Store.Reset,
		"sync:view-version-history": func() {
			if p := a.activeFile(); p != "" {
				a.openHistory(p)
			}
		},
		"markdown:toggle-preview":        a.toggleMode,
		"markdown:add-metadata-property": a.addFileProperty,
		"global-search:open":             a.searchAll,
		"editor:open-search":             run(code.Find),
		"editor:open-search-replace":     run(code.FindReplace),
		"editor:toggle-bold":             mark("**"),
		"editor:toggle-italics":          mark("*"),
		"editor:toggle-strikethrough":    mark("~~"),
		"editor:toggle-highlight":        mark("=="),
		"editor:toggle-code":             mark("`"),
		"editor:insert-link":             format(editor.InsertLink),
		"editor:toggle-checklist-status": format(editor.ToggleChecklist),
		"editor:toggle-comments":         format(editor.ToggleComment),
		"editor:delete-paragraph":        format(editor.DeleteParagraph),
		"editor:set-heading":             func() { a.Shell.Palette.ShowSource(sourceHeadings) },
		"editor:insert-wikilink":         pair("[[", "]]"),
		"editor:insert-embed":            pair("![[", "]]"),
		"editor:insert-tag":              pair("#", ""),
		"editor:toggle-blockquote":       format(editor.ToggleBlockquote),
		"editor:toggle-bullet-list":      format(editor.ToggleBulletList),
		"editor:toggle-numbered-list":    format(editor.ToggleNumberedList),
		"editor:indent-list":             run(code.Indent),
		"editor:unindent-list":           run(code.Outdent),
		"editor:attach-file":             a.attachFile,
		"editor:follow-link":             func() { a.followCaret(editor.WhereHere) },
		"editor:open-link-in-new-leaf":   func() { a.followCaret(editor.WhereTab) },
		"editor:open-link-in-new-split":  func() { a.followCaret(editor.WhereRight) },
		"editor:open-link-in-new-window": func() { a.followCaret(editor.WhereWindow) },
		"window:zoom-in":                 func() { a.setZoom(prefs.ZoomStep(a.Prefs.Zoom, 1)) },
		"window:zoom-out":                func() { a.setZoom(prefs.ZoomStep(a.Prefs.Zoom, -1)) },
		"window:reset-zoom":              func() { a.setZoom(1) },
		"graph:open":                     func() { a.notify("Frostnotes has no graph view yet.") },
		"editor:save-file": func() {
			if e := a.activeEditor(); e != nil {
				e.Binding().Sync()
				e.Binding().Document().Save()
			}
		},
		"editor:undo": func() {
			if e := a.activeEditor(); e != nil {
				e.Undo()
			} else if tc := a.activeContent(); tc != nil && tc.readBuffer != nil {
				tc.readBuffer.Undo()
			}
		},
		"editor:redo": func() {
			if e := a.activeEditor(); e != nil {
				e.Redo()
			} else if tc := a.activeContent(); tc != nil && tc.readBuffer != nil {
				tc.readBuffer.Redo()
			}
		},
	}
	for n := 1; n <= 8; n++ {
		m["workspace:goto-tab-"+strconv.Itoa(n)] = up(func(w *workspace.Workspace) { w.GotoTab(n) })
	}
	for level := 0; level <= 6; level++ {
		m["editor:set-heading-"+strconv.Itoa(level)] = format(func(t string, s []editor.Sel) editor.Edit { return editor.SetHeading(t, s, level) })
	}
	return m
}

// applies says whether a command does anything now.
func (a *App) applies(id string) bool {
	switch id {
	case "command-palette:open", "app:open-settings", "app:open-vault", "app:open-help", "window:zoom-in", "window:zoom-out", "window:reset-zoom", "workspace:close-window":
		return true
	}
	if a.vault == nil {
		return false
	}
	w := a.Store.Workspace()
	tab := w.ActiveTab()
	switch id {
	case "workspace:close-others":
		return len(w.ActiveGroup().Tabs) > 1
	case "workspace:undo-close-pane":
		return len(w.Closed) > 0
	case "app:go-back":
		return len(tab.Back) > 0
	case "app:go-forward":
		return len(tab.Forward) > 0
	case "markdown:toggle-preview":
		return tab.View.Type == workspace.ViewNote
	case "sync:view-version-history", "workspace:edit-file-title":
		return tab.View.Type == workspace.ViewNote || tab.View.Type == workspace.ViewFile
	case "editor:undo", "editor:redo":
		tc := a.activeContent()
		return a.activeEditor() != nil || tc != nil && tc.readBuffer != nil
	case "insert-template", "markdown:add-metadata-property":
		return a.activeEditor() != nil
	case "editor:follow-link", "editor:open-link-in-new-leaf", "editor:open-link-in-new-split", "editor:open-link-in-new-window":
		e := a.activeEditor()
		if e == nil {
			return false
		}
		_, ok := editor.LinkAt(e.Binding().Document().Text(), e.State.Cursor())
		return ok
	}
	if strings.HasPrefix(id, "editor:") {
		return a.activeEditor() != nil
	}
	return true
}

// run runs a command when it applies.
func (a *App) run(id string) {
	if f := a.actions[id]; f != nil && a.applies(id) {
		f()
	}
}

// closeWindow saves the notes, then closes.
func (a *App) closeWindow() {
	if a.cfg.Close != nil {
		a.cfg.Close()
	}
}

// why says why a command does not apply.
func (a *App) why(id string) string {
	switch {
	case a.vault == nil:
		return "No vault is open"
	case strings.HasPrefix(id, "editor:") || id == "insert-template" || id == "markdown:add-metadata-property":
		return "No note is being edited"
	}
	return ""
}

// commandTable makes frostui's table of the app's commands. The keys the
// palette's own command binds stay with it: Ctrl K and Ctrl Shift P open
// the commands and the notes; Ctrl P opens the commands alone.
func (a *App) commandTable() *frostui.CommandTable {
	t := frostui.NewCommandTable()
	t.Editing = func(*ui.Context) bool { return a.inEditor() }
	palette := frostui.PaletteBindings()
	for _, s := range a.table {
		cmd := frostui.Command{ID: s.ID, Title: s.Title, Group: s.Group, Icon: s.Icon, Unlisted: s.Unlisted,
			Run: func() {
				a.State.RanCommand(s.ID)
				a.saveState()
				a.actions[s.ID]()
			},
			Enabled: func() (bool, string) {
				if a.applies(s.ID) {
					return true, ""
				}
				return false, a.why(s.ID)
			}}
		for _, k := range s.Keys {
			if len(cmd.Keys) == 0 {
				cmd.Keys = keyCaps(k)
			}
			if k.Scope == commands.InEditor {
				continue
			}
			if slices.ContainsFunc(palette, func(b frostui.KeyBinding) bool { return b.Mods == k.Mods && b.Key == k.Key }) {
				continue
			}
			scope := frostui.OutsideEditors
			if k.Scope == commands.Everywhere {
				scope = frostui.Everywhere
			}
			cmd.Bindings = append(cmd.Bindings, frostui.KeyBinding{Mods: k.Mods, Key: k.Key, Scope: scope})
		}
		t.Add(cmd)
	}
	return t
}

func keyCaps(k commands.Key) []string {
	return frostui.KeyBinding{Mods: k.Mods, Key: k.Key}.Caps()
}

// recentCommands head the palette's commands while nothing is typed.
func (a *App) recentCommands() []frostui.Command {
	if strings.TrimSpace(a.Shell.Palette.Query) != "" {
		return nil
	}
	var rows []frostui.Command
	for _, id := range a.State.RecentCommands {
		cmd, ok := a.cmds.Command(id)
		if !ok || cmd.Disabled || cmd.Unlisted {
			continue
		}
		cmd.ID, cmd.Group, cmd.Bindings = "recent:"+id, "Recent", nil
		rows = append(rows, cmd)
	}
	return rows
}

func (a *App) paletteSources() []frostui.PaletteSource {
	return []frostui.PaletteSource{
		{ID: sourceNotes, Title: "Notes", Placeholder: "Find a note", Limit: 8, Query: a.queryNotes},
		{ID: sourceFolders, Title: "Folders", Placeholder: "Move to a folder", Query: a.queryFolders},
		{ID: sourceTemplates, Title: "Templates", Placeholder: "Insert a template", Query: a.queryTemplates},
		{ID: sourceHeadings, Title: "Headings", Placeholder: "Set the heading", Query: a.queryHeadings},
	}
}

func whereOfMods(mods ui.Modifiers) string {
	return workspace.WhereFor(mods&ui.Cmd != 0, mods&ui.Alt != 0, 0)
}

// queryNotes is the quick switcher: recent files for an empty query, else
// the best matches, and a row that creates what is typed. Shift + Enter
// creates the note named by the query.
func (a *App) queryNotes(q *frostui.PaletteQuery) {
	if a.Catalog == nil {
		q.Reply(nil)
		return
	}
	var rows []frostui.PaletteItem
	add := func(p, alias, group string) {
		title := strings.TrimSuffix(path.Base(p), ".md")
		detail := parentOf(p)
		if alias != "" {
			detail = strings.TrimSuffix(p, ".md")
			title = alias
		}
		rows = append(rows, frostui.PaletteItem{ID: p + "\x00" + alias, Title: title, Detail: detail, Group: group, Icon: "text-x-generic",
			RunWith: func(mods ui.Modifiers) {
				if mods&ui.Shift != 0 && q.Text != "" {
					a.openOrCreate(q.Text, workspace.Here)
					return
				}
				a.open(p, whereOfMods(mods))
			}})
	}
	if q.Text == "" {
		for _, p := range a.Store.Workspace().Recent {
			if a.hasFile(p) {
				add(p, "", "Recent files")
			}
		}
		q.Reply(rows)
		return
	}
	for _, m := range a.Catalog.Search(q.Text, 50, false) {
		add(m.Path, m.Alias, "")
	}
	if q.Alone {
		text := q.Text
		rows = append(rows, frostui.PaletteItem{ID: "\x00create", Title: "Create " + text, Icon: "document-new",
			RunWith: func(mods ui.Modifiers) { a.openOrCreate(text, whereOfMods(mods)) }})
	}
	q.Reply(rows)
}

// reply gives a chooser's rows, shown only on its own.
func reply(q *frostui.PaletteQuery, rows []frostui.PaletteItem) {
	if !q.Alone {
		q.Reply(nil)
		return
	}
	var kept []frostui.PaletteItem
	for _, r := range rows {
		if _, ok := frostui.Match(q.Text, r.Title); ok {
			kept = append(kept, r)
		}
	}
	q.Reply(kept)
}

func (a *App) queryFolders(q *frostui.PaletteQuery) {
	if a.Catalog == nil || a.moving == nil {
		q.Reply(nil)
		return
	}
	to := *a.moving
	rows := []frostui.PaletteItem{{ID: "/", Title: "/", Icon: "folder", Run: func() { a.moveInto(to.path, "", to.folder) }}}
	for _, f := range a.Catalog.Folders() {
		rows = append(rows, frostui.PaletteItem{ID: f, Title: f, Icon: "folder", Run: func() { a.moveInto(to.path, f, to.folder) }})
	}
	reply(q, rows)
}

func (a *App) queryTemplates(q *frostui.PaletteQuery) {
	if a.Catalog == nil || a.settings == nil {
		q.Reply(nil)
		return
	}
	folder := strings.Trim(a.settings.TemplateFolder, "/")
	var rows []frostui.PaletteItem
	for _, p := range a.templates(folder) {
		rows = append(rows, frostui.PaletteItem{ID: p, Title: strings.TrimSuffix(strings.TrimPrefix(p, folder+"/"), ".md"), Icon: "text-x-generic", Run: func() { a.insertTemplate(p) }})
	}
	reply(q, rows)
}

func (a *App) queryHeadings(q *frostui.PaletteQuery) {
	var rows []frostui.PaletteItem
	for level := 1; level <= 6; level++ {
		n := strconv.Itoa(level)
		rows = append(rows, frostui.PaletteItem{ID: n, Title: "Heading " + n, Run: func() { a.run("editor:set-heading-" + n) }})
	}
	reply(q, append(rows, frostui.PaletteItem{ID: "0", Title: "No heading", Run: func() { a.run("editor:set-heading-0") }}))
}

func (a *App) showShortcuts() {
	var entries []frostui.ShortcutEntry
	for _, s := range a.table {
		if s.Unlisted || len(s.Keys) == 0 {
			continue
		}
		entries = append(entries, frostui.ShortcutEntry{Group: s.Group, Title: s.Title, Keys: keyCaps(s.Keys[0])})
	}
	slices.SortStableFunc(entries, func(x, y frostui.ShortcutEntry) int { return strings.Compare(x.Group, y.Group) })
	a.Shell.Info.Show("Keyboard shortcuts", func(c *ui.Context) {
		ui.Scroll(c).MaxHeight(420).Children(func() {
			ui.Column(c).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Children(func() { frostui.ShortcutsBody(entries)(c) })
		})
	})
}

func (a *App) showAbout() {
	a.Shell.Info.Show("About Frostnotes", frostui.AboutBody(frostui.About{
		Name: "Frostnotes", Version: a.cfg.Version, Summary: "Plain Markdown notes that sync through your own hub.",
		Logo: "org.frostyard.Frostnotes-symbolic", Website: "https://github.com/frostyard/frostnotes", Issues: "https://github.com/frostyard/frostnotes/issues",
		License: "See the repository",
	}))
}

// showConflicts lists the flagged files: each opens beside its latest
// conflict revision.
func (a *App) showConflicts() {
	if a.vault == nil {
		return
	}
	a.Shell.Info.Show("Conflicts", func(c *ui.Context) {
		ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
			for _, f := range a.conflict {
				ui.Row(c.Key(f.Path)).Gap(frostui.SpaceM).AlignItems(ui.Center).Children(func() {
					frostui.Body(c, f.Path).Grow(1).SingleLine()
					if ui.Button(c, "Compare").Clicked() {
						a.Shell.Info.Open = false
						if n := len(f.Revisions); n > 0 {
							a.openMergeSide(f.Path, &workspace.MergeSide{Kind: "revision", Seq: f.Revisions[n-1].Seq})
						} else {
							a.open(f.Path, workspace.NewTab)
						}
					}
				})
			}
		})
	})
}

func hostOf(raw string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	host, _, _ := strings.Cut(s, "/")
	return host
}

func syncText(s *api.SyncStatus) string {
	switch s.State {
	case api.SyncStateIdle:
		if s.SyncedAt != nil {
			return "Synced " + s.SyncedAt.Local().Format("15:04")
		}
		return "Synced"
	case api.SyncStateSyncing:
		return "Syncing…"
	case api.SyncStateOffline:
		if s.Error != nil {
			return "Offline: " + *s.Error
		}
		return "Offline: the hub cannot be reached"
	case api.SyncStateRevoked:
		if s.Error != nil {
			return *s.Error
		}
		return "This device was removed from the hub."
	case api.SyncStateReset:
		if s.Error != nil {
			return *s.Error
		}
		return "The hub was reset: join it again."
	}
	return string(s.State)
}
