package window

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/history"
	"github.com/frostyard/frostnotes/internal/desktop/media"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/panes"
	"github.com/frostyard/frostnotes/internal/desktop/reading"
	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

var (
	errOffline = errors.New("not connected to notesd")
	errNoNote  = errors.New("no such note")
)

// tabContent is what a tab's view draws with, kept while the tab shows
// the same thing.
type tabContent struct {
	view       workspace.View
	doc        *notes.Document
	ed         *editor.NoteView
	read       *reading.View
	readBuffer *editor.Binding
	hist       *history.View
	merg       *history.Merge
	held       string // a merge's other text
	img        ui.ImageSource
	fileRead   bool
	err        string
	scroll     ui.ScrollState
	restored   bool
}

func sameContent(a, b workspace.View) bool {
	a.Scroll, a.Cursor, b.Scroll, b.Cursor = 0, 0, 0, 0
	return a.Same(b)
}

// content is the tab's content for its view, made when it changes.
func (a *App) content(tab *workspace.Tab) *tabContent {
	if tc := a.tabs[tab.ID]; tc != nil && sameContent(tc.view, tab.View) {
		tc.view = tab.View
		if tc.view.Type == workspace.ViewFile && !tc.fileRead {
			a.loadFile(tc)
		}
		return tc
	}
	if tc := a.tabs[tab.ID]; tc != nil {
		a.release(tc)
	}
	tc := a.make(tab)
	a.tabs[tab.ID] = tc
	return tc
}

func (a *App) make(tab *workspace.Tab) *tabContent {
	v := tab.View
	tc := &tabContent{view: v}
	tc.scroll.Y = float32(v.Scroll)
	switch v.Type {
	case workspace.ViewNote:
		tc.doc = a.Session.Open(a.vault.ID, v.Path)
		if v.Mode == workspace.ModeRead {
			tc.readBuffer = a.bindingFor(tc.doc)
			tc.read = reading.New(a.cfg.Env, a.readingAPI, tc.doc, a.cfg.Blobs)
			tc.read.RemoteImages = a.Prefs.LoadRemoteImages
			tc.read.Links = a.readingLinks()
			break
		}
		tc.ed = editor.NewNoteView(a.noteEditor(tc.doc, v))
		tc.ed.Rename = func(title string) { a.renameFile(v.Path, title, false) }
		tc.ed.Properties.Types = a.properties.Types
		tc.ed.Properties.Suggestions = a.tagNames
	case workspace.ViewHistory:
		tc.doc = a.Session.Open(a.vault.ID, v.Path)
		tc.hist = history.New(a.cfg.Env, a.historyAPI, a.hubAPI, tc.doc)
		tc.hist.Compare = func(text, title string) { a.openMerge(v.Path, text, title) }
	case workspace.ViewMerge:
		tc.doc = a.Session.Open(a.vault.ID, v.Path)
		a.loadMerge(tc, tab.ID)
	case workspace.ViewFile:
		a.loadFile(tc)
	}
	return tc
}

// readingLinks are where a rendered note's links lead.
func (a *App) readingLinks() reading.Links {
	return reading.Links{
		Note: func(p, sub string, newTab bool) { a.followNote(p, sub, newTab) },
		Tag:  func(t string) { a.searchFor("tag:#" + t) },
		URL:  func(u string) { a.report(a.openExternal(u)) },
	}
}

func (a *App) readingAPI() reading.Renderer {
	if c := a.Session.Client(); c != nil {
		n := c.Note()
		return &n
	}
	return nil
}

func (a *App) historyAPI() history.API {
	if c := a.Session.Client(); c != nil {
		n := c.Note()
		return &n
	}
	return nil
}

func (a *App) hubAPI() history.HubAPI {
	if c := a.Session.Client(); c != nil && a.syncInfo != nil {
		s := c.Sync()
		return &s
	}
	return nil
}

func (a *App) bindingFor(d *notes.Document) *editor.Binding {
	b := a.bindings[d]
	if b == nil {
		b = &binding{b: editor.Bind(d)}
		a.bindings[d] = b
	}
	b.refs++
	return b.b
}

func (a *App) editorPrefs() editor.Prefs {
	p := a.Prefs
	return editor.Prefs{SmartLists: p.SmartLists, AutoPairMarkdown: p.AutoPairMarkdown, AutoPairBrackets: p.AutoPairBrackets,
		LineNumbers: p.LineNumbers, FoldHeadings: p.FoldHeadings, FoldIndent: p.FoldIndent, IndentWithTabs: p.IndentWithTabs, TabSize: p.TabSize,
		IndentGuides: p.IndentGuides, FontSize: p.FontSize, Live: p.EditingMode == "live"}
}

func (a *App) noteEditor(d *notes.Document, v workspace.View) *editor.NoteEditor {
	e := editor.NewNoteEditor(a.bindingFor(d))
	e.Prefs = a.editorPrefs()
	src := a.Catalog.Sources(d.Path, a.openText)
	e.Sources, e.Uploads = src, src
	e.Clipboard, e.OnError = a.cfg.Clipboard, a.report
	e.Follow = func(l editor.Link, where editor.Where) { a.followLink(l, d.Path(), where) }
	e.FollowTag = func(tag string) { a.searchFor("tag:#" + tag) }
	e.Preview = a.linkPreview(d.Path)
	e.Hold = func() func() {
		held := a.Session.Open(d.Vault(), d.Path())
		return func() { a.Session.Release(held) }
	}
	e.Rendered = reading.NewBlocks(a.cfg.Env, a.readingAPI, d, a.cfg.Blobs)
	e.Rendered.RemoteImages = a.Prefs.LoadRemoteImages
	e.Rendered.Links = a.readingLinks()

	return e
}

// openText is a note's text when a tab has it open.
func (a *App) openText(p string) (string, bool) {
	for _, tc := range a.tabs {
		if tc.doc != nil && tc.doc.Path() == p && tc.doc.Loaded() {
			return tc.doc.Text(), true
		}
	}
	return "", false
}

func (a *App) release(tc *tabContent) {
	if tc.ed != nil {
		tc.ed.Properties.Commit() // a field with the focus has not committed
	}
	if tc.ed != nil || tc.merg != nil || tc.readBuffer != nil {
		if b := a.bindings[tc.doc]; b != nil {
			if b.refs--; b.refs == 0 {
				b.b.Close()
				delete(a.bindings, tc.doc)
			}
		}
	}
	if tc.doc != nil {
		a.Session.Release(tc.doc)
	}
}

func (a *App) releaseAllTabs() {
	for id, tc := range a.tabs {
		a.release(tc)
		delete(a.tabs, id)
	}
}

// sweep releases the contents of tabs that are gone.
func (a *App) sweep() {
	live := map[string]bool{}
	for _, g := range workspace.Groups(a.Store.Workspace().Root) {
		for _, t := range g.Tabs {
			live[t.ID] = true
		}
	}
	for id, tc := range a.tabs {
		if !live[id] {
			a.release(tc)
			delete(a.tabs, id)
		}
	}
}

func (a *App) loadMerge(tc *tabContent, tabID string) {
	other := tc.view.Other
	if other == nil {
		tc.err = "Nothing to compare with."
		return
	}
	done := func(text string, err error) {
		if err != nil {
			tc.err = err.Error()
			return
		}
		tc.held = text
		tc.merg = history.NewMerge(a.bindingFor(tc.doc), text, "Comparing with "+mergeName(other))
		tc.merg.Done = func() { a.mergeDone(tc, tabID) }
	}
	if other.Kind == "held" {
		h, ok := a.held[tc.view.Path]
		if !ok {
			tc.err = "The text to compare with is gone."
			return
		}
		delete(a.held, tc.view.Path)
		done(h.text, nil)
		return
	}
	var text string
	id := a.vault.ID
	a.call(func(ctx context.Context, c *api.Client) error {
		if other.Kind == "revision" {
			r, err := c.Sync().ReadRevision(ctx, &api.SyncReadRevisionParams{VaultID: id, Seq: other.Seq})
			if err != nil {
				return err
			}
			text = r.Content
			return nil
		}
		v, err := c.Note().ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: id, Version: other.Version})
		if err != nil {
			return err
		}
		text = v.Content
		return nil
	}, func(err error) { done(text, err) })
}

func mergeName(o *workspace.MergeSide) string {
	switch o.Kind {
	case "revision":
		return "a hub revision"
	case "held":
		return "your text"
	}
	return "a version"
}

func (a *App) mergeDone(tc *tabContent, tabID string) {
	if n := tc.doc.Notice(); n != nil && (n.Kind == notes.NoticeConflict || n.Kind == notes.NoticeRefused) {
		tc.doc.Dismiss()
	}
	if a.syncInfo != nil {
		id, p := a.vault.ID, tc.view.Path
		a.call(func(ctx context.Context, c *api.Client) error {
			return c.Sync().Resolve(ctx, &api.SyncResolveParams{VaultID: id, Path: p})
		}, nil)
	}
	a.updateWorkspace(func(w *workspace.Workspace) { w.Close(tabID) })
}

// loadFile reads an image tab's bytes, once the catalog knows its hash: a
// tab the workspace restored draws before the files are read.
func (a *App) loadFile(tc *tabContent) {
	if !a.Catalog.Loaded() {
		return
	}
	tc.fileRead = true
	if panes.FileType(tc.view.Path) != "image" {
		return
	}
	hash := a.fileHash(tc.view.Path)
	if hash == "" {
		tc.err = "This file is not in the vault."
		return
	}
	blob := filepath.Join(a.cfg.Blobs, hash[:2], hash)
	ext := strings.TrimPrefix(filepath.Ext(tc.view.Path), ".")
	go func() { // a large file must not hold up the window
		var img ui.ImageSource
		data, err := reading.ReadImage(blob)
		if err == nil {
			img, err = reading.Decode(data, ext)
		}
		a.cfg.Env.Post(func() {
			if err != nil {
				tc.err = err.Error()
				return
			}
			tc.img = img
		})
	}()
}

func (a *App) fileHash(p string) string {
	f, _ := a.Catalog.File(p)
	return f.Hash
}

// workspaceView draws the tab groups in their splits.
func (a *App) workspaceView(c *ui.Context) {
	a.sweep()
	a.wasDrag, a.dragging = a.dragging, false
	root := a.splitNode(a.Store.Workspace().Root)
	uikit.SplitView(c, root, &a.split, func(id string, sizes []float64) {
		a.updateWorkspace(func(w *workspace.Workspace) { w.Resize(id, sizes) })
	})
}

func (a *App) splitNode(n *workspace.Node) *uikit.SplitNode {
	if n.Kind == workspace.KindTabs {
		return &uikit.SplitNode{ID: n.ID, Leaf: func(c *ui.Context) { a.group(c, n) }}
	}
	out := &uikit.SplitNode{ID: n.ID, Row: n.Direction == workspace.Row, Sizes: n.Sizes}
	for _, ch := range n.Children {
		out.Children = append(out.Children, a.splitNode(ch))
	}
	return out
}

func (a *App) group(c *ui.Context, g *workspace.Node) {
	col := ui.Column(c).Grow(1).MinWidth(160).MinHeight(160).AlignItems(ui.Stretch)
	// The focus coming into a group makes it the active one, as a click in
	// its editor does; a command that moves the active group moves no focus.
	if col.FocusWithin() {
		if a.focusGroup != g.ID {
			a.focusGroup = g.ID
			if a.Store.Workspace().ActiveID != g.ID {
				a.updateWorkspace(func(w *workspace.Workspace) { w.ActiveID = g.ID })
			}
		}
	} else if a.focusGroup == g.ID {
		a.focusGroup = ""
	}
	col.Children(func() {
		a.strip(c, g)
		tab := &g.Tabs[g.Active]
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.Prefs.ViewHeader {
				a.viewHeader(c, tab)
			}
			a.tabView(c, tab)
			if a.wasDrag {
				uikit.DropEdges(c, func(id, edge string) {
					a.updateWorkspace(func(w *workspace.Workspace) { w.MoveTab(id, g.ID, 0, edge) })
				})
			}
		})
	})
}

func (a *App) strip(c *ui.Context, g *workspace.Node) {
	tabs := make([]uikit.Tab, len(g.Tabs))
	for i, t := range g.Tabs {
		tabs[i] = uikit.Tab{ID: t.ID, Title: t.View.Title(), Pinned: t.Pinned}
	}
	uikit.TabStrip{Label: "Tabs", NewLabel: "New tab", ListLabel: "Tab list", CloseLabel: func(title string) string { return "Close " + title }, Tabs: tabs, Active: g.Active, Dragging: &a.dragging, Revealed: a.revealedTab(g.ID),
		Activate: func(id string) { a.updateWorkspace(func(w *workspace.Workspace) { w.Activate(id) }) },
		Close:    func(id string) { a.updateWorkspace(func(w *workspace.Workspace) { w.Close(id) }) },
		New: func() {
			a.updateWorkspace(func(w *workspace.Workspace) { w.ActiveID = g.ID; w.Open(workspace.Empty(), workspace.NewTab) })
		},
		Move: func(id string, i int) { a.updateWorkspace(func(w *workspace.Workspace) { w.MoveTab(id, g.ID, i, "") }) },
		Menu: func(m *ui.Menu, id string) { a.tabMenu(m, g, id) },
		List: func(m *ui.Menu) {
			for i, t := range g.Tabs {
				if m.Item(t.View.Title()).Checked(i == g.Active).Chosen() {
					a.updateWorkspace(func(w *workspace.Workspace) { w.Activate(t.ID) })
				}
			}
		},
	}.View(c)
}

// revealedTab is the tab a group's strip last scrolled into view.
func (a *App) revealedTab(group string) *string {
	if a.revealed == nil {
		a.revealed = map[string]*string{}
	}
	r := a.revealed[group]
	if r == nil {
		r = new(string)
		a.revealed[group] = r
	}
	return r
}

func (a *App) tabMenu(m *ui.Menu, g *workspace.Node, id string) {
	up := a.updateWorkspace
	if m.Item("Close").Chosen() {
		up(func(w *workspace.Workspace) { w.Close(id) })
	}
	if m.Item("Close others").Chosen() {
		up(func(w *workspace.Workspace) { w.CloseOthers(id) })
	}
	if m.Item("Close tabs to the right").Chosen() {
		up(func(w *workspace.Workspace) { w.CloseToTheRight(id) })
	}
	if m.Item("Close all").Chosen() {
		up(func(w *workspace.Workspace) { w.CloseAll(g.ID) })
	}
	m.Separator()
	gr, i := a.Store.Workspace().FindTab(id)
	pinned := gr != nil && gr.Tabs[i].Pinned
	title := "Pin"
	if pinned {
		title = "Unpin"
	}
	if m.Item(title).Chosen() {
		up(func(w *workspace.Workspace) { w.Pin(id, !pinned) })
	}
	m.Separator()
	if m.Item("Split right").Chosen() {
		up(func(w *workspace.Workspace) { w.Split(id, workspace.ToRight) })
	}
	if m.Item("Split down").Chosen() {
		up(func(w *workspace.Workspace) { w.Split(id, workspace.Below) })
	}
}

func (a *App) viewHeader(c *ui.Context, tab *workspace.Tab) {
	ui.Row(c).Height(32).Padding(0, frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
		if uikit.ToolButton(c, "go-previous", "Navigate back").Disabled(len(tab.Back) == 0).Clicked() {
			a.updateWorkspace(func(w *workspace.Workspace) { w.Activate(tab.ID); w.Back() })
		}
		if uikit.ToolButton(c, "go-next", "Navigate forward").Disabled(len(tab.Forward) == 0).Clicked() {
			a.updateWorkspace(func(w *workspace.Workspace) { w.Activate(tab.ID); w.Forward() })
		}
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Justify(ui.Center).AlignItems(ui.Center).Gap(frostui.SpaceXS).Children(func() { a.breadcrumb(c, tab.View.Path) })
		if tab.View.Type == workspace.ViewNote {
			icon, label := "document-edit", "Current view: editing (Ctrl E)"
			if tab.View.Mode == workspace.ModeRead {
				icon, label = "document-open", "Current view: reading (Ctrl E)"
			}
			if uikit.ToolButton(c, icon, label).Clicked() {
				a.updateWorkspace(func(w *workspace.Workspace) { w.Activate(tab.ID) })
				a.toggleMode()
			}
		}
		if tab.View.Path != "" {
			uikit.ToolButton(c, "view-more", "More options").Menu(func(m *ui.Menu) { a.moreOptions(m, tab) })
		}
	})
}

// breadcrumb is a file's folders, each a button that reveals it in the
// Files pane, and its title.
func (a *App) breadcrumb(c *ui.Context, p string) {
	t := c.Theme()
	if p == "" {
		return
	}
	parts := strings.Split(strings.TrimSuffix(p, ".md"), "/")
	for i, part := range parts[:len(parts)-1] {
		folder := strings.Join(parts[:i+1], "/")
		b := ui.ButtonBase(c.Key(folder)).Cursor(ui.CursorPointer).Label("Reveal " + part)
		b.Children(func() { ui.Text(c, part).AccessHidden(true).FontSize(13).TextColor(t.TextMuted).SingleLine() })
		if b.Clicked() {
			a.files.Reveal(folder)
			a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "files") })
		}
		ui.Text(c.Key(folder+"/"), "/").FontSize(13).TextColor(t.TextMuted)
	}
	ui.Text(c, parts[len(parts)-1]).FontSize(13).SingleLine()
}

func (a *App) moreOptions(m *ui.Menu, tab *workspace.Tab) {
	p := tab.View.Path
	if m.Item("Open version history").Chosen() {
		a.openHistory(p)
	}
	m.Separator()
	if m.Item("Split right").Chosen() {
		a.updateWorkspace(func(w *workspace.Workspace) { w.Split(tab.ID, workspace.ToRight) })
	}
	if m.Item("Split down").Chosen() {
		a.updateWorkspace(func(w *workspace.Workspace) { w.Split(tab.ID, workspace.Below) })
	}
	m.Separator()
	if m.Item("Rename…").Chosen() {
		a.files.Reveal(p)
		a.files.Rename(p)
		a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "files") })
	}
	if m.Item("Move file to…").Chosen() {
		a.chooseFolder(p, false)
	}
	if m.Item("Copy path").Chosen() && a.cfg.Copy != nil {
		a.cfg.Copy(p)
	}
	if m.Item("Reveal in Files").Chosen() {
		a.files.Reveal(p)
		a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideLeft, "files") })
	}
	if m.Item("Open in default app").Chosen() {
		a.report(a.openVaultFile(p))
	}
	m.Separator()
	if m.Item("Delete file").Chosen() {
		a.deleteFile(p, false)
	}
}

func (a *App) tabView(c *ui.Context, tab *workspace.Tab) {
	if tab.View.Type == workspace.ViewEmpty {
		a.emptyTab(c)
		return
	}
	tc := a.content(tab)
	el := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch)
	el.Children(func() {
		if tc.err != "" {
			frostui.Problem(c, tc.err)
			return
		}
		if tc.doc != nil && tc.hist == nil {
			if !tc.doc.Loaded() && tc.doc.Status() == notes.StatusMissing {
				frostui.Empty(c, "dialog-question", "This note is not in the vault", tab.View.Path)
				return
			}
			editor.Notice(c, tc.doc, editor.NoticeActions{
				CompareVersion: func(v int64) { a.openMergeSide(tc.doc.Path(), &workspace.MergeSide{Kind: "version", Version: v}) },
				CompareText:    func(s string) { a.openMerge(tc.doc.Path(), s, "your text") },
				Close:          func() { a.updateWorkspace(func(w *workspace.Workspace) { w.Close(tab.ID) }) },
			})
		}
		switch {
		case tc.ed != nil:
			tc.ed.ShowTitle, tc.ed.ShowProperties, tc.ed.Readable = a.Prefs.InlineTitle, a.Prefs.Properties == "visible", a.Prefs.ReadableLineLength
			tc.ed.HideProperties = a.Prefs.Properties == "hidden"
			tc.ed.View(c)
			if !tc.restored && tc.doc.Loaded() {
				tc.restored = true
				if tab.View.Cursor > 0 {
					tc.ed.Editor.State.Select(tab.View.Cursor, tab.View.Cursor)
				}
				tc.ed.Editor.State.SetScrollY(tab.View.Scroll)
			}
			if a.pending != nil && a.pending.tabID == tab.ID && a.pending.path == tc.doc.Path() && tc.doc.Loaded() {
				line := a.pending.line
				if a.pending.subpath != "" {
					line = editor.SubpathLine(tc.doc.Text(), a.pending.subpath)
				}
				if a.pending.subpath == "" || line > 0 {
					goToLine(tc.ed.Editor, line)
				}
				a.pending = nil
			}
		case tc.read != nil:
			if p := a.pending; p != nil && p.subpath != "" && p.tabID == tab.ID && p.path == tc.doc.Path() && tc.read.Markdown() != "" {
				tc.read.GoTo(p.subpath)
				a.pending = nil
			}
			ui.Scroll(c).TrackScroll(&tc.scroll).Grow(1).Fill().Padding(frostui.SpaceL, frostui.SpaceXL).Children(func() { tc.read.View(c) })
		case tc.hist != nil:
			tc.hist.View(c)
		case tc.merg != nil:
			tc.merg.View(c)
		case tc.img != nil:
			ui.Scroll(c).Grow(1).Fill().Center().Children(func() { ui.Image(c, tc.img) })
		case tab.View.Type == workspace.ViewFile:
			a.fileTab(c, tc)
		}
	})
}

func (a *App) fileTab(c *ui.Context, tc *tabContent) {
	if media.Kind(tc.view.Path) != "" {
		hash := a.fileHash(tc.view.Path)
		ui.Column(c).Fill().Center().Children(func() {
			media.View(c, a.cfg.Blobs, "blob:"+hash+strings.ToLower(filepath.Ext(tc.view.Path)), filepath.Base(tc.view.Path))
			if frostui.LabeledButton(c, "document-open", "Open in default app", true).Clicked() {
				a.report(a.openVaultFile(tc.view.Path))
			}
		})
		return
	}

	ui.Column(c).Fill().Center().Gap(frostui.SpaceM).Children(func() {
		frostui.Muted(c, filepath.Base(tc.view.Path)+" opens outside the app.")
		if frostui.LabeledButton(c, "document-open", "Open in default app", true).Clicked() {
			a.report(a.openVaultFile(tc.view.Path))
		}
	})
}

func (a *App) emptyTab(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(frostui.SpaceS).Children(func() {
		for _, l := range []struct{ title, id string }{{"Create new note", "file-explorer:new-file"}, {"Go to file", "switcher:open"}, {"Close", "workspace:close"}} {
			if ui.Button(c.Key(l.id), l.title).Clicked() {
				a.run(l.id)
			}
		}
	})
}

// activeFile is the active tab's file, "" for none.
func (a *App) activeFile() string {
	v := a.Store.Workspace().ActiveTab().View
	if v.Type == workspace.ViewNote || v.Type == workspace.ViewFile {
		return v.Path
	}
	return ""
}

func (a *App) activeContent() *tabContent { return a.tabs[a.Store.Workspace().ActiveTab().ID] }

func (a *App) activeText() (string, bool) {
	if tc := a.activeContent(); tc != nil && tc.doc != nil && tc.doc.Loaded() {
		return tc.doc.Text(), true
	}
	return "", false
}

func (a *App) activeEditor() *editor.NoteEditor {
	if tc := a.activeContent(); tc != nil && tc.ed != nil {
		return tc.ed.Editor
	}
	return nil
}

func (a *App) caretLine() int {
	if e := a.activeEditor(); e != nil {
		return e.State.CursorPos().Line + 1
	}
	return 0
}

func (a *App) inEditor() bool {
	e := a.activeEditor()
	return e != nil && e.State.Focused()
}

type textCounts struct {
	text         string
	words, chars int
}

// textCounts is the active note's words and characters, of the selection
// when there is one.
func (a *App) textCounts() (int, int, bool) {
	text, ok := a.activeText()
	if !ok {
		return 0, 0, false
	}
	if e := a.activeEditor(); e != nil {
		if s := e.State.SelectedText(); s != "" {
			return countWords(s), utf8.RuneCountInString(s), true
		}
	}
	if text != a.counts.text || a.counts.text == "" {
		a.counts = textCounts{text: text, words: countWords(text), chars: utf8.RuneCountInString(text)}
	}
	return a.counts.words, a.counts.chars, true
}

func countWords(s string) int {
	n, in := 0, false
	for _, r := range s {
		if unicode.IsSpace(r) {
			in = false
		} else if !in {
			in = true
			n++
		}
	}
	return n
}

func (a *App) modeName(v workspace.View) string {
	if v.Mode == workspace.ModeRead {
		return "Reading view"
	}
	if a.Prefs.EditingMode == "live" {
		return "Live Preview"
	}
	return "Source mode"
}

func (a *App) toggleMode() {
	a.updateWorkspace(func(w *workspace.Workspace) {
		t := w.ActiveTab()
		if t.View.Type != workspace.ViewNote {
			return
		}
		v := t.View
		if v.Mode == workspace.ModeRead {
			v.Mode = workspace.ModeEdit
		} else {
			v.Mode = workspace.ModeRead
		}
		w.SetView(t.ID, v)
	})
}

// savePositions snapshots every mounted note before navigation changes its view.
func (a *App) savePositions() {
	for _, g := range workspace.Groups(a.Store.Workspace().Root) {
		for i := range g.Tabs {
			tab := &g.Tabs[i]
			tc := a.tabs[tab.ID]
			if tc == nil || !sameContent(tc.view, tab.View) {
				continue
			}
			if tc.ed != nil && tc.restored {
				tab.View.Cursor = tc.ed.Editor.State.Cursor()
				tab.View.Scroll = tc.ed.Editor.State.ScrollY()
			} else if tc.read != nil {
				tab.View.Scroll = float64(tc.scroll.Y)
			}
		}
	}
}
func (a *App) updateWorkspace(op func(*workspace.Workspace)) {
	a.savePositions()
	a.Store.Update(op)
}
