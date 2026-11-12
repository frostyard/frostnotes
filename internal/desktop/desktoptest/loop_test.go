package desktoptest

import "testing"

// Until checks its condition once for each batch of posted functions, as
// the UI thread draws one frame for what it ran: a condition that draws a
// frame must not draw one for each answer (#203).
func TestUntilChecksOnceForWhatWaits(t *testing.T) {
	l := NewLoop()
	ran := 0
	for range 5 {
		l.Post(func() { ran++ })
	}
	checks := 0
	l.Until(t, "the posted functions", func() bool {
		checks++
		return ran == 5
	})
	if checks != 2 {
		t.Fatalf("the condition ran %d times for 1 batch, want 2: before and after it", checks)
	}
}

// A function that posts itself again does not keep Until past its limit.
func TestUntilRunsOnlyWhatWaitsBeforeItChecks(t *testing.T) {
	l := NewLoop()
	runs := 0
	var again func()
	again = func() {
		runs++
		l.Post(again)
	}
	l.Post(again)
	l.Until(t, "a second run", func() bool { return runs >= 2 })
	if runs != 2 {
		t.Fatalf("%d runs before the check, want 2", runs)
	}
}
