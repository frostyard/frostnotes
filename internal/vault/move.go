package vault

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/store"
)

// Move renames from to to, which must not exist, creating missing folders;
// the file keeps its ID and versions. With updateLinks, every link to it
// from a note, and every link of a moved note that the move would point
// elsewhere, is rewritten to the shortest target that resolves to the same
// file, as Obsidian's "Automatically update internal links" does; each
// note's rewrite is an ordinary write with a base
// (docs/design/vault.md#writes).
func (v *Vault) Move(ctx context.Context, from, to string, updateLinks bool, who Author) (*api.MoveResult, error) {
	for _, p := range []string{from, to} {
		if err := checkNotePath(p); err != nil {
			return nil, err
		}
	}
	if from == to {
		return nil, api.InvalidParams("%s is already at %s", from, to)
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(from, to); err != nil {
		return nil, err
	}
	row, _, err := v.refresh(ctx, r, from)
	if err != nil {
		return nil, apiError(from, err)
	}
	if _, _, err := v.refresh(ctx, r, to); err == nil {
		return nil, api.Conflict("%s exists", to)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, apiError(to, err)
	}
	if _, err := r.Lstat(to); err == nil {
		return nil, api.Conflict("%s exists and is not a regular file", to)
	}
	moves := map[string]string{from: to}
	var plans []rewritePlan
	if updateLinks {
		if plans, err = v.planRewrites(ctx, r, []store.File{row}, moves); err != nil {
			return nil, err
		}
		if err := checkRewrites(ctx, plans, moves); err != nil {
			return nil, err
		}
	}
	if err := mkdirs(r, to); err != nil {
		return nil, apiError(to, err)
	}
	if err := v.settled(rename(r, v.root, from, to)); errors.Is(err, fs.ErrExist) {
		return nil, api.Conflict("%s exists", to)
	} else if err != nil {
		return nil, err
	}
	if v.crashed("renamed") {
		return nil, ErrCrashed
	}
	// The file has moved: record it, and rewrite its links, whatever the
	// client does now.
	ctx = context.WithoutCancel(ctx)
	var doc markdown.Doc
	var data []byte
	becameNote := kindOf(to) == store.FileNote && row.Kind() != store.FileNote
	if becameNote {
		if c, err := read(r, to); err == nil && c.data != nil {
			data, doc = c.data, markdown.Parse(c.data)
		}
	}
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		f, err := tx.MoveFile(ctx, row.ID, to)
		if err != nil {
			return err
		}
		if becameNote {
			if err := tx.IndexNote(ctx, f, data, doc); err != nil {
				return err
			}
		}
		if err := v.logRevision(ctx, tx, api.RevisionOpMove, f.ID, f.Path, f.Hash, f.Size, who); err != nil {
			return err
		}
		return tx.Emit(ctx, api.NoteMoved{VaultID: v.id, From: from, To: to})
	})
	if err != nil {
		return nil, err
	}
	return &api.MoveResult{Path: to, Rewritten: v.rewrite(ctx, r, plans, moves, who)}, nil
}

// rewrite carries out plans after the moves (old path to new) they were
// planned for, and returns the notes it rewrote, by path. Each rewrite is
// a write with its base; one that fails is logged and skipped, since the
// moves have happened.
func (v *Vault) rewrite(ctx context.Context, r *os.Root, plans []rewritePlan, moves map[string]string, who Author) []string {
	rewritten := []string{}
	for _, plan := range plans {
		source := plan.source
		if to, ok := moves[source]; ok {
			source = to
		}
		content, changed := v.rewriteLinks(ctx, source, plan)
		if !changed {
			continue
		}
		if _, err := v.write(ctx, r, source, plan.hash, content, false, who); err != nil {
			v.log.Warn("rewrite links after a move", "note", source, "err", err)
			continue
		}
		rewritten = append(rewritten, source)
	}
	slices.Sort(rewritten)
	return rewritten
}

// rewritePlan is a note whose links a move retargets, as read before it.
type rewritePlan struct {
	source string // the note's path before the move
	hash   string // its content's hash: the base of its rewrite
	data   []byte
	links  []retarget
}

// retarget is a link and the file it must resolve to after the move.
type retarget struct {
	link markdown.Link
	want string
}

// lookup is markdown.Resolve's files(name) over the vault's index; errors
// leave it empty, so a link they touch is left as it is.
func (v *Vault) lookup(ctx context.Context) func(string) []string {
	return func(name string) []string {
		paths, err := v.cfg.DB.FilesNamed(ctx, v.id, name)
		if err != nil {
			v.log.Warn("look up files by name", "name", name, "err", err)
		}
		return paths
	}
}

// planRewrites finds, before the files moved go to their paths in moves
// (old path to new), the notes that link to them and, for the notes among
// them, their own links.
func (v *Vault) planRewrites(ctx context.Context, r *os.Root, moved []store.File, moves map[string]string) ([]rewritePlan, error) {
	var sources []string
	for _, f := range moved {
		backlinks, err := v.cfg.DB.Backlinks(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		for _, b := range backlinks {
			sources = append(sources, b.Source)
		}
		if f.Kind() == store.FileNote {
			sources = append(sources, f.Path)
		}
	}
	slices.Sort(sources)
	sources = slices.Compact(sources)
	lookup := v.lookup(ctx)
	var plans []rewritePlan
	for _, source := range sources {
		row, cur, err := v.refresh(ctx, r, source)
		if err != nil || cur.data == nil || row.Kind() != store.FileNote {
			continue
		}
		plan := rewritePlan{source: source, hash: row.Hash, data: cur.data}
		for _, l := range markdown.Parse(cur.data).Links {
			if l.Target == "" {
				continue // the note itself, wherever it is
			}
			dest := markdown.Resolve(source, l.Target, lookup)
			_, sourceMoves := moves[source]
			if to, ok := moves[dest]; ok {
				plan.links = append(plan.links, retarget{l, to})
			} else if sourceMoves && dest != "" {
				plan.links = append(plan.links, retarget{l, dest})
			}
		}
		if len(plan.links) > 0 {
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

// rewriteLinks returns plan's note, now at source, with each link that no
// longer resolves to its file retargeted, and whether any was. Each
// rewrite replaces only the link's target, so links that nest, as a
// clickable image's do, are rewritten both.
func (v *Vault) rewriteLinks(ctx context.Context, source string, plan rewritePlan) ([]byte, bool) {
	lookup := v.lookup(ctx)
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for _, rt := range plan.links {
		l := rt.link
		if markdown.Resolve(source, l.Target, lookup) == rt.want {
			continue
		}
		target, ok := shortestTarget(source, rt.want, l.Kind, lookup)
		if !ok {
			continue
		}
		start, end, text, ok := targetSpan(string(plan.data[l.Start:l.End]), l.Kind, target)
		if !ok {
			continue
		}
		edits = append(edits, edit{l.Start + start, l.Start + end, text})
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var out []byte
	last := 0
	for _, e := range edits {
		if e.start < last {
			continue // overlaps an edit made already; never for well-formed links
		}
		out = append(out, plan.data[last:e.start]...)
		out = append(out, e.text...)
		last = e.end
	}
	out = append(out, plan.data[last:]...)
	return out, last > 0
}

// shortestTarget is how a link from source names want, as Obsidian's
// "shortest path when possible" writes it: the file name when that
// resolves to want, else its vault path, else the path from the root. A
// wikilink leaves out a note's ".md"; a markdown link keeps it.
func shortestTarget(source, want string, kind markdown.LinkKind, files func(string) []string) (string, bool) {
	full := want
	if kind != markdown.LinkMarkdown {
		full = strings.TrimSuffix(full, ".md")
	}
	for _, target := range []string{path.Base(full), full, "/" + full} {
		if markdown.Resolve(source, target, files) == want {
			return target, true
		}
	}
	return "", false
}

// retargetText is raw, a link's text, with its target replaced, keeping
// its subpath, alias and embed mark.
func retargetText(raw string, kind markdown.LinkKind, target string) (string, bool) {
	start, end, text, ok := targetSpan(raw, kind, target)
	if !ok {
		return "", false
	}
	return raw[:start] + text + raw[end:], true
}

// targetSpan returns the byte range of the target in raw, a link's text,
// and what replaces it to name target.
func targetSpan(raw string, kind markdown.LinkKind, target string) (start, end int, text string, ok bool) {
	if kind != markdown.LinkMarkdown {
		if strings.ContainsAny(target, "[]|#^") {
			return 0, 0, "", false
		}
		open, closing := strings.Index(raw, "[["), strings.LastIndex(raw, "]]")
		if open < 0 || closing < open+2 {
			return 0, 0, "", false
		}
		inner := raw[open+2 : closing]
		cut := len(inner)
		if i := strings.IndexByte(inner, '|'); i >= 0 {
			cut = i
			if cut > 0 && inner[cut-1] == '\\' {
				cut--
			}
		}
		if i := strings.IndexByte(inner[:cut], '#'); i >= 0 {
			cut = i
		}
		return open + 2, open + 2 + cut, target, true
	}
	i := strings.LastIndex(raw, "](")
	if i < 0 || !strings.HasSuffix(raw, ")") {
		return 0, 0, "", false // a reference link: its definition names the file
	}
	start = i + 2
	inside := raw[start : len(raw)-1]
	rest := strings.TrimLeft(inside, " \t")
	start += len(inside) - len(rest)
	var dest string
	if strings.HasPrefix(rest, "<") {
		end := strings.IndexByte(rest, '>')
		if end < 0 || strings.ContainsAny(target, "<>") {
			return 0, 0, "", false
		}
		dest = rest[1:end]
		sub := ""
		if j := strings.IndexByte(dest, '#'); j >= 0 {
			sub = dest[j:]
		}
		return start, start + end + 1, "<" + target + sub + ">", true
	}
	end = strings.IndexAny(rest, " \t")
	if end < 0 {
		end = len(rest)
	}
	dest = rest[:end]
	sub := ""
	if j := strings.IndexByte(dest, '#'); j >= 0 {
		sub = dest[j:]
	}
	return start, start + end, escapePath(target) + sub, true
}

// escapePath percent-encodes each segment of a vault path for a markdown
// link's destination.
func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}
