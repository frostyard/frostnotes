//go:build !race

package syncer_test

import "time"

// generatedNotes is the user's vault's size (plan 0004, Phase 4).
const generatedNotes = 666

// settleWait is how long settle waits for a hub and its devices to go
// quiet.
const settleWait = 30 * time.Second
