package syncer_test

import (
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// The hub answered a push and the device's session ended before it applied
// the answer: the answer is still recorded, so the device does not send the
// change again as a new push (a second create of a merged file would make a
// second file on the hub). The loop's context ends at the hook, so the
// answer, the agreed row and everything after them run on a context that
// is already cancelled.
func TestAnAnswerIsRecordedWhenTheSessionEndsAfterTheHubAnswered(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": "one\ntwo\nthree\n"})
	var (
		armed atomic.Bool
		a     *node
	)
	o := deviceOptions("a")
	o.Crash = func(point string) bool {
		if point != "answered" || !armed.CompareAndSwap(true, false) {
			return false
		}
		a.srv.Syncer.CancelLoop(a.vault) // the session ends with the hub's answers in hand
		return false
	}
	a = pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "a")
	settle(t, h, a)
	ctx := t.Context()
	armed.Store(true)
	a.write("n.md", "one\ntwo, edited\nthree\n")
	// The loop ends after the answer was applied or lost; the API server
	// keeps running, so the store and the vault can be read.
	a.srv.Syncer.WaitLoop(a.vault)
	if left, err := a.srv.DB.Unanswered(ctx, a.vault); err != nil || len(left) != 0 {
		t.Fatalf("unanswered pushes = %+v, %v; want none: the hub's answer is recorded", left, err)
	}
	sent, err := a.srv.DB.Pushes(ctx, a.vault)
	if err != nil || len(sent) != 1 || sent[0].Status != api.PushStatusAccepted {
		t.Fatalf("answered pushes = %+v, %v; want one accepted push", sent, err)
	}
	note, err := a.c.Note().Read(ctx, &api.NoteReadParams{VaultID: a.vault, Path: "n.md"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := a.srv.DB.AgreedRows(ctx, a.vault)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Path != "n.md" || rows[0].Hash != note.Hash || rows[0].FileID != sent[0].FileID {
		t.Fatalf("agreed rows = %+v; want n.md agreed with the answer's head at the file it pushed", rows)
	}
}
