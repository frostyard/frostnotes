package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/system"
)

const (
	// SaveDelay is how long after the last change the workspace is written.
	SaveDelay = 500 * time.Millisecond
	// SaveRetry is how long after a failed write it is tried again.
	SaveRetry = 5 * time.Second
)

var (
	idRe  = regexp.MustCompile(`^[tgs][0-9]+$`)
	tabRe = regexp.MustCompile(`^t[0-9]+$`)
	sorts = []string{"alphabetical", "alphabeticalReverse", "byModifiedTime", "byModifiedTimeReverse"}
)

func isPath(p string) bool { return p != "" && !strings.HasPrefix(p, "/") }

// Validate reads a stored workspace, keeping what it understands and
// dropping the rest, so a stored layout never wedges the window; it
// returns a new workspace when nothing is left.
func Validate(data []byte) *Workspace {
	var in Workspace
	if json.Unmarshal(data, &in) != nil || in.Version != Version {
		return New()
	}
	ids := map[string]bool{}
	root := validNode(in.Root, ids)
	if root == nil {
		return New()
	}
	out := New()
	out.Root = root
	all := Groups(root)
	out.ActiveID = all[0].ID
	if slices.ContainsFunc(all, func(g *Node) bool { return g.ID == in.ActiveID }) {
		out.ActiveID = in.ActiveID
	}
	closed := in.Closed
	if len(closed) > MaxClosed {
		closed = closed[len(closed)-MaxClosed:]
	}
	for _, c := range closed {
		if t, ok := validTab(c.Tab, ids); ok && c.Group != "" && c.Index >= 0 {
			out.Closed = append(out.Closed, ClosedTab{Tab: t, Group: c.Group, Index: c.Index})
		}
	}
	out.Recent = uniquePaths(in.Recent, MaxRecent)
	out.Left = validSidebar(in.Left, SideLeft, LeftWidth)
	out.Right = validSidebar(in.Right, SideRight, RightWidth)
	if slices.Contains(sorts, in.Files.Sort) {
		out.Files.Sort = in.Files.Sort
	}
	out.Files.Expanded = uniquePaths(in.Files.Expanded, MaxExpanded)
	highest := 0
	for id := range ids {
		if n, err := strconv.Atoi(id[1:]); err == nil {
			highest = max(highest, n)
		}
	}
	out.NextID = max(in.NextID, highest+1)
	return out
}

func uniquePaths(in []string, limit int) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range in {
		if isPath(p) && !seen[p] && len(out) < limit {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func validNode(n *Node, ids map[string]bool) *Node {
	if n == nil || !idRe.MatchString(n.ID) || ids[n.ID] {
		return nil
	}
	switch n.Kind {
	case KindTabs:
		if n.ID[0] != 'g' {
			return nil
		}
		var tabs []Tab
		for _, t := range n.Tabs {
			if v, ok := validTab(t, ids); ok {
				tabs = append(tabs, v)
			}
		}
		if len(tabs) == 0 {
			return nil
		}
		ids[n.ID] = true
		active := n.Active
		if active < 0 || active >= len(tabs) {
			active = 0
		}
		return &Node{Kind: KindTabs, ID: n.ID, Tabs: tabs, Active: active}
	case KindSplit:
		if n.ID[0] != 's' || n.Direction != Row && n.Direction != Column {
			return nil
		}
		ids[n.ID] = true
		var children []*Node
		var sizes []float64
		for i, c := range n.Children {
			child := validNode(c, ids)
			if child == nil {
				continue
			}
			size := 0.0
			if i < len(n.Sizes) && n.Sizes[i] > 0 {
				size = n.Sizes[i]
			}
			children, sizes = append(children, child), append(sizes, size)
		}
		switch len(children) {
		case 0:
			return nil
		case 1:
			return children[0]
		}
		sum := 0.0
		for i := range sizes {
			if sizes[i] <= 0 {
				sizes[i] = 1 / float64(len(sizes))
			}
			sum += sizes[i]
		}
		for i := range sizes {
			sizes[i] /= sum
		}
		return &Node{Kind: KindSplit, ID: n.ID, Direction: n.Direction, Children: children, Sizes: sizes}
	}
	return nil
}

func validTab(t Tab, ids map[string]bool) (Tab, bool) {
	if !tabRe.MatchString(t.ID) || ids[t.ID] {
		return Tab{}, false
	}
	v, ok := validView(t.View)
	if !ok {
		return Tab{}, false
	}
	ids[t.ID] = true
	stack := func(in []View) []View {
		var out []View
		for _, x := range in {
			if y, ok := validView(x); ok {
				out = append(out, y)
			}
		}
		return capped(out)
	}
	return Tab{ID: t.ID, View: v, Back: stack(t.Back), Forward: stack(t.Forward), Pinned: t.Pinned}, true
}

func validView(v View) (View, bool) {
	switch v.Type {
	case ViewEmpty:
		return Empty(), true
	case ViewNote:
		if !isPath(v.Path) {
			return View{}, false
		}
		out := Note(v.Path, ModeEdit)
		if v.Mode == ModeRead {
			out.Mode = ModeRead
		}
		out.Scroll, out.Cursor = max(0, v.Scroll), max(0, v.Cursor)
		return out, true
	case ViewHistory, ViewFile:
		return View{Type: v.Type, Path: v.Path}, isPath(v.Path)
	case ViewMerge:
		// A held text lives in memory only: a stored one is dropped.
		if !isPath(v.Path) || v.Other == nil || v.Other.Kind != "version" && v.Other.Kind != "revision" {
			return View{}, false
		}
		return View{Type: ViewMerge, Path: v.Path, Other: v.Other}, true
	}
	return View{}, false
}

func validSidebar(s Sidebar, side string, width float64) Sidebar {
	all := Panes[side]
	out := Sidebar{Panes: slices.Clone(all), Width: width, Open: s.Open}
	if s.Active >= 0 && s.Active < len(s.Panes) {
		out.Active = max(0, slices.Index(all, s.Panes[s.Active]))
	}
	if s.Width > 0 {
		out.Width = max(SidebarMin, min(SidebarMax, s.Width))
	}
	return out
}

// Store is the window's vault's workspace, kept in a file per vault and
// written SaveDelay after the last change (ADR-0021). UI thread only.
type Store struct {
	env   notes.Env
	dir   string
	vault string
	ws    *Workspace
	stop  func()
	// Err is the last write's failure, nil when it worked.
	Err error
	// OnError is told of a new failure.
	OnError func(error)

	// unwritten holds, by vault, a workspace the window has left whose last
	// write failed, until a retry lands it.
	unwritten map[string]*Workspace
	retry     func() // stops the timer of the next retry
}

// NewStore keeps workspaces in dir.
func NewStore(env notes.Env, dir string) *Store {
	return &Store{env: env, dir: dir, ws: New(), unwritten: map[string]*Workspace{}}
}

func (s *Store) file(vaultPath string) string {
	sum := sha256.Sum256([]byte(vaultPath))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+".json")
}

func (s *Store) read(vaultPath string) *Workspace {
	data, err := os.ReadFile(s.file(vaultPath))
	if err != nil {
		return New()
	}
	return Validate(data)
}

func (s *Store) write(vaultPath string, w *Workspace) error {
	data, err := json.Marshal(w)
	if err != nil {
		return fmt.Errorf("encode the workspace: %w", err)
	}
	return system.WriteFile(s.file(vaultPath), data)
}

// Workspace is the window's workspace.
func (s *Store) Workspace() *Workspace { return s.ws }

// Vault is the path of the vault it belongs to, "" for none.
func (s *Store) Vault() string { return s.vault }

// Load shows a vault's stored workspace, writing the one shown before.
func (s *Store) Load(vaultPath string) {
	if vaultPath == s.vault {
		return
	}
	s.Flush()
	if s.stop != nil { // the write failed, and the retry would write the next vault's
		s.stop()
		s.stop = nil
		s.unwritten[s.vault] = s.ws
		s.scheduleRetry()
	}
	s.vault = vaultPath
	if vaultPath == "" {
		s.ws = New()
		return
	}
	if w := s.unwritten[vaultPath]; w != nil {
		delete(s.unwritten, vaultPath)
		s.ws = w
		s.stop = s.env.After(SaveDelay, s.Flush)
		return
	}
	s.ws = s.read(vaultPath)
}

func (s *Store) scheduleRetry() {
	if s.retry != nil {
		s.retry()
	}
	s.retry = s.env.After(SaveRetry, s.writeUnwritten)
}

// writeUnwritten tries again the workspaces of vaults the window left.
func (s *Store) writeUnwritten() {
	if s.retry != nil {
		s.retry()
		s.retry = nil
	}
	for p, w := range s.unwritten {
		err := s.write(p, w)
		s.record(err)
		if err == nil {
			delete(s.unwritten, p)
		}
	}
	if len(s.unwritten) > 0 {
		s.scheduleRetry()
	}
}

// Update applies an operation and writes the result soon.
func (s *Store) Update(op func(*Workspace)) {
	op(s.ws)
	if s.stop != nil {
		s.stop()
	}
	s.stop = s.env.After(SaveDelay, s.Flush)
}

// Flush writes a pending change now.
func (s *Store) Flush() {
	s.writeUnwritten()
	if s.stop == nil {
		return
	}
	s.stop()
	s.stop = nil
	if s.vault != "" {
		s.record(s.write(s.vault, s.ws))
		if s.Err != nil {
			s.stop = s.env.After(SaveRetry, s.Flush)
		}
	}
}

// record keeps the latest write's error, and reports a new one.
func (s *Store) record(err error) {
	changed := (err == nil) != (s.Err == nil) || err != nil && err.Error() != s.Err.Error()
	s.Err = err
	if changed && err != nil && s.OnError != nil {
		s.OnError(err)
	}
}

// Reset forgets the window's vault's layout (Reset workspace layout).
func (s *Store) Reset() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	if s.vault != "" {
		_ = os.Remove(s.file(s.vault))
	}
	next := s.ws.NextID
	s.ws = New()
	// A new ID, or the window would keep showing the old tab that had it.
	s.ws.NextID = max(s.ws.NextID, next)
	s.ws.Root.Tabs[0].ID = s.ws.takeID("t")
}

// Follow applies a move or delete to the workspace of the vault it
// happened in: the window's when that vault is shown, else the stored one,
// so tabs follow what happened while another vault was shown.
func (s *Store) Follow(vaultPath string, op func(*Workspace)) {
	if vaultPath == "" {
		return
	}
	if vaultPath == s.vault {
		// At once: the event's sequence is kept, so it will not be replayed.
		s.Update(op)
		s.Flush()
		return
	}
	if w := s.unwritten[vaultPath]; w != nil { // newer than its file
		op(w)
		s.writeUnwritten()
		return
	}
	data, err := os.ReadFile(s.file(vaultPath))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.record(err)
		return
	}
	if !readable(data) {
		return // not ours to overwrite
	}
	w := Validate(data)
	op(w)
	s.record(s.write(vaultPath, w))
}

// readable reports whether a stored workspace can be read at all.
func readable(data []byte) bool {
	var in Workspace
	return json.Unmarshal(data, &in) == nil && in.Version == Version
}
