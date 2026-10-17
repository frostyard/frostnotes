package syncer_test

import "testing"

func TestAnAnswerLeavesAParkedFileForTheFollowingPull(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"x.md": "X\n", "y.md": "Y\n"})
	b.write("x.md", "X\nfrom b\n")
	b.srv.Syncer.Kick(b.vault)
	settle(t, h, a.node, b.node)
	b.net.cut()
	inOrder(t, h, []*device{a}, func() {
		a.move("x.md", "t.md")
		a.move("y.md", "x.md")
		a.move("t.md", "y.md")
	})
	// Only y is pending on b. Its answer moves it onto x, parking the
	// unchanged x until the following pull supplies x's new head at y.
	b.write("y.md", "Y\nfrom b\n")
	b.net.heal()
	b.srv.Syncer.Kick(b.vault)
	settle(t, h, a.node, b.node)
	got := same(t, h, a.node, b.node)
	if len(got) != 2 || got["x.md"] != "Y\nfrom b\n" || got["y.md"] != "X\nfrom b\n" {
		t.Fatalf("after an answer and a pull = %q, want both swapped names with b's edits", got)
	}
}
