package synctest

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/store"
)

// Check lets the world go quiet and checks it (ADR-0020): every device's
// files are the hub's heads; every token no writer removed is in a head or
// a conflict revision; no file is a conflict copy; the log has no gaps,
// every base is earlier, and no two live files share a path ignoring case;
// every device agrees with the hub.
func (w *World) Check() error {
	if err := w.settle(); err != nil {
		return err
	}
	heads := files(w.Hub)
	for _, d := range w.Devices {
		if err := sameFiles(heads, files(d), d.Name); err != nil {
			return fmt.Errorf("%w%s", err, w.explain(d, heads, files(d)))
		}
	}
	if err := w.checkTokens(heads); err != nil {
		return err
	}
	for p := range heads {
		if !w.allowed(p) {
			return fmt.Errorf("%s is on the hub, but no step made it: a conflict copy", p)
		}
	}
	if err := w.checkLog(); err != nil {
		return err
	}
	for _, d := range w.Devices {
		if err := w.checkAgreed(d); err != nil {
			return err
		}
	}
	if p := w.o.Profile; p != nil && p.Check != nil {
		return p.Check(w)
	}
	return nil
}

func sameFiles(want, got map[string]string, name string) error {
	var diff []string
	for p, c := range want {
		if g, ok := got[p]; !ok || g != c {
			diff = append(diff, p)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			diff = append(diff, p)
		}
	}
	if len(diff) == 0 {
		return nil
	}
	slices.Sort(diff)
	p := diff[0]
	return fmt.Errorf("%s's vault differs from the hub's heads at %d paths, among them %q (hub %q, %s %q)",
		name, len(diff), diff[:min(5, len(diff))], short(want[p]), name, short(got[p]))
}

func short(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

var numbered = regexp.MustCompile(`^(.*) \d+(\.[^./]*)?$`)

// allowed reports whether a step made path p, or the protocol numbered
// such a path because it was taken ("name 1.md").
func (w *World) allowed(p string) bool {
	for {
		if w.paths[p] {
			return true
		}
		m := numbered.FindStringSubmatch(p)
		if m == nil {
			return false
		}
		p = m[1] + m[2]
	}
}

// hubLog lists every revision of the hub's vault, by sequence number.
func (w *World) hubLog(ctx context.Context) (store.HubVault, []store.Revision, error) {
	db := w.Hub.Srv.DB
	hv, err := db.HubVault(ctx, w.Hub.Vault)
	if err != nil {
		return store.HubVault{}, nil, err
	}
	revs := make([]store.Revision, 0, hv.Seq)
	for seq := int64(1); seq <= hv.Seq; seq++ {
		r, err := db.Revision(ctx, w.Hub.Vault, seq)
		if err != nil {
			return hv, nil, fmt.Errorf("the log has no revision %d of %d: %w", seq, hv.Seq, err)
		}
		revs = append(revs, r)
	}
	return hv, revs, nil
}

// checkTokens finds every token in the heads and the conflict revisions.
func (w *World) checkTokens(heads map[string]string) error {
	ctx := w.t.Context()
	present := map[string]bool{}
	for _, c := range heads {
		for _, tok := range tokenRE.FindAllString(c, -1) {
			present[tok] = true
		}
	}
	_, revs, err := w.hubLog(ctx)
	if err != nil {
		return err
	}
	for _, r := range revs {
		if r.Op != api.RevisionOpConflict {
			continue
		}
		data, err := w.Hub.Srv.Blobs.Get(r.Hash)
		if err != nil {
			return fmt.Errorf("conflict revision %d has no content: %w", r.Seq, err)
		}
		for _, tok := range tokenRE.FindAllString(string(data), -1) {
			present[tok] = true
		}
	}
	var lost []string
	for tok := range w.oracle.inserted {
		if !w.oracle.removed[tok] && !present[tok] {
			lost = append(lost, tok)
		}
	}
	if len(lost) > 0 {
		slices.Sort(lost)
		var where []string
		for _, tok := range lost[:min(3, len(lost))] {
			where = append(where, tok+", written "+w.oracle.origin[tok])
		}
		return fmt.Errorf("%d tokens no writer removed are in no head and no conflict revision: %s%s", len(lost),
			strings.Join(where, "; "), w.history(lost[0], revs))
	}
	return nil
}

// checkLog checks the hub's log: no gaps, every base earlier, and no two
// live files at one path ignoring case.
func (w *World) checkLog() error {
	ctx := w.t.Context()
	_, revs, err := w.hubLog(ctx)
	if err != nil {
		return err
	}
	for _, r := range revs {
		if r.Base >= r.Seq {
			return fmt.Errorf("revision %d is based on %d", r.Seq, r.Base)
		}
	}
	all, err := w.hubFiles(ctx)
	if err != nil {
		return err
	}
	seen := map[string]string{}
	for _, f := range all {
		if f.Deleted {
			continue
		}
		key := strings.ToLower(f.Path)
		if other, ok := seen[key]; ok {
			return fmt.Errorf("two live files share a path: %s and %s", other, f.Path)
		}
		seen[key] = f.Path
	}
	return nil
}

// hubFiles is every file of the hub's log, deleted ones included, page by
// page.
func (w *World) hubFiles(ctx context.Context) ([]api.LogFile, error) {
	var all []api.LogFile
	after := int64(0)
	for {
		page, err := w.Hub.Srv.DB.Pull(ctx, w.Hub.Vault, after, 1000)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Files...)
		if !page.More {
			return all, nil
		}
		after = page.Next
	}
}

// checkAgreed checks that device d agrees with the hub on every file: its
// agreed state is the head, and its file holds it.
func (w *World) checkAgreed(d *Node) error {
	ctx := w.t.Context()
	all, err := w.hubFiles(ctx)
	if err != nil {
		return err
	}
	rows, err := d.Srv.DB.AgreedRows(ctx, d.Vault)
	if err != nil {
		return err
	}
	agreed := map[int64]store.Agreed{}
	for _, a := range rows {
		agreed[a.HubFile] = a
	}
	for _, f := range all {
		a, ok := agreed[f.File]
		switch {
		case f.Deleted && ok:
			return fmt.Errorf("%s agrees with %s, which the hub deleted", d.Name, a.Path)
		case f.Deleted:
		case !ok:
			return fmt.Errorf("%s has no agreed state for %s", d.Name, f.Path)
		case a.Path != f.Path || a.Hash != f.Hash || a.Revision != f.Revision:
			return fmt.Errorf("%s agrees %s at %d (%.8s), the hub's head is %s at %d (%.8s)", d.Name, a.Path, a.Revision,
				a.Hash, f.Path, f.Revision, f.Hash)
		}
		delete(agreed, f.File)
	}
	for _, a := range agreed {
		return fmt.Errorf("%s agrees with hub file %d at %s, which the hub does not have", d.Name, a.HubFile, a.Path)
	}
	return nil
}

// CheckIndexes checks that every node's index equals a fresh rebuild of
// its vault: the last check of a run, since a rebuild costs a notesd.
func (w *World) CheckIndexes() error {
	for _, n := range append([]*Node{w.Hub}, w.Devices...) {
		got, err := n.Srv.DB.IndexSnapshot(w.t.Context(), n.Vault)
		if err != nil {
			return err
		}
		fresh := rpctest.Start(w.t)
		c := fresh.Dial(w.t)
		v, err := c.Vault().Add(w.t.Context(), &api.VaultAddParams{Path: n.Dir})
		if err != nil {
			return err
		}
		for p := range files(n) {
			if _, err := c.Note().Read(w.t.Context(), &api.NoteReadParams{VaultID: v.ID, Path: p}); err != nil && !benign(err) {
				return err
			}
		}
		want, err := fresh.DB.IndexSnapshot(w.t.Context(), v.ID)
		fresh.Stop()
		if err != nil {
			return err
		}
		if !slices.Equal(got, want) {
			for _, row := range got {
				if !slices.Contains(want, row) {
					return fmt.Errorf("%s's index has a row a rebuild lacks: %.200s", n.Name, row)
				}
			}
			for _, row := range want {
				if !slices.Contains(got, row) {
					return fmt.Errorf("%s's index lacks a row a rebuild has: %.200s", n.Name, row)
				}
			}
		}
	}
	return nil
}

// explain describes, for a failed check, where device d and the hub
// stand on each path where they differ: the index rows, the agreed rows
// and the hub's files.
func (w *World) explain(d *Node, want, got map[string]string) string {
	ctx := w.t.Context()
	var diff []string
	for p, c := range want {
		if g, ok := got[p]; !ok || g != c {
			diff = append(diff, p)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			diff = append(diff, p)
		}
	}
	slices.Sort(diff)
	rows, _ := d.Srv.DB.AgreedRows(ctx, d.Vault)
	all, _ := w.hubFiles(ctx)
	out := ""
	for _, p := range diff[:min(4, len(diff))] {
		out += "\n  " + p + ":"
		if f, err := d.Srv.DB.FileByPath(ctx, d.Vault, p); err == nil {
			out += fmt.Sprintf("\n    %s's index: file %d, hash %.8s", d.Name, f.ID, f.Hash)
			if a, err := d.Srv.DB.AgreedByFile(ctx, f.ID); err == nil {
				out += fmt.Sprintf("\n    agreed by file: %+v", a)
			} else {
				out += fmt.Sprintf("\n    agreed by file: %v", err)
			}
		} else {
			out += fmt.Sprintf("\n    %s's index: %v", d.Name, err)
		}
		for _, a := range rows {
			if a.Path == p {
				out += fmt.Sprintf("\n    agreed at the path: %+v", a)
			}
		}
		for _, f := range all {
			if f.Path == p {
				out += fmt.Sprintf("\n    the hub's file: %d, seq %d, head %d, deleted %v, hash %.8s", f.File, f.Seq, f.Revision, f.Deleted, f.Hash)
			}
		}
	}
	if _, revs, err := w.hubLog(ctx); err == nil {
		for _, p := range diff[:min(2, len(diff))] {
			files := map[int64]bool{}
			for _, r := range revs {
				if strings.EqualFold(r.Path, p) {
					files[r.FileID] = true
				}
			}
			for _, r := range revs {
				if files[r.FileID] {
					out += fmt.Sprintf("\n  hub rev %d file %d %s %s %.8s base %d device %d push %d", r.Seq, r.FileID, r.Op, r.Path,
						r.Hash, r.Base, r.DeviceID, r.PushID)
				}
			}
		}
	}
	if ps, err := d.Srv.DB.Pushes(ctx, d.Vault); err == nil {
		for _, lp := range ps {
			if slices.ContainsFunc(diff[:min(2, len(diff))], func(p string) bool { return strings.EqualFold(p, lp.Path) }) {
				out += fmt.Sprintf("\n  %s push %d file %d hub %d base %d %s %.8s: %s %s", d.Name, lp.PushID, lp.FileID, lp.HubFile,
					lp.Base, lp.Path, lp.Hash, lp.Status, lp.Reason)
			}
		}
	}
	if st, err := d.C.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &d.Vault}); err == nil {
		out += fmt.Sprintf("\n  %s's status: %+v", d.Name, st[0])
	}
	return out
}

// history lists, for a lost token, the hub's revisions whose content held
// it and the revisions of the files those were.
func (w *World) history(tok string, revs []store.Revision) string {
	held := map[int64]bool{}
	files := map[int64]bool{}
	for _, r := range revs {
		if r.Hash == "" {
			continue
		}
		if data, err := w.Hub.Srv.Blobs.Get(r.Hash); err == nil && slices.Contains(tokenRE.FindAllString(string(data), -1), tok) {
			held[r.Seq], files[r.FileID] = true, true
		}
	}
	out := ""
	for _, r := range revs {
		if !files[r.FileID] {
			continue
		}
		mark := " "
		if held[r.Seq] {
			mark = "*"
		}
		out += fmt.Sprintf("\n  %s rev %d file %d %s %s %.8s base %d device %d push %d lost_to %d %s", mark, r.Seq, r.FileID, r.Op,
			r.Path, r.Hash, r.Base, r.DeviceID, r.PushID, r.LostTo, w.tokensOf(w.Hub, r.Hash))
	}
	if out == "" {
		out = "\n  no revision on the hub ever held it"
	}
	for _, t := range w.oracle.tried[tok] {
		out += "\n  a failed write took it out: " + t
	}
	// Each device's local versions of those files, and of the file at the
	// token's path, oldest first.
	ctx := w.t.Context()
	for _, d := range w.Devices {
		local := map[int64]string{}
		for f := range files {
			if a, err := d.Srv.DB.AgreedByHubFile(ctx, d.Vault, f); err == nil {
				local[a.FileID] = fmt.Sprintf("hub %d", f)
			}
		}
		if row, err := d.Srv.DB.FileByPath(ctx, d.Vault, w.oracle.at[tok]); err == nil {
			if _, ok := local[row.ID]; !ok {
				local[row.ID] = "at " + w.oracle.at[tok]
			}
		}
		for id, why := range local {
			vs, err := d.Srv.DB.Versions(ctx, id, 50)
			if err != nil {
				continue
			}
			out += fmt.Sprintf("\n  %s's file %d (%s):", d.Name, id, why)
			for i := len(vs) - 1; i >= 0; i-- {
				v := vs[i]
				mark := " "
				if data, err := d.Srv.Blobs.Get(v.Hash); err == nil && slices.Contains(tokenRE.FindAllString(string(data), -1), tok) {
					mark = "*"
				}
				out += fmt.Sprintf("\n    %s v%d %s %.8s %s %s conflict %v at %s %s", mark, v.ID, v.Path, v.Hash, v.Source, v.Author,
					v.Conflict, v.At.Format("15:04:05.000"), w.tokensOf(d, v.Hash))
			}
		}
	}
	return out
}

// tokensOf lists the tokens in node n's blob hash, for a report.
func (w *World) tokensOf(n *Node, hash string) string {
	if hash == "" {
		return ""
	}
	data, err := n.Srv.Blobs.Get(hash)
	if err != nil {
		return "(no content)"
	}
	return fmt.Sprint(tokenRE.FindAllString(string(data), -1))
}
