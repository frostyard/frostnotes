package workspace

import (
	"encoding/json/v2"
	"testing"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
)

func TestValidateKeepsWhatItUnderstands(t *testing.T) {
	if w := Validate([]byte("garbage")); w.Root.Tabs[0].View.Type != ViewEmpty {
		t.Error("garbage made no fresh workspace")
	}
	if w := Validate([]byte(`{"version":2}`)); w.Version != Version || w.Root == nil {
		t.Error("an unknown version")
	}
	w := Validate([]byte(`{"version":1,"root":{"kind":"split","id":"s1","direction":"row","sizes":[3,1],"children":[
		{"kind":"tabs","id":"g2","active":9,"tabs":[{"id":"t3","view":{"type":"note","path":"a.md","mode":"read"}},{"id":"t3","view":{"type":"empty"}},{"id":"t4","view":{"type":"weird"}}]},
		{"kind":"tabs","id":"g5","tabs":[{"id":"t6","view":{"type":"merge","path":"a.md","other":{"kind":"held"}}}]}]},
		"activeGroup":"g9","recent":["a.md","a.md","/abs"],"left":{"panes":["search"],"active":0,"width":9999,"open":false},"files":{"sort":"nope"},"nextId":1}`))
	if w.Root.Kind != KindTabs || w.Root.ID != "g2" || len(w.Root.Tabs) != 1 || w.Root.Active != 0 {
		t.Fatalf("root %+v", w.Root)
	}
	if w.Root.Tabs[0].View.Mode != ModeRead || w.ActiveID != "g2" || w.NextID != 4 {
		t.Errorf("%+v next %d", w.Root.Tabs[0], w.NextID)
	}
	eq(t, "recent", w.Recent, []string{"a.md"})
	if w.Left.Panes[w.Left.Active] != "search" || w.Left.Width != SidebarMax || w.Left.Open || w.Files.Sort != "alphabetical" {
		t.Errorf("%+v %+v", w.Left, w.Files)
	}
}

func TestARoundTrip(t *testing.T) {
	w := with("a.md", "b.md")
	w.Open(note("c.md"), ToRight)
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	got := Validate(data)
	if len(Groups(got.Root)) != 2 || got.ActiveTab().View != note("c.md") || got.NextID != w.NextID {
		t.Errorf("%+v", got)
	}
}

func TestTheStore(t *testing.T) {
	loop := desktoptest.NewLoop()
	dir := t.TempDir()
	s := NewStore(loop, dir)
	s.Load("/vault/one")
	s.Update(func(w *Workspace) { w.Open(note("a.md"), Here) })
	loop.Advance(SaveDelay)
	if s.Err != nil {
		t.Fatal(s.Err)
	}

	again := NewStore(loop, dir)
	again.Load("/vault/one")
	if again.Workspace().ActiveTab().View != note("a.md") {
		t.Fatalf("not stored: %+v", again.Workspace().ActiveTab())
	}

	// A change to a vault not shown lands in its file.
	s.Load("/vault/two")
	s.Follow("/vault/one", func(w *Workspace) { w.ApplyMove("a.md", "b.md") })
	s.Load("/vault/one")
	if s.Workspace().ActiveTab().View != note("b.md") {
		t.Errorf("not followed: %+v", s.Workspace().ActiveTab())
	}
	s.Update(func(w *Workspace) { w.Open(note("c.md"), Here) })
	s.Reset()
	if s.Workspace().ActiveTab().View.Type != ViewEmpty {
		t.Error("reset")
	}
	loop.Advance(SaveDelay)
	fresh := NewStore(loop, dir)
	fresh.Load("/vault/one")
	if fresh.Workspace().ActiveTab().View.Type != ViewEmpty {
		t.Error("reset left the file")
	}
}
