// Package desktoptest holds what the desktop app's tests share: a loop
// that stands in for the UI thread, with a clock that moves only when a
// test advances it, and a real notesd with a vault (internal/rpctest).
package desktoptest

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// Epoch is the loop's time zero.
var Epoch = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

// Loop stands in for the UI thread: workers Post to it from any goroutine,
// and the test runs what they posted on its own goroutine. Its clock
// moves only when the test calls Advance.
type Loop struct {
	posted chan func()
	mu     sync.Mutex
	now    time.Duration
	timers []*timer
}

type timer struct {
	at      time.Duration
	f       func()
	stopped bool
}

// NewLoop makes a loop.
func NewLoop() *Loop { return &Loop{posted: make(chan func(), 256)} }

// Post runs f on the test's goroutine, at its next Until or Drain.
func (l *Loop) Post(f func()) { l.posted <- f }

// After runs f on the test's goroutine once Advance moves the clock d on.
func (l *Loop) After(d time.Duration, f func()) func() {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := &timer{at: l.now + d, f: f}
	l.timers = append(l.timers, t)
	return func() {
		l.mu.Lock()
		t.stopped = true
		l.mu.Unlock()
	}
}

// Now is the loop's time.
func (l *Loop) Now() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Epoch.Add(l.now)
}

// Advance moves the clock by d and runs the timers that come due, in
// order.
func (l *Loop) Advance(d time.Duration) {
	l.mu.Lock()
	l.now += d
	var due []*timer
	keep := l.timers[:0]
	for _, t := range l.timers {
		if t.at <= l.now {
			due = append(due, t)
		} else {
			keep = append(keep, t)
		}
	}
	l.timers = keep
	l.mu.Unlock()
	slices.SortStableFunc(due, func(a, b *timer) int { return int(a.at - b.at) })
	for _, t := range due {
		l.mu.Lock()
		stopped := t.stopped
		l.mu.Unlock()
		if !stopped {
			t.f()
		}
	}
}

// Until runs what workers post until cond holds, for at most 10 s.
//
// Like the UI thread, which runs everything posted and then draws one
// frame, it runs every function waiting in the queue before it checks
// cond again. A cond that draws a frame thus draws one for each batch, not
// one for each function: when frames are slow (the race detector, a busy
// runner), more work waits, and one frame covers it all. A frame for each
// function made the wait grow with the frames' cost times the number of
// answers, past the limit on a slow runner.
func (l *Loop) Until(t testing.TB, what string, cond func() bool) {
	t.Helper()
	l.UntilWithin(t, what, 10*time.Second, cond)
}

// UntilWithin is Until with its own limit.
func (l *Loop) UntilWithin(t testing.TB, what string, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.After(limit)
	for !cond() {
		select {
		case f := <-l.posted:
			f()
			l.runWaiting()
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// runWaiting runs the functions in the queue now, but not those they
// post: a function that posts itself again cannot keep Until from its
// deadline.
func (l *Loop) runWaiting() {
	for range len(l.posted) {
		(<-l.posted)()
	}
}

// Drain runs what has been posted so far, without waiting for more.
func (l *Loop) Drain() {
	for {
		select {
		case f := <-l.posted:
			f()
		default:
			return
		}
	}
}

// Notesd is a real notesd with one vault, and two connections: the app's
// and another writer's.
type Notesd struct {
	T     testing.TB
	Srv   *rpctest.Server
	App   *api.Client
	Other *api.Client
	Vault int64
	Dir   string
	// Blobs is notesd's blob store, which the app reads by hash.
	Blobs string
	// Data is notesd's data directory, which Restart keeps.
	Data string
	// Down, while true, makes AppNotes return nil, as a lost connection.
	Down bool
	// opts are the server's options, which Restart keeps.
	opts rpctest.Options
}

// StartNotesd starts a notesd with an empty vault.
func StartNotesd(t testing.TB) *Notesd { return StartNotesdWith(t, rpctest.Options{}) }

// StartNotesdWith is StartNotesd with the server's options, such as a fake
// domain (Options.Services). Its data directory is always a fresh one.
func StartNotesdWith(t testing.TB, o rpctest.Options) *Notesd {
	t.Helper()
	data := t.TempDir()
	o.DataDir = data
	srv := rpctest.StartWith(t, o)
	n := &Notesd{T: t, Srv: srv, App: srv.Dial(t), Other: srv.Dial(t), Dir: t.TempDir(), Blobs: filepath.Join(data, "blobs"), Data: data, opts: o}
	v, err := n.App.Vault().Add(t.Context(), &api.VaultAddParams{Path: n.Dir})
	if err != nil {
		t.Fatal(err)
	}
	n.Vault = v.ID
	return n
}

// Restart stops notesd and starts it again on the same data, on a new
// socket; the connections are made again.
func (n *Notesd) Restart() {
	n.T.Helper()
	n.Srv.Stop()
	n.Srv = rpctest.StartWith(n.T, n.opts)
	n.App, n.Other = n.Srv.Dial(n.T), n.Srv.Dial(n.T)
}

// AppNotes is the app's note domain, nil while Down.
func (n *Notesd) AppNotes() *api.NoteClient {
	if n.Down {
		return nil
	}
	c := n.App.Note()
	return &c
}

// Create makes a note as another writer and returns its hash.
func (n *Notesd) Create(path, content string) string {
	n.T.Helper()
	res, err := n.Other.Note().Create(n.T.Context(), &api.NoteCreateParams{VaultID: n.Vault, Path: path, Content: content})
	if err != nil {
		n.T.Fatal(err)
	}
	return res.Hash
}

// Write writes a note as another writer, from base, and returns its hash.
func (n *Notesd) Write(path, base, content string) string {
	n.T.Helper()
	res, err := n.Other.Note().Write(n.T.Context(), &api.NoteWriteParams{VaultID: n.Vault, Path: path, Base: base, Content: content})
	if err != nil {
		n.T.Fatal(err)
	}
	return res.Hash
}

// Read reads a note as another writer.
func (n *Notesd) Read(path string) *api.Note {
	n.T.Helper()
	note, err := n.Other.Note().Read(n.T.Context(), &api.NoteReadParams{VaultID: n.Vault, Path: path})
	if err != nil {
		n.T.Fatal(err)
	}
	return note
}
