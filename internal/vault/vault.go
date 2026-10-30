package vault

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/store"
)

// Config is what every vault's actor shares. Zero fields take the defaults
// in brackets.
type Config struct {
	DB    *store.DB
	Blobs *blobs.Store
	Log   *slog.Logger // [discard]
	// Publish delivers transient events (vault.scan); nil drops them.
	// Durable events go through the store as usual.
	Publish func([]api.EventEnvelope)
	// Now is the clock of daily notes and pruning [time.Now].
	Now func() time.Time
	// Debounce is how long a path must be quiet before the watcher's
	// events for it are handled [100ms].
	Debounce time.Duration
	// Rescan is the interval of rescans [5m]; NetworkRescan replaces it
	// for a vault on a network file system [10s].
	Rescan, NetworkRescan time.Duration
	// Batch is the most files one scan transaction indexes [200].
	Batch int
	// Keep is how long versions are kept [30 days]; PruneEvery is the
	// interval of the pass that prunes them [24h].
	Keep, PruneEvery time.Duration
	// Crash, when set, is asked at each point a crash could fall (only
	// "renamed": after a change's rename, before its transaction); true
	// stops the change there with ErrCrashed. The simulation sets it
	// (ADR-0020); notesd never does.
	Crash func(point string) bool
}

func (c Config) withDefaults() Config {
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.Debounce == 0 {
		c.Debounce = 100 * time.Millisecond
	}
	if c.Rescan == 0 {
		c.Rescan = 5 * time.Minute
	}
	if c.NetworkRescan == 0 {
		c.NetworkRescan = 10 * time.Second
	}
	if c.Batch == 0 {
		c.Batch = 200
	}
	if c.Keep == 0 {
		c.Keep = 30 * 24 * time.Hour
	}
	if c.PruneEvery == 0 {
		c.PruneEvery = 24 * time.Hour
	}
	return c
}

// Vault is one vault's actor. A goroutine scans and watches the vault's
// folder; mu serializes every change to the vault, whoever makes it: a
// write, a scan's batch, the watcher's paths. Reads of the index need no
// lock.
type Vault struct {
	id   int64
	root string
	cfg  *Config
	log  *slog.Logger

	mu sync.Mutex
	// ignore is the vault's ignore patterns as the last scan read them;
	// mu guards it.
	ignore *Ignore
	// removed is set, under mu, before the vault's row is deleted: no
	// change may start on a vault that is going away.
	removed bool
	// settingsDigest is vault.yaml's content hash as notesd last read or
	// wrote it, once settingsSeen; mu guards both. A scan that finds
	// another emits vault.configured.
	settingsDigest string
	settingsSeen   bool

	smu      sync.Mutex
	status   api.ScanStatus
	lastSent time.Time

	// walked, when set, gets the folders each scan's walk found: the
	// actor watches them before the scan reads a file.
	walked func(dirs []string)

	// staged, when set, runs in a write after its content is staged and
	// before the disk is compared and the rename: tests edit the file
	// there, as another program might.
	staged func(p string)

	scanReq chan struct{} // a scan is wanted
	reindex atomic.Bool   // the next scan reads every file again
	cancel  context.CancelFunc
	done    chan struct{}

	scanned     chan struct{} // closed when the open scan has finished
	scannedOnce sync.Once
}

func newVault(cfg *Config, v store.Vault) *Vault {
	return &Vault{
		id: v.ID, root: v.Path, cfg: cfg, log: cfg.Log.With("vault", v.ID),
		status:  api.ScanStatus{VaultID: v.ID, State: api.ScanStateIdle},
		scanReq: make(chan struct{}, 1),
		done:    make(chan struct{}),
		scanned: make(chan struct{}),
	}
}

// ID is the vault's ID.
func (v *Vault) ID() int64 { return v.id }

// Root is the vault's folder.
func (v *Vault) Root() string { return v.root }

func (v *Vault) start(ctx context.Context) {
	ctx, v.cancel = context.WithCancel(ctx)
	go v.run(ctx)
}

func (v *Vault) stop() {
	v.setRemoved(true)
	v.cancel()
	<-v.done
}

// lock takes mu for a change, or refuses one on a vault being removed.
func (v *Vault) lock() error {
	v.mu.Lock()
	if v.removed {
		v.mu.Unlock()
		return api.NotFound("vault %d does not exist", v.id)
	}
	return nil
}

// setRemoved marks the vault as going away, once the changes running now
// have finished, or not, when its removal failed.
func (v *Vault) setRemoved(removed bool) {
	v.mu.Lock()
	v.removed = removed
	v.mu.Unlock()
}

// open returns an os.Root of the vault's folder: every file operation
// goes through one, so no path escapes the vault.
func (v *Vault) open() (*os.Root, error) { return os.OpenRoot(v.root) }

// Scanned waits until the vault's open scan has finished, done or
// failed: the index then holds what the disk held when notesd started, a
// file a crash left behind or took away included.
func (v *Vault) Scanned(ctx context.Context) error {
	select {
	case <-v.scanned:
		return nil
	case <-v.done:
		return api.NotFound("vault %d stopped", v.id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status is the state of the vault's index and scans.
func (v *Vault) Status(ctx context.Context) (api.ScanStatus, error) {
	v.smu.Lock()
	s := v.status
	v.smu.Unlock()
	notes, attachments, err := v.cfg.DB.FileCounts(ctx, v.id)
	if err != nil {
		return api.ScanStatus{}, err
	}
	s.Notes, s.Attachments = int64(notes), int64(attachments)
	return s, nil
}

// Reindex asks for a scan that reads every file again and rebuilds its
// index rows, and returns the status.
func (v *Vault) Reindex(ctx context.Context) (api.ScanStatus, error) {
	v.reindex.Store(true)
	v.requestScan()
	v.setStatus(func(s *api.ScanStatus) {
		if s.State != api.ScanStateScanning {
			s.State, s.Done, s.Total = api.ScanStateScanning, 0, 0
		}
	}, true)
	return v.Status(ctx)
}

// requestScan asks for a scan unless one is pending.
func (v *Vault) requestScan() {
	select {
	case v.scanReq <- struct{}{}:
	default:
	}
}

// setStatus changes the status and publishes a vault.scan event, at most
// every 250 ms unless force.
func (v *Vault) setStatus(f func(*api.ScanStatus), force bool) {
	v.smu.Lock()
	f(&v.status)
	s := v.status
	now := time.Now()
	send := force || now.Sub(v.lastSent) >= 250*time.Millisecond
	if send {
		v.lastSent = now
	}
	v.smu.Unlock()
	if !send || v.cfg.Publish == nil {
		return
	}
	notes, attachments, err := v.cfg.DB.FileCounts(context.Background(), v.id)
	if err == nil {
		s.Notes, s.Attachments = int64(notes), int64(attachments)
	}
	env, err := api.NewEnvelope(0, api.VaultScan{Status: s})
	if err != nil {
		v.log.Warn("encode scan event", "err", err)
		return
	}
	v.cfg.Publish([]api.EventEnvelope{env})
}
