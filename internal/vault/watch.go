package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// run is the vault's actor loop: the open scan, then the watcher's paths
// and rescans until ctx ends.
func (v *Vault) run(ctx context.Context) {
	defer close(v.done)
	w := v.newWatcher()
	if w != nil {
		defer w.close()
		go w.read(ctx)
		v.walked = w.watchDirs // before a long cold scan reads its files
	}
	v.requestScan()
	rescan := time.NewTimer(time.Hour)
	rescan.Stop()
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	var kick <-chan struct{}
	if w != nil {
		kick = w.kick
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.scanReq:
			if _, _ = v.runScan(ctx, v.reindex.Swap(false)); ctx.Err() != nil {
				return // runScan logs a failure and keeps it in the status
			}
			interval := v.cfg.Rescan
			if s, _ := v.Status(ctx); s.Network {
				interval = v.cfg.NetworkRescan
			}
			rescan.Reset(interval)
		case <-rescan.C:
			v.requestScan()
		case <-kick:
			if next, ok := w.next(); ok {
				debounce.Reset(time.Until(next))
			}
		case <-debounce.C:
			paths := w.due(time.Now().Add(v.cfg.Debounce / 2))
			if len(paths) > 0 {
				if err := v.scanPaths(ctx, paths); err != nil && ctx.Err() == nil {
					v.log.Warn("index changed files", "err", err)
				}
			}
			if next, ok := w.next(); ok {
				debounce.Reset(time.Until(next))
			}
		}
	}
}

// watcher watches every folder of a vault with fsnotify and collects the
// paths its events name, each due when it has been quiet for
// Config.Debounce (docs/design/vault.md#scan-and-watch).
type watcher struct {
	v    *Vault
	w    *fsnotify.Watcher
	kick chan struct{}

	mu      sync.Mutex
	pending map[string]time.Time // vault-relative path → when it is due
	dirs    map[string]bool      // the folders watched; "" is the root
	full    bool                 // the watch limit was reached: rescans only
}

// newWatcher watches the vault's root, or returns nil when fsnotify cannot
// run: the vault is then kept up to date by rescans alone.
func (v *Vault) newWatcher() *watcher {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		v.log.Warn("cannot watch the vault; relying on rescans", "err", err)
		return nil
	}
	w := &watcher{v: v, w: fw, kick: make(chan struct{}, 1), pending: map[string]time.Time{}, dirs: map[string]bool{}}
	w.watchDirs(nil)
	return w
}

func (w *watcher) close() { _ = w.w.Close() }

// watchDirs adds watches for the root and dirs, the folders a walk found,
// and forgets the folders that are gone.
func (w *watcher) watchDirs(dirs []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	keep := map[string]bool{"": true}
	for _, d := range dirs {
		keep[d] = true
	}
	for d := range w.dirs {
		if !keep[d] {
			delete(w.dirs, d)
			_ = w.w.Remove(w.abs(d))
		}
	}
	for d := range keep {
		if !w.dirs[d] {
			w.addLocked(d)
		}
	}
}

func (w *watcher) addLocked(dir string) {
	if w.full {
		return
	}
	if err := w.w.Add(w.abs(dir)); err != nil {
		if errors.Is(err, syscall.ENOSPC) {
			w.full = true
			w.v.log.Warn("the inotify watch limit is reached; relying on rescans", "err", err)
		} else {
			w.v.log.Debug("cannot watch folder", "folder", dir, "err", err)
		}
		return
	}
	w.dirs[dir] = true
}

func (w *watcher) abs(rel string) string {
	if rel == "" {
		return w.v.root
	}
	return filepath.Join(w.v.root, filepath.FromSlash(rel))
}

// read takes fsnotify's events until ctx ends.
func (w *watcher) read(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			w.event(ev)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			// An overflow lost events: only a scan finds what they said.
			w.v.log.Warn("watcher error; rescanning", "err", err)
			w.v.requestScan()
		}
	}
}

func (w *watcher) event(ev fsnotify.Event) {
	rel, err := filepath.Rel(w.v.root, ev.Name)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	rel = filepath.ToSlash(rel)
	if rel == SettingsDir || IsSettings(rel) {
		w.v.requestScan() // the ignore patterns may have changed
		return
	}
	if CheckPath(rel) != nil {
		return // hidden: other programs' settings, and writes' temporary files
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if ev.Has(fsnotify.Create) {
		if fi, err := os.Lstat(ev.Name); err == nil && fi.IsDir() {
			// A folder made, or moved in with its files: watch it, and let
			// a scan find what it holds.
			w.addLocked(rel)
			w.v.requestScan()
			return
		}
	}
	if (ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename)) && w.dirs[rel] {
		for d := range w.dirs {
			if d == rel || strings.HasPrefix(d, rel+"/") {
				delete(w.dirs, d)
			}
		}
		w.v.requestScan() // a folder moved or deleted with its files
		return
	}
	w.pending[rel] = time.Now().Add(w.v.cfg.Debounce)
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// next returns when the earliest pending path is due.
func (w *watcher) next() (time.Time, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var first time.Time
	for _, t := range w.pending {
		if first.IsZero() || t.Before(first) {
			first = t
		}
	}
	return first, !first.IsZero()
}

// due removes and returns, sorted, the pending paths due by before. The
// paths of one rename are due together, so a move is seen as one.
func (w *watcher) due(before time.Time) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for p, t := range w.pending {
		if !t.After(before) {
			out = append(out, p)
			delete(w.pending, p)
		}
	}
	slices.Sort(out)
	return out
}
