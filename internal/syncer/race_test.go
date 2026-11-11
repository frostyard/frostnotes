//go:build race

package syncer_test

// generatedNotes is smaller under the race detector, where the hub's
// fsynced creates of 666 notes take over a minute.
const generatedNotes = 150
