package editor

import (
	"testing"
	"time"
)

// TestTypingBudgets holds the typing gate's two values apart: the local gate
// stays at 16 ms, CI's is a separate, looser value, and only CI=true selects
// it (ADR-0043). A change that moved CI's limit into the local gate, or made
// the two one value, fails here.
func TestTypingBudgets(t *testing.T) {
	if localTypingBudget != 16*time.Millisecond {
		t.Errorf("localTypingBudget = %v, want 16ms (plan 0009, Phases 2 and 5)", localTypingBudget)
	}
	if ciTypingBudget <= localTypingBudget {
		t.Errorf("ciTypingBudget = %v, want more than localTypingBudget (%v)", ciTypingBudget, localTypingBudget)
	}
	for _, tc := range []struct {
		ci   string
		want time.Duration
	}{
		{"", localTypingBudget},
		{"false", localTypingBudget},
		{"0", localTypingBudget},
		{"yes", localTypingBudget},
		{"true", ciTypingBudget},
		{"1", ciTypingBudget},
	} {
		getenv := func(k string) string {
			if k == "CI" {
				return tc.ci
			}
			return ""
		}
		if got := typingBudget(getenv); got != tc.want {
			t.Errorf("typingBudget with CI=%q = %v, want %v", tc.ci, got, tc.want)
		}
	}
}
