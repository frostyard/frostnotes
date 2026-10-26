// Package syncer is a device's side of sync (ADR-0005, ADR-0017,
// docs/specs/sync-protocol.md): pairing with hubs, and for each synced
// vault a loop that pushes what differs from the agreed state, applies the
// hub's answers, and pulls and applies the heads the hub announces. A
// device never merges another device's content: it applies a head only
// where the file still holds its agreed state (vault.Vault.Apply), and
// otherwise pushes its own change first.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Config is what the syncer needs. Zero fields take the defaults in
// brackets.
type Config struct {
	DB      *store.DB
	Blobs   *blobs.Store
	Vaults  *vault.Manager
	Secrets secrets.Store
	// Broker delivers the device's own committed events: the loops watch
	// them for local changes. Publish sends the transient sync.changed.
	Broker  *events.Broker
	Publish func([]api.EventEnvelope)
	Log     *slog.Logger     // [discard]
	Now     func() time.Time // the clock of quiet periods [time.Now]
	// Device is this device's name on hubs unless a pairing names one
	// [the host name].
	Device string
	// Client is the name rpc.hello carries to hubs ["notesd"].
	Client string
	// PartDir holds partial downloads; it must exist.
	PartDir string
	// Dial opens every connection to a hub, for HTTP and the WebSocket
	// alike, so a test can cut a device off [a TCP dial with a 15 s
	// timeout].
	Dial func(ctx context.Context, addr string) (net.Conn, error)
	// HTTP sends pairing and blob requests [a client with timeouts, over
	// Dial].
	HTTP *http.Client
	// Quiet is how long a changed file waits for quiet before it is pushed,
	// and MaxQuiet the longest it waits [2s, 30s]. Idle is how often a loop
	// pulls without an announcement [60s].
	Quiet, MaxQuiet, Idle time.Duration
	// Backoff is the first wait before reconnecting, doubling to MaxBackoff
	// [1s, 60s].
	Backoff, MaxBackoff time.Duration
	// Crash, when set, is asked at each point a crash could fall in a push
	// ("uploaded": after the uploads, before the push; "answered": after
	// the hub's answers, before they are applied); true ends the session
	// with ErrCrashed. The simulation sets it (ADR-0020).
	Crash func(point string) bool
}

func (c Config) withDefaults() Config {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Device == "" {
		c.Device, _ = os.Hostname()
	}
	if c.Device == "" {
		c.Device = "device"
	}
	if c.Client == "" {
		c.Client = "notesd"
	}
	if c.Dial == nil {
		d := &net.Dialer{Timeout: 15 * time.Second}
		c.Dial = func(ctx context.Context, addr string) (net.Conn, error) { return d.DialContext(ctx, "tcp", addr) }
	}
	if c.HTTP == nil {
		dial := c.Dial
		c.HTTP = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 45 * time.Second,
			TLSHandshakeTimeout: 15 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 4,
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) { return dial(ctx, addr) }}}
	}
	if c.Quiet == 0 {
		c.Quiet = 2 * time.Second
	}
	if c.MaxQuiet == 0 {
		c.MaxQuiet = 30 * time.Second
	}
	if c.Idle == 0 {
		c.Idle = time.Minute
	}
	if c.Backoff == 0 {
		c.Backoff = time.Second
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = time.Minute
	}
	return c
}

// Syncer runs one loop per synced vault and serves the sync domain.
type Syncer struct {
	cfg Config

	mu       sync.Mutex
	ctx      context.Context // the loops' parent, set by Start
	loops    map[int64]*loop
	pairings map[string]*devicePairing
	wg       sync.WaitGroup
	// keepMu makes keeping a token one unit: the secret store's value, the
	// hub's row and the cleanup of a pairing that failed run together, so
	// that two pairings for one hub cannot undo each other.
	keepMu sync.Mutex
}

// New returns a syncer over cfg; Start runs its loops.
func New(cfg Config) *Syncer {
	return &Syncer{cfg: cfg.withDefaults(), loops: map[int64]*loop{}, pairings: map[string]*devicePairing{}}
}

// Start runs a loop for every synced vault, until ctx ends.
func (s *Syncer) Start(ctx context.Context) error {
	remotes, err := s.cfg.DB.Remotes(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	for _, r := range remotes {
		s.startLoop(r.VaultID)
	}
	return nil
}

// Wait waits for every loop to stop after Start's ctx ends.
func (s *Syncer) Wait() { s.wg.Wait() }

// startLoop runs vault's loop unless one runs.
func (s *Syncer) startLoop(vaultID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.loops[vaultID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	l := newLoop(s, vaultID, cancel)
	s.loops[vaultID] = l
	s.wg.Go(func() {
		l.run(ctx)
		close(l.done)
	})
}

// stopLoop stops vault's loop and waits for it.
func (s *Syncer) stopLoop(vaultID int64) {
	s.mu.Lock()
	l := s.loops[vaultID]
	delete(s.loops, vaultID)
	s.mu.Unlock()
	if l != nil {
		l.cancel()
		<-l.done
	}
}

func (s *Syncer) loop(vaultID int64) *loop {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loops[vaultID]
}

// Kick makes vault's loop look for work now, as a local change or an
// announcement does.
func (s *Syncer) Kick(vaultID int64) {
	if l := s.loop(vaultID); l != nil {
		l.kick()
	}
}

// kickHub wakes the loops of every vault synced with hub row id, after a
// new pairing gave it a token.
func (s *Syncer) kickHub(id int64) {
	s.mu.Lock()
	var ls []*loop
	for _, l := range s.loops {
		ls = append(ls, l)
	}
	s.mu.Unlock()
	for _, l := range ls {
		if l.hubRow() == id {
			l.kick()
		}
	}
}

// WaitIdle returns once vault's loop has finished a cycle, begun after the
// call, that found nothing to push, nothing to pull and nothing pending:
// the quiet point tests and the simulation check at.
func (s *Syncer) WaitIdle(ctx context.Context, vaultID int64) error {
	l := s.loop(vaultID)
	if l == nil {
		return fmt.Errorf("vault %d does not sync", vaultID)
	}
	return l.waitIdle(ctx)
}

// ErrCrashed ends a session that Config.Crash stopped.
var ErrCrashed = errors.New("syncer: crashed for the simulation")

func (s *Syncer) crashed(point string) bool { return s.cfg.Crash != nil && s.cfg.Crash(point) }

// errRevoked means a hub refused this device's token.
var errRevoked = errors.New("the hub refused this device's token: pair again")

// errReset means the hub no longer serves the vault as this device knew it.
var errReset = errors.New("the hub no longer serves this vault as the device knew it: sync reset")
