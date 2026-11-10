package notes

import "time"

// History is a document's undo history of the user's own edits, as
// CodeMirror's: another writer's change and a merge never enter it, and
// the steps in it move over them, so undo takes back only what the user
// typed (ADR-0022). MyGo's editor buffer keeps every edit in its own
// history, so the app keeps this one and binds undo and redo to it.
type History struct {
	undo, redo []step
	// last is when the newest step was added to, and lastTo where its
	// change ended, for joining typing into one step.
	last   time.Time
	lastTo int
	joinOK bool
}

// step is one step of undo: inv applies to the text as it is now (for the
// top step) and makes the text before the step.
type step struct{ inv ChangeSet }

const (
	// joinDelay joins edits closer than this, at adjacent places, into
	// one step, as CodeMirror's newGroupDelay.
	joinDelay = 500 * time.Millisecond
	// historyDepth is how many steps are kept.
	historyDepth = 200
)

// Record adds a user's edit; before is the text cs applied to.
func (h *History) Record(cs ChangeSet, before string, at time.Time) {
	if cs.Empty() {
		return
	}
	h.redo = nil
	inv := cs.Invert(before)
	changes := cs.Changes()
	first := changes[0]
	if h.joinOK && len(h.undo) > 0 && at.Sub(h.last) < joinDelay && first.From <= h.lastTo && h.lastTo <= first.To {
		top := &h.undo[len(h.undo)-1]
		// The new inverse applies after cs; the top's applied before it.
		if joined, err := inv.Compose(top.inv); err == nil {
			top.inv = joined
			h.last, h.lastTo = at, cs.MapPos(changes[len(changes)-1].To, 1)
			return
		}
	}
	h.undo = append(h.undo, step{inv: inv})
	if len(h.undo) > historyDepth {
		h.undo = h.undo[len(h.undo)-historyDepth:]
	}
	h.last, h.lastTo, h.joinOK = at, cs.MapPos(changes[len(changes)-1].To, 1), len(changes) == 1
}

// Remote moves the steps over another writer's change, which applies to
// the text as it is now.
func (h *History) Remote(cs ChangeSet) {
	if cs.Empty() {
		return
	}
	h.undo = mapSteps(h.undo, cs)
	h.redo = mapSteps(h.redo, cs)
	h.joinOK = false
}

// mapSteps moves a stack of steps over m: the top step applies to the
// text m applies to; each step below applies to the text the one above
// makes, so m is moved over each step on the way down.
func mapSteps(steps []step, m ChangeSet) []step {
	out := make([]step, len(steps))
	for i := len(steps) - 1; i >= 0; i-- {
		inv := steps[i].inv
		mapped, err := inv.Map(m, false)
		if err != nil {
			return out[i+1:] // the steps below no longer apply
		}
		next, err := m.Map(inv, true)
		if err != nil {
			out[i] = step{inv: mapped}
			return out[i:]
		}
		out[i] = step{inv: mapped}
		m = next
	}
	return out
}

// CanUndo and CanRedo report whether there is a step to take.
func (h *History) CanUndo() bool { return len(h.undo) > 0 }
func (h *History) CanRedo() bool { return len(h.redo) > 0 }

// Undo returns the change that takes back the newest step, applying to
// text, and moves the step to redo.
func (h *History) Undo(text string) (ChangeSet, bool) {
	return h.take(&h.undo, &h.redo, text)
}

// Redo returns the change that makes the newest undone step again.
func (h *History) Redo(text string) (ChangeSet, bool) {
	return h.take(&h.redo, &h.undo, text)
}

func (h *History) take(from, to *[]step, text string) (ChangeSet, bool) {
	for len(*from) > 0 {
		s := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		if s.inv.Empty() || s.inv.Len() != len(text) {
			continue
		}
		*to = append(*to, step{inv: s.inv.Invert(text)})
		h.joinOK = false
		return s.inv, true
	}
	return ChangeSet{}, false
}
