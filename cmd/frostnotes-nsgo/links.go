package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/markup"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/nsgo-dev/nsgo/base/editor"
)

// The vault's catalog (internal/desktop/catalog: its files, aliases and
// tags, and the link text of its link format), what source mode completes
// from, and following the link or tag under the caret, as the MyGo client
// does (internal/desktop/window/actions.go, followLink).

// loadVaultData makes the shown vault's catalog and reads its settings,
// which give the link format, where a linked note is made and whether a
// move updates links.
func (w *workspace) loadVaultData() {
	id := w.vault.GetUntracked()
	if id == 0 {
		w.catalog, w.vaultSettings = nil, nil
		return
	}
	if w.catalog == nil || w.catalogVault != id {
		w.catalog, w.catalogVault, w.vaultSettings = catalog.New(environment{w.cx}, w.catalogAPI, id), id, nil
	}
	w.catalog.Reload()
	c := w.session.Client()
	if c == nil {
		return
	}
	cat := w.catalog
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		settings, err := c.Vault().Settings(ctx, &api.VaultSettingsParams{ID: id})
		w.cx.Post(func() {
			if err != nil || w.catalog != cat {
				return
			}
			w.vaultSettings = settings
			cat.LinkFormat, cat.AttachmentFolder, cat.Wikilinks = string(settings.LinkFormat), settings.AttachmentFolder, settings.Wikilinks
		})
	}()
}

func (w *workspace) catalogAPI() catalog.API {
	if c := w.session.Client(); c != nil {
		n := c.Note()
		return &n
	}
	return nil
}

// sources are what the note d completes from, nil without a catalog.
func (w *workspace) sources(d *notes.Document) *catalog.NoteSources {
	if w.catalog == nil || w.catalogVault != d.Vault() {
		return nil
	}
	return w.catalog.Sources(d.Path, func(p string) (string, bool) {
		if cur := w.doc.GetUntracked(); cur != nil && cur.Path() == p && cur.Loaded() {
			return cur.Text(), true
		}
		return "", false
	})
}

// maxCompletions is how many notes or tags the list holds, the MyGo
// client's.
const maxCompletions = 50

// complete answers the editor's request for completions: the notes after
// [[ in the vault's link format, a note's headings after [[note# and its
// blocks after [[note#^, and the tags after #, by how many notes have
// them. Typing on asks again for notes, as the list holds the best few.
func (s *sourceEditor) complete(r editor.CompletionRequest) {
	src := s.w.sources(s.d)
	if src == nil {
		return
	}
	at := offset(s.last, r.Position)
	c, ok := markup.CompletionAt(s.last, at)
	if !ok {
		return
	}
	rng := &editor.Range{Start: position(s.last, c.From), End: r.Position}
	closing := "]]"
	if c.Closed {
		closing = ""
	}
	switch c.Kind {
	case markup.CompleteNote:
		var items []editor.CompletionItem
		for _, t := range src.LinkTargets(c.Query, maxCompletions) {
			items = append(items, editor.CompletionItem{Label: t.Label, Detail: t.Detail, InsertText: t.Insert + closing, Kind: "note", Range: rng})
		}
		s.ref.PresentCompletions(r.ID, items, true)
	case markup.CompleteHeading, markup.CompleteBlock:
		kind := editor.CompletionKind("head")
		if c.Kind == markup.CompleteBlock {
			kind = "block"
		}
		src.Subpaths(c.Target, c.Kind == markup.CompleteBlock, func(found []markup.Target) {
			var items []editor.CompletionItem
			for _, t := range found {
				items = append(items, editor.CompletionItem{Label: t.Label, Detail: t.Detail, InsertText: t.Insert + closing, Kind: kind, Range: rng})
			}
			// The editor drops an answer to a request it has gone past.
			s.ref.PresentCompletions(r.ID, items, false)
		})
	case markup.CompleteTag:
		var items []editor.CompletionItem
		for _, tag := range src.Tags() {
			if markup.FuzzyMatch(tag, c.Query) {
				items = append(items, editor.CompletionItem{Label: tag, InsertText: tag, Kind: "tag", Range: rng,
					SortText: fmt.Sprintf("%04d", len(items))})
			}
			if len(items) == maxCompletions {
				break
			}
		}
		s.ref.PresentCompletions(r.ID, items, false)
	}
}

// definition answers Ctrl over a link or tag, which it underlines, and
// Ctrl+click and F12, which follow it.
func (s *sourceEditor) definition(r editor.DefinitionRequest) {
	at := offset(s.last, r.Position)
	if r.Trigger == editor.DefinitionHover {
		if l, ok := markup.LinkAt(s.last, at); ok && l.URL == "" {
			s.ref.PresentDefinitionLink(r.ID, editor.Range{Start: position(s.last, l.From), End: position(s.last, l.To)})
		}
		return
	}
	s.followAt(at)
}

// followCaret follows the link or tag under the caret (Alt+Enter).
func (s *sourceEditor) followCaret() { s.followAt(s.caretOffset()) }

// followAt follows the link or tag at byte offset at: a note link opens
// its note, at its heading or block, a link to a note that is not there
// makes it, a heading or block of this note is shown, and a tag searches
// the vault for it. A web link waits for the allow-list of plan 0011's
// Phase 8, and opening a link in a new tab or to the right for its
// Phase 3.
func (s *sourceEditor) followAt(at int) {
	if l, ok := markup.LinkAt(s.last, at); ok {
		s.w.followLink(s.d, l)
		return
	}
	if tag, ok := markup.TagAt(s.last, at); ok {
		s.w.searchFor("tag:#" + tag)
	}
}

// searchFor shows Search vault with query in its field.
func (w *workspace) searchFor(query string) {
	if w.vault.GetUntracked() == 0 {
		return
	}
	w.searchQuery = query
	w.action.Set("")
	w.action.Set(vaultSearch)
}

// jump is a heading or block of a note to show once its source mode shows
// it.
type jump struct{ path, subpath string }

// followLink follows l, a link in the note d.
func (w *workspace) followLink(d *notes.Document, l markup.Link) {
	if l.URL != "" {
		return
	}
	if l.Target == "" {
		w.jumpTo(d.Path(), l.Subpath)
		return
	}
	c := w.session.Client()
	if c == nil {
		w.problem.Set("Reconnect to follow this link.")
		return
	}
	w.navigation++
	request, from, id := w.navigation, d.Path(), d.Vault()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		hits, err := c.Note().Resolve(ctx, &api.NoteResolveParams{VaultID: id, From: from, Targets: []string{l.Target}})
		w.cx.Post(func() {
			if w.navigation != request || w.doc.GetUntracked() != d || w.session.Client() != c {
				return
			}
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			if len(hits) > 0 && hits[0].Path != nil {
				// A file that is not a note waits for plan 0011's Phase 5.
				if p := *hits[0].Path; strings.HasSuffix(strings.ToLower(p), ".md") {
					w.jumpTo(p, l.Subpath)
				}
				return
			}
			w.createLinked(d, l.Target)
		})
	}()
}

// jumpTo opens the note at p and shows its heading or block sub there.
func (w *workspace) jumpTo(p, sub string) {
	w.jump = jump{p, sub}
	if s := w.source; s != nil && s.d.Path() == p {
		s.takeJump()
		return
	}
	w.open(p)
}

// takeJump shows the heading or block the workspace was asked to, when it
// is of this note.
func (s *sourceEditor) takeJump() {
	j := s.w.jump
	if j.path == "" || j.path != s.d.Path() {
		return
	}
	s.w.jump = jump{}
	if j.subpath != "" {
		if line := markup.SubpathLine(s.last, j.subpath); line > 0 {
			s.ref.GoToLine(line - 1)
		}
	}
	s.ref.Focus()
}

// createLinked makes the note an unresolved link in d names, where the
// vault's settings say (catalog.LinkedNotePath), and opens it; a note
// there already opens.
func (w *workspace) createLinked(d *notes.Document, target string) {
	p := catalog.LinkedNotePath(target, w.vaultSettings, d.Path())
	if p == "" {
		return
	}
	if slices.ContainsFunc(w.files.GetUntracked(), func(f api.NoteEntry) bool { return strings.EqualFold(f.Path, p) }) {
		w.open(p)
		return
	}
	c := w.session.Client()
	if c == nil {
		return
	}
	id := d.Vault()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result, err := c.Note().Create(ctx, &api.NoteCreateParams{VaultID: id, Path: p})
		w.cx.Post(func() {
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			if id != w.vault.GetUntracked() {
				return
			}
			w.loadFiles()
			w.open(result.Path)
		})
	}()
}
