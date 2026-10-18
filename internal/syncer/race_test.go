//go:build race

package syncer_test

import "time"

// generatedNotes is smaller under the race detector, where the hub's
// fsynced creates of 666 notes take over a minute.
const generatedNotes = 150

// settleWait is how long settle waits for a hub and its devices to go
// quiet. The race detector slows sync several times over, and CI's
// runners vary: 30 s ran out on one (PR #25), so it has room here.
const settleWait = 2 * time.Minute
