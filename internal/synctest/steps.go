package synctest

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
)

var folders = []string{"", "a/", "a/b/", "c/"}

// benign is a step's failure the world's state explains: the file changed
// or went since the step looked, or its target is taken. The step then
// does nothing.
func benign(err error) bool {
	var ae *api.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case api.CodeConflict, api.CodeNotFound, api.CodeInvalidParams, api.CodeUnavailable:
			return true
		}
	}
	return false
}

// files lists node n's vault files on disk, hidden ones aside, by path.
func files(n *Node) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(n.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != n.Dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(n.Dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

// pick returns the k-th of node n's files (notes only when notes), by
// path, or "" when it has none.
func pick(n *Node, k uint64, notes bool) string {
	var ps []string
	for p := range files(n) {
		if !notes || strings.HasSuffix(p, ".md") {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return ""
	}
	slices.Sort(ps)
	return ps[k%uint64(len(ps))]
}

// insertLine puts line into content at line index k, and, with remove,
// takes out one other line that holds a token.
func insertLine(content, line string, k uint64, remove bool) (string, string) {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
		lines[len(lines)-1] += "\n"
	}
	removed := ""
	if remove {
		var withTokens []int
		for i, l := range lines {
			if tokenRE.MatchString(l) {
				withTokens = append(withTokens, i)
			}
		}
		if len(withTokens) > 0 {
			i := withTokens[(k>>8)%uint64(len(withTokens))]
			removed = lines[i]
			lines = slices.Delete(lines, i, i+1)
		}
	}
	at := int(k % uint64(len(lines)+1))
	lines = slices.Insert(lines, at, line)
	return strings.Join(lines, ""), removed
}

func (w *World) device(s Step) *Node { return w.Devices[s.Device%len(w.Devices)] }

func (w *World) step(i int, s Step) error {
	d := w.device(s)
	switch s.Op {
	case Edit:
		_, err := w.edit(d, s, api.VersionSourceEditor)
		return err
	case StaleEdit:
		return w.staleEdit(d, s)
	case Append:
		_, err := w.edit(d, Step{Op: Append, A: s.A, B: ^uint64(0)}, api.VersionSourceAgent)
		return err
	case Outside:
		return w.outside(d, i, s)
	case Create:
		return w.create(d, fmt.Sprintf("%sn%d.md", folders[s.A%4], i), fmt.Sprintf("# n%d\n\n", i))
	case Daily:
		return w.daily(d, s)
	case Delete:
		return w.remove(d, s)
	case Move:
		return w.move(d, i, s)
	case DiskMove:
		return w.diskMove(d, i, s)
	case Attach:
		return w.attach(d, s)
	case Offline:
		d.link.cut()
	case Online:
		d.link.heal()
		d.Srv.Syncer.Kick(d.Vault)
	case Crash:
		return w.crash(d, s)
	case HubRestart:
		w.Hub.stop()
		w.Hub.start(w.t)
	case HubCrash:
		return w.hubFault(d, i, s, "renamed", true)
	case DropAnswer:
		return w.hubFault(d, i, s, "pushed", false)
	case Reset:
		if d.link.isOffline() {
			return nil
		}
		if _, err := d.C.Sync().Reset(w.t.Context(), &api.SyncResetParams{VaultID: d.Vault}); err != nil && !benign(err) {
			return err
		}
	}
	return nil
}

// edited is what an edit wrote, or tried to: its token, its path, and the
// token line it took out, if any.
type edited struct {
	tok, path, removed string
}

// edit inserts a token line into a note through notesd, with the base it
// read; with B's bit, it also takes a token line out. It returns what it
// wrote, or tried to.
func (w *World) edit(d *Node, s Step, source api.VersionSource) (edited, error) {
	p := pick(d, s.A, true)
	if p == "" {
		return edited{}, w.create(d, fmt.Sprintf("n-%s.md", w.oracle.token()), "# new\n\n")
	}
	return w.editAt(d, p, s, source)
}

// editAt is edit of the note at p.
func (w *World) editAt(d *Node, p string, s Step, source api.VersionSource) (edited, error) {
	ctx := w.t.Context()
	note, err := d.C.Note().Read(ctx, &api.NoteReadParams{VaultID: d.Vault, Path: p})
	if err != nil {
		if benign(err) {
			return edited{}, nil
		}
		return edited{}, err
	}
	tok := w.oracle.token()
	content, removed := insertLine(note.Content, tok+"\n", s.B, source == api.VersionSourceEditor && (s.B>>16)%5 == 0)
	if source == api.VersionSourceAgent {
		content, removed = strings.TrimSuffix(note.Content, "\n")+"\n"+tok+"\n", ""
		if note.Content == "" {
			content = tok + "\n"
		}
	}
	author := "editor@" + d.Name
	_, err = d.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: d.Vault, Path: p, Base: note.Hash, Content: content,
		Source: &source, Author: &author})
	switch {
	case err == nil:
		w.oracle.insert(tok)
		w.oracle.wrote(tok, d.Name, p)
		if removed != "" {
			w.oracle.remove(removed)
		}
	case benign(err):
	default:
		return edited{tok, p, removed}, err
	}
	if err != nil {
		for _, t := range tokenRE.FindAllString(removed, -1) {
			w.oracle.tried[t] = append(w.oracle.tried[t], fmt.Sprintf("step %d on %s at %s: %v", w.oracle.step, d.Name, p, err))
		}
	}
	return edited{tok, p, removed}, nil
}

// staleEdit writes from an older version of a note, so notesd merges it
// into the current one locally.
func (w *World) staleEdit(d *Node, s Step) error {
	ctx := w.t.Context()
	p := pick(d, s.A, true)
	if p == "" {
		return nil
	}
	note, err := d.C.Note().Read(ctx, &api.NoteReadParams{VaultID: d.Vault, Path: p})
	if err != nil {
		return ignoreBenign(err)
	}
	vs, err := d.C.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: d.Vault, Path: p})
	if err != nil {
		return ignoreBenign(err)
	}
	var older []api.Version
	for _, v := range vs {
		if !v.Deleted && !v.Conflict && v.Hash != note.Hash && v.Path == p {
			older = append(older, v)
		}
	}
	if len(older) == 0 {
		_, err := w.edit(d, s, api.VersionSourceEditor)
		return err
	}
	v := older[s.B%uint64(len(older))]
	old, err := d.Srv.Blobs.Get(v.Hash)
	if err != nil {
		return nil // pruned
	}
	tok := w.oracle.token()
	content, _ := insertLine(string(old), tok+"\n", s.B, false)
	_, err = d.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: d.Vault, Path: p, Base: v.Hash, Content: content})
	if err == nil {
		w.oracle.insert(tok)
		w.oracle.wrote(tok, d.Name, p+" (stale)")
	}
	return ignoreBenign(err)
}

func ignoreBenign(err error) error {
	if err == nil || benign(err) {
		return nil
	}
	return err
}

// outside edits a note on disk as another program would, in place or by
// a rename as vim does; a concurrent run writes only new files, since an
// edit on disk racing a pull loses whichever renames first.
func (w *World) outside(d *Node, i int, s Step) error {
	tok := w.oracle.token()
	if w.o.Concurrent {
		p := fmt.Sprintf("outside/o%d.md", i)
		if err := writeFile(d, p, tok+"\n", false); err != nil {
			return err
		}
		w.oracle.insert(tok)
		w.oracle.wrote(tok, d.Name, p+" (outside)")
		w.paths[p] = true
		return w.index(d, p)
	}
	p := pick(d, s.A, true)
	if p == "" {
		return nil
	}
	cur := files(d)[p]
	content, _ := insertLine(cur, tok+"\n", s.B, false)
	if err := writeFile(d, p, content, (s.B>>8)%2 == 0); err != nil {
		return err
	}
	w.oracle.insert(tok)
	w.oracle.wrote(tok, d.Name, p+" (outside)")
	if d.link.isOffline() && (s.B>>12)%2 == 0 {
		// Back online at once, before the watcher has indexed the edit: a
		// pulled head then meets a file its index does not know changed,
		// which Apply's own comparison must catch.
		d.link.heal()
		d.Srv.Syncer.Kick(d.Vault)
		w.lagging = append(w.lagging, lag{d, p})
		return nil
	}
	return w.index(d, p)
}

// index brings node n's index up to date with p, as its watcher would.
func (w *World) index(n *Node, p string) error {
	_, err := n.C.Note().Read(w.t.Context(), &api.NoteReadParams{VaultID: n.Vault, Path: p})
	return ignoreBenign(err)
}

// writeFile writes content at p in node n's vault, in place or through a
// temporary file and a rename. A folder that sync removes meanwhile, empty
// after a move, is made again.
func writeFile(n *Node, p, content string, inPlace bool) error {
	full := filepath.Join(n.Dir, filepath.FromSlash(p))
	for range 10 {
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		err := writeOnce(full, content, inPlace)
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return fmt.Errorf("%s: its folder kept going", p)
}

func writeOnce(full, content string, inPlace bool) error {
	if inPlace {
		return os.WriteFile(full, []byte(content), 0o644)
	}
	tmp := filepath.Join(filepath.Dir(full), ".synctest-"+filepath.Base(full))
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, full)
}

func (w *World) create(d *Node, p, head string) error {
	tok := w.oracle.token()
	_, err := d.C.Note().Create(w.t.Context(), &api.NoteCreateParams{VaultID: d.Vault, Path: p, Content: head + tok + "\n"})
	if err == nil {
		w.oracle.insert(tok)
		w.oracle.wrote(tok, d.Name, p+" (create)")
		w.paths[p] = true
	}
	return ignoreBenign(err)
}

// daily makes or adds to one of three days' notes, from one template: two
// devices offline make the same note.
func (w *World) daily(d *Node, s Step) error {
	day := fmt.Sprintf("2026-10-%02d", 10+int(s.A%3))
	p := "daily/" + day + ".md"
	if _, ok := files(d)[p]; ok {
		_, err := w.editAt(d, p, s, api.VersionSourceAgent)
		return err
	}
	return w.create(d, p, "# "+day+"\n## Tasks\n## Notes\n")
}

// remove deletes a file with the base it read; what it held is removed on
// purpose.
func (w *World) remove(d *Node, s Step) error {
	ctx := w.t.Context()
	p := pick(d, s.A, false)
	if p == "" {
		return nil
	}
	held := files(d)[p]
	note, err := d.C.Note().Read(ctx, &api.NoteReadParams{VaultID: d.Vault, Path: p})
	if err != nil {
		return ignoreBenign(err)
	}
	err = d.C.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: d.Vault, Path: p, Base: note.Hash})
	if err == nil {
		// What the delete's base held, and what the disk held before.
		w.oracle.remove(held + note.Content)
	}
	return ignoreBenign(err)
}

func movedName(i int, p string) string { return fmt.Sprintf("m%d-%s", i, path.Base(p)) }

func (w *World) move(d *Node, i int, s Step) error {
	p := pick(d, s.A, false)
	if p == "" {
		return nil
	}
	to := folders[s.B%4] + movedName(i, p)
	links := (s.B>>8)%2 == 0
	_, err := d.C.Note().Move(w.t.Context(), &api.NoteMoveParams{VaultID: d.Vault, Path: p, To: to, UpdateLinks: &links})
	if err == nil {
		w.paths[to] = true
	}
	return ignoreBenign(err)
}

// diskMove renames a file on disk, as a file manager would.
func (w *World) diskMove(d *Node, i int, s Step) error {
	p := pick(d, s.A, false)
	if p == "" {
		return nil
	}
	to := folders[s.B%4] + movedName(i, p)
	full := filepath.Join(d.Dir, filepath.FromSlash(to))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(d.Dir, filepath.FromSlash(p)), full); err != nil {
		return nil // gone meanwhile
	}
	w.paths[to] = true
	if err := w.index(d, p); err != nil {
		return err
	}
	return w.index(d, to)
}

// attach writes one of three attachments that are not text: what it
// replaces is replaced on purpose.
func (w *World) attach(d *Node, s Step) error {
	p := fmt.Sprintf("files/att%d.bin", s.A%3)
	if w.o.Concurrent {
		p = fmt.Sprintf("files/att-%s.bin", w.oracle.token())
	}
	tok := w.oracle.token()
	old, had := files(d)[p]
	rng := rand.New(rand.NewPCG(s.B, 3))
	junk := make([]byte, 64+rng.IntN(512))
	for i := range junk {
		junk[i] = byte(rng.IntN(256))
	}
	if err := writeFile(d, p, "\x00\xff"+tok+"\xff"+string(junk), false); err != nil {
		return err
	}
	w.oracle.insert(tok)
	w.oracle.wrote(tok, d.Name, p+" (attach)")
	if had {
		w.oracle.remove(old)
	}
	w.paths[p] = true
	return w.index(d, p)
}

// crash arms a crash point on device d, makes a change that reaches it,
// and restarts the device once it fires.
func (w *World) crash(d *Node, s Step) error {
	point := []string{"renamed", "uploaded", "answered"}[s.A%3]
	if point != "renamed" {
		d.link.heal()
	}
	fired := d.crash.arm(point)
	e, err := w.edit(d, s, api.VersionSourceEditor)
	if err != nil && !strings.Contains(err.Error(), "crashed") {
		d.crash.disarm()
		return err
	}
	d.Srv.Syncer.Kick(d.Vault)
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		d.crash.disarm()
		return nil // the change never reached the point
	}
	d.stop()
	d.start(w.t)
	if err != nil && e.tok != "" && slices.Contains(tokenRE.FindAllString(files(d)[e.path], -1), e.tok) {
		// The write crashed after its rename: its bytes are on disk, and
		// so is the line it took out, if it took one out.
		w.oracle.insert(e.tok)
		w.oracle.wrote(e.tok, d.Name, e.path+" (crashed)")
		w.oracle.remove(e.removed)
	}
	return nil
}

// hubFault arms a fault on the hub, makes device d push a change (an
// edit, or with B's bit a create), and, for a crash, restarts the hub once
// it fires. A lost answer cuts d's link as it is lost, as a dropped
// connection would, so d changes on before it hears again.
func (w *World) hubFault(d *Node, i int, s Step, point string, restart bool) error {
	d.link.heal()
	var then func()
	if !restart {
		then = d.link.cut
	}
	fired := w.Hub.crash.armThen(point, then)
	var err error
	if (s.B>>20)%2 == 0 {
		err = w.create(d, fmt.Sprintf("%sn%d.md", folders[s.A%4], i), fmt.Sprintf("# n%d\n\n", i))
	} else {
		_, err = w.edit(d, s, api.VersionSourceEditor)
	}
	if err != nil {
		w.Hub.crash.disarm()
		return err
	}
	d.Srv.Syncer.Kick(d.Vault)
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		w.Hub.crash.disarm()
		return nil
	}
	if restart {
		w.Hub.stop()
		w.Hub.start(w.t)
	}
	return nil
}
