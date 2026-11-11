// Package session is the desktop app's connection to notesd
// (docs/design/app.md#data, plan 0009): it dials the private socket as the
// local caller, says hello, subscribes to events from the sequence it last
// saw, dials again each second when the connection drops, and keeps the
// open notes' documents, to which it routes note.changed, note.moved and
// note.removed. It imports no UI: the window gives it an Env that runs
// work on its thread.
package session

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// RetryDelay is how long the session waits before it dials again, as the
// web app's.
const RetryDelay = time.Second

// ClientName is what the app tells notesd in rpc.hello, and the author of
// its versions.
const ClientName = "Frostnotes"

// State is where the connection stands.
type State int

// The states.
const (
	Connecting State = iota
	Connected
	Reconnecting
)

func (s State) String() string {
	return [...]string{"connecting", "connected", "reconnecting"}[s]
}

// Dialer connects to notesd and completes rpc.hello.
type Dialer func(ctx context.Context) (*api.Client, *api.Hello, error)

// SocketDialer dials notesd's socket at path.
func SocketDialer(path string) Dialer {
	return func(ctx context.Context) (*api.Client, *api.Hello, error) {
		return api.Dial(ctx, path, ClientName)
	}
}

type docKey struct {
	vault int64
	path  string
}

// Session is the app's connection to notesd. Its methods run on the UI
// thread.
type Session struct {
	env  notes.Env
	dial Dialer

	state  State
	reason string
	client *api.Client
	hello  *api.Hello
	seq    int64
	docs   map[docKey]*notes.Document
	refs   map[*notes.Document]int
	// watched are released documents waiting for a retry to land.
	watched map[*notes.Document]bool
	closed  bool

	listeners []func()
	// OnEvent sees every event after the session has routed note events
	// to the open documents: the stores follow vaults, files and sync.
	OnEvent func(seq int64, ev api.Event)
	// OnResync is called when notesd could not replay the events since
	// the stored sequence: the stores read everything again.
	OnResync func()
	// OnSeq is called with each new event sequence, for the app to keep
	// as its resume point (ADR-0021).
	OnSeq func(seq int64)
}

// New makes a session that resumes events after seq (0 for live events
// only). Start connects it.
func New(env notes.Env, dial Dialer, seq int64) *Session {
	return &Session{env: env, dial: dial, seq: seq, docs: map[docKey]*notes.Document{}, refs: map[*notes.Document]int{}, watched: map[*notes.Document]bool{}}
}

// State and Reason say where the connection stands, and why it dropped.
func (s *Session) State() State   { return s.state }
func (s *Session) Reason() string { return s.reason }

// Hello is notesd's answer to rpc.hello, nil while not connected.
func (s *Session) Hello() *api.Hello { return s.hello }

// Seq is the last event sequence seen.
func (s *Session) Seq() int64 { return s.seq }

// Client is the connection, nil while not connected.
func (s *Session) Client() *api.Client { return s.client }

// Notes is notesd's note domain, nil while not connected.
func (s *Session) Notes() notes.NoteAPI {
	if s.client == nil {
		return nil
	}
	n := s.client.Note()
	return &n
}

// OnChange calls fn on the UI thread when the state changes.
func (s *Session) OnChange(fn func()) { s.listeners = append(s.listeners, fn) }

func (s *Session) notify() {
	for _, fn := range s.listeners {
		fn()
	}
}

// Start connects, and connects again whenever the connection drops,
// until Close.
func (s *Session) Start() { s.connect() }

// Close drops the connection and stops dialing.
func (s *Session) Close() {
	s.closed = true
	if s.client != nil {
		_ = s.client.Close()
	}
}

func (s *Session) connect() {
	since := s.seq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, hello, err := s.dial(ctx)
		var sub *api.Subscription
		if err == nil {
			p := &api.EventsSubscribeParams{}
			if since > 0 {
				p.SinceSeq = &since
			}
			sub, err = c.Events().Subscribe(ctx, p)
			if err != nil {
				_ = c.Close()
			}
		}
		s.env.Post(func() {
			if err != nil {
				s.failed(err)
				return
			}
			s.connected(c, hello, sub)
		})
	}()
}

func (s *Session) failed(err error) {
	if s.closed {
		return
	}
	s.reason = err.Error()
	if s.state == Connected {
		s.state = Reconnecting
	}
	s.notify()
	s.env.After(RetryDelay, s.connect)
}

func (s *Session) connected(c *api.Client, hello *api.Hello, sub *api.Subscription) {
	if s.closed {
		_ = c.Close()
		return
	}
	s.client, s.hello, s.state, s.reason = c, hello, Connected, ""
	if sub.Resync && s.OnResync != nil {
		s.OnResync()
	}
	go s.read(c)
	for _, d := range s.docs {
		d.Reconnected()
	}
	s.notify()
}

// read hands the connection's events to the UI thread until it closes.
func (s *Session) read(c *api.Client) {
	for env := range c.Notifications() {
		ev, err := api.DecodeEvent(env.Event, env.Data)
		if err != nil {
			continue // an event this build does not know
		}
		seq := env.Seq
		s.env.Post(func() { s.event(seq, ev) })
	}
	err := c.Err()
	if err == nil {
		err = errors.New("notesd closed the connection")
	}
	s.env.Post(func() {
		if s.client == c {
			s.client, s.hello = nil, nil
			s.failed(err)
		}
	})
}

func (s *Session) event(seq int64, ev api.Event) {
	if seq > s.seq {
		s.seq = seq
		if s.OnSeq != nil {
			s.OnSeq(seq)
		}
	}
	switch e := ev.(type) {
	case api.NoteChanged:
		if d := s.docs[docKey{e.VaultID, e.Path}]; d != nil {
			d.Changed(e.Hash)
		}
	case api.NoteMoved:
		k := docKey{e.VaultID, e.From}
		if d := s.docs[k]; d != nil {
			delete(s.docs, k)
			s.docs[docKey{e.VaultID, e.To}] = d
			d.Moved(e.To)
		}
	case api.NoteRemoved:
		if d := s.docs[docKey{e.VaultID, e.Path}]; d != nil {
			d.Removed()
		}
	}
	if s.OnEvent != nil {
		s.OnEvent(seq, ev)
	}
}

// Open returns the document of a note, shared by every tab that shows it,
// reading it the first time. Release gives it back.
func (s *Session) Open(vault int64, path string) *notes.Document {
	k := docKey{vault, path}
	d := s.docs[k]
	if d == nil {
		d = notes.NewDocument(s.env, s.Notes, vault, path)
		s.docs[k] = d
		d.Load()
	}
	s.refs[d]++
	return d
}

// Release gives back a document Open returned; the last release saves
// what is unsaved, then forgets it. A save that failed keeps the document
// until a retry lands it, so closing a tab loses no text.
func (s *Session) Release(d *notes.Document) {
	if s.refs[d]--; s.refs[d] > 0 {
		return
	}
	delete(s.refs, d)
	d.Flush(func() {
		if !s.forget(d) && !s.watched[d] {
			s.watched[d] = true
			d.OnChange(func() { s.forget(d) })
		}
	})
}

// forget drops a released document once nothing of it is unsaved; it
// reports whether nothing more is to be waited for.
func (s *Session) forget(d *notes.Document) bool {
	if s.refs[d] > 0 {
		return true // opened again meanwhile
	}
	if d.Unsaved() {
		return false
	}
	for k, v := range s.docs {
		if v == d {
			delete(s.docs, k)
		}
	}
	delete(s.watched, d)
	d.Dispose()
	return true
}

// Unsaved reports whether any open note has text notesd has not saved,
// for the window's close.
func (s *Session) Unsaved() bool {
	for _, d := range s.docs {
		if d.Unsaved() {
			return true
		}
	}
	return false
}

// Flush saves every open note and calls done once none has text unsaved
// (or a failed save waits to retry).
func (s *Session) Flush(done func()) {
	var mu sync.Mutex
	left := len(s.docs) + 1
	finish := func() {
		mu.Lock()
		left--
		last := left == 0
		mu.Unlock()
		if last {
			done()
		}
	}
	for _, d := range s.docs {
		d.Flush(finish)
	}
	finish()
}
