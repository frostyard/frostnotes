package syncer

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobxfer"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Batch sizes (docs/specs/sync-protocol.md, Limits and timings).
const (
	pushBatch  = 100
	queryBatch = 1000
	pullPage   = 500
)

// loop syncs one vault with its hub.
type loop struct {
	s       *Syncer
	vaultID int64
	cancel  context.CancelFunc
	done    chan struct{}
	wake    chan struct{}

	mu      sync.Mutex
	hub     int64 // the remote's sync_hubs row
	status  api.SyncStatus
	dirty   map[string]mark // local changes by path, for quiet periods
	skipped map[string]bool
	kicks   uint64        // kick() calls
	idle    uint64        // the kicks a cycle that found nothing to do had seen
	idled   chan struct{} // closed and replaced after each idle cycle

	// parked are local files moved aside to make room for a head, until
	// their own head or answer moves them on; the loop's goroutine alone
	// uses it.
	parked map[int64]parkedFile
}

// parkedFile is where a parked file was and is, and whether its agreed
// state followed it there.
type parkedFile struct {
	from, to string
	agreed   bool
}

// mark is when a path last changed, and first since it was last pushed.
type mark struct{ first, last time.Time }

func newLoop(s *Syncer, vaultID int64, cancel context.CancelFunc) *loop {
	return &loop{s: s, vaultID: vaultID, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1),
		status: api.SyncStatus{VaultID: vaultID, State: api.SyncStateOffline, Refused: []api.SyncRefusal{}, Skipped: []string{}},
		dirty:  map[string]mark{}, skipped: map[string]bool{}, idled: make(chan struct{}), parked: map[int64]parkedFile{}}
}

func (l *loop) kick() {
	l.mu.Lock()
	l.kicks++
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *loop) hubRow() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hub
}

func (l *loop) waitIdle(ctx context.Context) error {
	l.mu.Lock()
	want := l.kicks + 1
	l.mu.Unlock()
	l.kick()
	for {
		l.mu.Lock()
		reached, ch := l.idle >= want, l.idled
		l.mu.Unlock()
		if reached {
			return nil
		}
		select {
		case <-ch:
		case <-l.done:
			return errors.New("the vault stopped syncing")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// setState records the loop's state and announces a change.
func (l *loop) setState(state api.SyncState, err error) {
	l.mu.Lock()
	changed := l.status.State != state
	l.status.State = state
	if err != nil {
		msg := err.Error()
		changed = changed || l.status.Error == nil || *l.status.Error != msg
		l.status.Error = &msg
	} else if l.status.Error != nil {
		l.status.Error, changed = nil, true
	}
	st := l.status
	l.mu.Unlock()
	if changed {
		l.announce(st)
	}
}

func (l *loop) announce(st api.SyncStatus) {
	if l.s.cfg.Publish == nil {
		return
	}
	env, err := api.NewEnvelope(0, api.SyncChanged{Status: st})
	if err == nil {
		l.s.cfg.Publish([]api.EventEnvelope{env})
	}
}

// run connects and syncs until ctx ends or the vault leaves its hub.
func (l *loop) run(ctx context.Context) {
	go l.watchLocal(ctx)
	backoff := l.s.cfg.Backoff
	for ctx.Err() == nil {
		remote, err := l.s.cfg.DB.Remote(ctx, l.vaultID)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				l.s.cfg.Log.Warn("read a remote", "vault", l.vaultID, "err", err)
			}
			return
		}
		l.mu.Lock()
		l.hub = remote.Hub
		l.mu.Unlock()
		sess, err := l.s.dial(ctx, remote.Hub)
		if err == nil {
			backoff = l.s.cfg.Backoff
			err = l.serve(ctx, sess)
			_ = sess.client.Close()
		}
		if ctx.Err() != nil {
			return
		}
		l.s.cfg.Log.Info("sync session ended", "vault", l.vaultID, "err", err)
		var wait <-chan time.Time
		switch {
		case errors.Is(err, errRevoked):
			l.setState(api.SyncStateRevoked, err)
		case errors.Is(err, errReset):
			l.setState(api.SyncStateReset, err)
		default:
			l.setState(api.SyncStateOffline, err)
			jitter := 0.8 + 0.4*rand.Float64()
			wait = time.After(time.Duration(float64(backoff) * jitter))
			backoff = min(backoff*2, l.s.cfg.MaxBackoff)
		}
		select {
		case <-l.wake:
		case <-wait:
		case <-ctx.Done():
			return
		}
	}
}

// watchLocal marks the paths the device's own writers change, and wakes
// the loop.
func (l *loop) watchLocal(ctx context.Context) {
	for ctx.Err() == nil {
		sub, _, err := l.s.cfg.Broker.Subscribe(ctx, nil)
		if err != nil {
			return
		}
		_ = sub.Run(ctx, func(ev api.EventEnvelope) error {
			e, err := api.DecodeEvent(ev.Event, ev.Data)
			if err != nil {
				return nil
			}
			var paths []string
			switch e := e.(type) {
			case api.NoteChanged:
				if e.VaultID == l.vaultID && e.Source != api.VersionSourceSync {
					paths = []string{e.Path}
				}
			case api.NoteMoved:
				if e.VaultID == l.vaultID {
					paths = []string{e.From, e.To}
				}
			case api.NoteRemoved:
				if e.VaultID == l.vaultID {
					paths = []string{e.Path}
				}
			}
			if len(paths) > 0 {
				now := l.s.cfg.Now()
				l.mu.Lock()
				for _, p := range paths {
					m, ok := l.dirty[p]
					if !ok {
						m.first = now
					}
					m.last = now
					l.dirty[p] = m
				}
				l.mu.Unlock()
				l.kick()
			}
			return nil
		})
		sub.Close()
		l.kick() // events may have been missed: look at everything
	}
}

// serve syncs over one connection until it fails.
func (l *loop) serve(ctx context.Context, sess *session) error {
	remote, err := l.s.cfg.DB.Remote(ctx, l.vaultID)
	if err != nil {
		return err
	}
	vaults, err := sess.client.Log().Vaults(ctx, &api.LogVaultsParams{})
	if err != nil {
		return err
	}
	served := slices.IndexFunc(vaults, func(v api.LogVault) bool { return v.Sync == remote.SyncID })
	if served < 0 {
		return errReset
	}
	if err := l.ownReplica(ctx, sess, remote); err != nil {
		return err
	}
	l.setHubSeq(vaults[served].Seq)
	l.setState(api.SyncStateSyncing, nil)
	for {
		l.mu.Lock()
		seen := l.kicks
		l.mu.Unlock()
		worked, next, err := l.cycle(ctx, sess)
		if err != nil {
			return err
		}
		if worked {
			continue
		}
		l.setState(api.SyncStateIdle, nil)
		l.mu.Lock()
		if seen > l.idle {
			l.idle = seen
		}
		close(l.idled)
		l.idled = make(chan struct{})
		l.mu.Unlock()
		timer := time.NewTimer(next)
		select {
		case <-l.wake:
		case ev, ok := <-sess.client.Notifications():
			if !ok {
				timer.Stop()
				return sess.client.Err()
			}
			// One pull answers every announcement that is waiting.
			l.noteAppended(ev, remote.SyncID)
			for drained := false; !drained; {
				select {
				case ev, ok := <-sess.client.Notifications():
					if !ok {
						timer.Stop()
						return sess.client.Err()
					}
					l.noteAppended(ev, remote.SyncID)
				default:
					drained = true
				}
			}
		case <-timer.C:
		case <-sess.client.Done():
			timer.Stop()
			return sess.client.Err()
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		timer.Stop()
		l.setState(api.SyncStateSyncing, nil)
	}
}

// ownReplica makes sure the remote's replica is this device's: after the
// device paired again, the hub knows it as a new device, which joins anew
// and keeps its agreed state and cursor (docs/specs/sync-protocol.md, URLs
// and tokens).
func (l *loop) ownReplica(ctx context.Context, sess *session, remote store.Remote) error {
	_, err := sess.client.Log().Pushes(ctx, &api.LogPushesParams{Vault: remote.SyncID, Replica: remote.ReplicaID, Pushes: []int64{}})
	var ae *api.Error
	if err == nil || !errors.As(err, &ae) || ae.Code != api.CodeNotFound {
		return err
	}
	rep, err := sess.client.Log().Join(ctx, &api.LogJoinParams{Vault: remote.SyncID})
	if err != nil {
		return err
	}
	l.s.cfg.Log.Info("joined the hub's vault again after pairing", "vault", l.vaultID, "replica", rep.ID)
	return l.s.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetReplica(ctx, l.vaultID, rep.ID) })
}

// noteAppended takes the hub's sequence number from a log.appended.
func (l *loop) noteAppended(ev api.EventEnvelope, syncID string) {
	if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
		if a, ok := e.(api.LogAppended); ok && a.Vault == syncID {
			l.setHubSeq(a.Seq)
		}
	}
}

func (l *loop) setHubSeq(seq int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.status.HubSeq = max(l.status.HubSeq, seq)
}

// cycle pushes what is due and pulls what is new. It reports whether it
// changed anything, and how long to wait before the next cycle unasked.
func (l *loop) cycle(ctx context.Context, sess *session) (bool, time.Duration, error) {
	worked := false
	next := l.s.cfg.Idle
	for range 4 {
		pushed, wait, err := l.push(ctx, sess)
		if err != nil {
			return false, 0, err
		}
		if wait > 0 {
			next = min(next, wait)
		}
		pulled, blocked, err := l.pull(ctx, sess)
		if err != nil {
			return false, 0, err
		}
		l.s.cfg.Log.Debug("sync cycle", "device", l.s.cfg.Device, "vault", l.vaultID, "pushed", pushed, "pulled", pulled, "blocked", blocked, "held", wait)
		worked = worked || pushed || pulled
		if !blocked || (!pushed && wait > 0) {
			break
		}
	}
	if err := l.refreshStatus(ctx); err != nil {
		return false, 0, err
	}
	if !worked {
		l.mu.Lock()
		pending := l.status.Pending
		l.mu.Unlock()
		if pending == 0 {
			if err := l.s.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.MarkSynced(ctx, l.vaultID) }); err != nil {
				return false, 0, err
			}
		}
	}
	return worked, next, nil
}

// marks returns a copy of the quiet periods' marks, by path. One push
// judges all its changes by one copy: the events that mark paths arrive
// while it judges them, and two changes that share paths, the two files
// of a swap, must be held or sent together. Split, the hub would number
// the file that moves first onto the path the other still holds.
func (l *loop) marks() map[string]mark {
	l.mu.Lock()
	defer l.mu.Unlock()
	return maps.Clone(l.dirty)
}

// due reports whether a change to the given paths is past its quiet
// period in marks, and if not, how long until it is.
func (l *loop) due(marks map[string]mark, now time.Time, paths ...string) (bool, time.Duration) {
	wait := time.Duration(0)
	for _, p := range paths {
		m, ok := marks[p]
		if !ok {
			continue
		}
		at := m.last.Add(l.s.cfg.Quiet)
		if cap := m.first.Add(l.s.cfg.MaxQuiet); cap.Before(at) {
			at = cap
		}
		if d := at.Sub(now); d > wait {
			wait = d
		}
	}
	return wait <= 0, wait
}

func (l *loop) clean(paths ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, p := range paths {
		delete(l.dirty, p)
	}
}

// push sends the unanswered pushes and the changes past their quiet
// period, and applies the answers. It reports whether it sent any, and the
// wait until a held change is due (0 when none is held).
func (l *loop) push(ctx context.Context, sess *session) (bool, time.Duration, error) {
	db := l.s.cfg.DB
	remote, err := db.Remote(ctx, l.vaultID)
	if err != nil {
		return false, 0, err
	}
	sends, err := db.Unanswered(ctx, l.vaultID)
	if err != nil {
		return false, 0, err
	}
	changes, err := db.Pending(ctx, l.vaultID)
	if err != nil {
		return false, 0, err
	}
	// A file with a push in flight waits for its answer before it is
	// pushed again: a second create of one file would make two on the hub,
	// and a second change is pushed from the state the answer agrees.
	inFlight := map[int64]bool{}
	for _, p := range sends {
		inFlight[p.FileID] = true
	}
	now, marks := l.s.cfg.Now(), l.marks()
	var held time.Duration
	for _, c := range changes {
		if _, parked := l.parked[c.FileID]; inFlight[c.FileID] || parked {
			continue // a parked file waits to be moved on or back
		}
		paths := []string{c.Path}
		if c.Agreed != nil {
			paths = append(paths, c.Agreed.Path)
		}
		if ok, wait := l.due(marks, now, paths...); !ok {
			if held == 0 || wait < held {
				held = wait
			}
			continue
		}
		p := store.LocalPush{VaultID: l.vaultID, FileID: c.FileID, Path: c.Path, Hash: c.Hash}
		if c.Agreed != nil {
			p.HubFile, p.Base = c.Agreed.HubFile, c.Agreed.Revision
		}
		if err := db.Tx(ctx, func(tx *store.Tx) error {
			var err error
			p, err = tx.AddPush(ctx, p)
			return err
		}); err != nil {
			return false, 0, err
		}
		sends = append(sends, p)
	}
	// A writer's content that lost a merge on this device goes to the
	// hub's log as a conflict revision, so it reaches every device's
	// history and flags the file there too.
	lost, err := db.PendingConflicts(ctx, l.vaultID)
	if err != nil {
		return false, 0, err
	}
	for _, c := range lost {
		if inFlight[c.FileID] || slices.ContainsFunc(sends, func(p store.LocalPush) bool { return p.FileID == c.FileID && p.Hash == c.Hash }) {
			continue
		}
		p := store.LocalPush{VaultID: l.vaultID, FileID: c.FileID, HubFile: c.Agreed.HubFile, Base: c.Agreed.Revision,
			Path: c.Agreed.Path, Hash: c.Hash, Conflict: true}
		if err := db.Tx(ctx, func(tx *store.Tx) error {
			var err error
			p, err = tx.AddPush(ctx, p)
			return err
		}); err != nil {
			return false, 0, err
		}
		sends = append(sends, p)
	}
	if len(sends) == 0 {
		return false, held, nil
	}
	if err := l.upload(ctx, sess, remote, sends); err != nil {
		return false, 0, err
	}
	if l.s.crashed("uploaded") {
		return false, 0, ErrCrashed
	}
	for batch := range slices.Chunk(sends, pushBatch) {
		items, send, err := l.items(ctx, batch)
		if err != nil {
			return false, 0, err
		}
		if len(items) == 0 {
			continue
		}
		answers, err := sess.client.Log().Push(ctx, &api.LogPushParams{Vault: remote.SyncID, Replica: remote.ReplicaID, Items: items})
		if err != nil {
			return false, 0, err
		}
		if len(answers) != len(send) {
			return false, 0, fmt.Errorf("the hub answered %d of %d pushes", len(answers), len(send))
		}
		if l.s.crashed("answered") {
			return false, 0, ErrCrashed
		}
		b := &answering{sent: send}
		for i, a := range answers {
			b.at = i
			if err := l.answer(ctx, sess, remote, send[i], a, b); err != nil {
				return false, 0, err
			}
		}
		// A parked file whose answer could not apply, its path then taken,
		// gets a second try once the rest of the batch has applied.
		b.at = len(send)
		for _, i := range slices.Clone(b.retry) {
			if err := l.answer(ctx, sess, remote, send[i], answers[i], b); err != nil {
				return false, 0, err
			}
		}
		// An answer can also park an unchanged file whose new head was
		// not part of this push. Leave it parked until the following pull
		// moves it on; restoring it now could number it and push that
		// temporary name back to the hub as a new move.
	}
	return true, held, nil
}

// upload sends the hub the contents it lacks.
func (l *loop) upload(ctx context.Context, sess *session, remote store.Remote, sends []store.LocalPush) error {
	var hashes []string
	for _, p := range sends {
		if p.Hash != "" && !slices.Contains(hashes, p.Hash) {
			if ok, err := l.s.cfg.Blobs.Has(p.Hash); err == nil && ok {
				hashes = append(hashes, p.Hash)
			}
		}
	}
	for batch := range slices.Chunk(hashes, queryBatch) {
		missing, err := sess.client.Log().Missing(ctx, &api.LogMissingParams{Vault: remote.SyncID, Hashes: batch})
		if err != nil {
			return err
		}
		if err := sess.blobs(l.s).UploadAll(ctx, missing); err != nil {
			if errors.Is(err, blobxfer.ErrUnauthorized) {
				return errRevoked
			}
			return err
		}
	}
	return nil
}

func (sess *session) blobs(s *Syncer) *blobxfer.Client {
	return &blobxfer.Client{Base: sess.base, Token: sess.token, HTTP: s.cfg.HTTP, Store: s.cfg.Blobs, Dir: s.cfg.PartDir}
}

// items makes the wire items of a batch of pushes, with the source and
// author of each file's newest version. A push whose base no longer
// matches the agreed state is answered locally as refused.
func (l *loop) items(ctx context.Context, batch []store.LocalPush) ([]api.PushItem, []store.LocalPush, error) {
	db := l.s.cfg.DB
	var items []api.PushItem
	var sent []store.LocalPush
	for _, p := range batch {
		it := api.PushItem{Push: p.PushID, Base: p.Base, Path: p.Path, Source: api.VersionSourceOutside, At: p.At}
		if vs, err := db.Versions(ctx, p.FileID, 1); err == nil && len(vs) > 0 {
			it.Source, it.Author = vs[0].Source, vs[0].Author
			if it.Source == api.VersionSourceSync {
				it.Source = api.VersionSourceOutside
			}
		}
		if p.HubFile != 0 {
			a, err := db.AgreedByHubFile(ctx, l.vaultID, p.HubFile)
			baseHash := a.Hash
			switch {
			case err == nil && p.Conflict:
				p.Base = a.Revision // a conflict item adds to the file whatever its head
			case errors.Is(err, store.ErrNotFound) && p.Conflict && p.BaseHash != "":
				// Carried past a delete that dropped the agreed row
				// (store.Tx.DropAgreed): its base is the row it had.
				baseHash = p.BaseHash
			case err != nil || a.Revision != p.Base:
				if err := db.Tx(ctx, func(tx *store.Tx) error {
					return tx.AnswerPush(ctx, l.vaultID, p.PushID, api.PushStatusRefused, api.RefuseReasonBase)
				}); err != nil {
					return nil, nil, err
				}
				continue
			}
			file := p.HubFile
			it.File, it.Base, it.BaseHash = &file, p.Base, baseHash
		}
		if p.Conflict {
			conflict := true
			it.Conflict = &conflict
		}
		if p.Hash == "" {
			deleted := true
			it.Deleted = &deleted
		} else {
			hash := p.Hash
			it.Hash = &hash
		}
		items = append(items, it)
		sent = append(sent, p)
	}
	return items, sent, nil
}

// fromHub is the author of what a device applies: the head's author and
// device.
func fromHub(author, device string) vault.Author {
	name := device
	if author != "" {
		name = author + "@" + device
	}
	return vault.Author{Source: api.VersionSourceSync, Name: name}
}

// answer records the hub's answer to push p and makes the file what the
// hub now has, where it still holds what was pushed.
// answering is the pushes of one log.push whose answers are being applied, in
// order: when an answer moves a file onto the path of another file of the
// batch still to be answered, that file is parked aside first, and its own
// answer moves it on from there. A swap of two files' paths applies so.
type answering struct {
	sent  []store.LocalPush
	at    int   // the answer being applied
	retry []int // answers of parked files that could not apply
}

// makeRoom parks the file at p if it is a later push of b, still as it was
// pushed; a crash before that push's answer applies leaves it at the
// parked path, which the device then pushes as a move.
func (l *loop) makeRoom(ctx context.Context, v *vault.Vault, b *answering, p string) error {
	occ, err := l.s.cfg.DB.FileByPath(ctx, l.vaultID, p)
	if err != nil {
		return nil
	}
	for _, q := range b.sent[b.at+1:] {
		// A create's answer may merge it or number it anywhere: only a file
		// the hub knows is parked, and a move a create blocks waits for
		// its answer.
		if q.FileID != occ.ID || q.Path != occ.Path || q.Hash != occ.Hash || q.Hash == "" || q.Conflict || q.HubFile == 0 {
			continue
		}
		to := vault.Beside(p, fmt.Sprintf(".sync-%d", q.FileID))
		if _, err := v.Apply(ctx, vault.Expect{Path: p, Hash: q.Hash}, vault.Head{Path: to, Hash: q.Hash},
			vault.Author{Source: api.VersionSourceSync, Name: "sync"}, nil); err != nil {
			if errors.Is(err, vault.ErrChanged) {
				return nil
			}
			return err
		}
		l.parked[q.FileID] = parkedFile{from: p, to: to}
		l.s.cfg.Log.Debug("parked a file for an answer", "vault", l.vaultID, "file", q.FileID, "from", p, "to", to)
		return nil
	}
	return nil
}

func (l *loop) answer(ctx context.Context, sess *session, remote store.Remote, p store.LocalPush, a api.PushAnswer, b *answering) error {
	db := l.s.cfg.DB
	var reason api.RefuseReason
	if a.Reason != nil {
		reason = *a.Reason
	}
	answerOnly := func(drop int64) error {
		return db.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error {
			if err := tx.AnswerPush(ctx, l.vaultID, p.PushID, a.Status, reason); err != nil {
				return err
			}
			if drop != 0 {
				return tx.DeleteAgreed(ctx, l.vaultID, drop)
			}
			return nil
		})
	}
	l.clean(p.Path)
	if p.Conflict {
		// A conflict item changes the file's flag, not its head.
		return db.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error {
			if err := tx.AnswerPush(ctx, l.vaultID, p.PushID, a.Status, reason); err != nil {
				return err
			}
			if prev, err := tx.AgreedByHubFileTx(ctx, l.vaultID, p.HubFile); err == nil && a.File != nil &&
				prev.Revision == a.File.Revision {
				prev.Seq, prev.Conflicted = a.File.Seq, a.File.Conflicted
				return tx.PutAgreed(ctx, prev)
			}
			return nil
		})
	}
	if a.File == nil || (a.Status == api.PushStatusRefused && reason != api.RefuseReasonEdited) {
		var drop int64
		if reason == api.RefuseReasonBase || reason == api.RefuseReasonUnknown {
			drop = p.HubFile // the agreed state is not the hub's: push the file afresh
		}
		return answerOnly(drop)
	}
	f := *a.File
	if !f.Deleted && (vault.CheckPath(f.Path) != nil || vault.IsSettings(f.Path) || l.ignored(f.Path)) {
		return l.refuseHere(ctx, p, f.Path, api.RefuseReasonIgnored)
	}
	exp := vault.Expect{Path: p.Path, Hash: p.Hash}
	head := vault.Head{Path: f.Path, Hash: f.Hash, Deleted: f.Deleted}
	if !f.Deleted {
		if err := l.download(ctx, sess, f.Hash); err != nil {
			return err
		}
	}
	// Another local file may be agreed with this hub file: a create merged
	// into a file this device knew at another path.
	if prev, err := db.AgreedByHubFile(ctx, l.vaultID, f.File); err == nil && prev.FileID != p.FileID && p.HubFile == 0 {
		if err := l.dropOther(ctx, prev); err != nil {
			return err
		}
	}
	rec := func(ctx context.Context, tx *store.Tx, file store.File) error {
		if err := tx.AnswerPush(ctx, l.vaultID, p.PushID, a.Status, reason); err != nil {
			return err
		}
		if f.Deleted {
			// The file's unpushed conflict versions go on as pushes.
			return tx.DropAgreed(ctx, l.vaultID, f.File)
		}
		id := file.ID
		if id == 0 {
			id = p.FileID
		}
		return tx.PutAgreed(ctx, agreedOf(l.vaultID, f, id))
	}
	v, err := l.s.cfg.Vaults.Vault(l.vaultID)
	if err != nil {
		return err
	}
	if pk, ok := l.parked[p.FileID]; ok {
		exp.Path = pk.to
	}
	if exp.Path == head.Path && exp.Hash == head.Hash {
		return db.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error { return rec(ctx, tx, store.File{}) })
	}
	if !head.Deleted && head.Path != exp.Path {
		if err := l.makeRoom(ctx, v, b, head.Path); err != nil {
			return err
		}
		if _, err := l.clearPath(ctx, v, head.Path, p.FileID, nil); err != nil {
			return err
		}
	}
	_, err = v.Apply(ctx, exp, head, fromHub(f.Author, f.DeviceName), rec)
	var ae *api.Error
	if vault.Unholdable(err) || (errors.As(err, &ae) && ae.Code == api.CodeInvalidParams) {
		return l.refuseHere(ctx, p, f.Path, api.RefuseReasonInvalid)
	}
	if _, parked := l.parked[p.FileID]; parked && errors.Is(err, vault.ErrChanged) && b.at < len(b.sent) {
		b.retry = append(b.retry, b.at)
	}
	if errors.Is(err, vault.ErrChanged) && p.HubFile == 0 && !f.Deleted {
		// A create the hub merged, whose file moved since without changing:
		// the merge applies where the file is now, and the move is pushed
		// next. Without an agreed state, it would be pushed as a second
		// create.
		if local, lerr := db.FileByID(ctx, p.FileID); lerr == nil && local.Hash == p.Hash && local.Path != p.Path {
			_, err = v.Apply(ctx, vault.Expect{Path: local.Path, Hash: p.Hash}, vault.Head{Path: local.Path, Hash: f.Hash},
				fromHub(f.Author, f.DeviceName), rec)
		}
	}
	if errors.Is(err, vault.ErrChanged) {
		// The file changed again since it was pushed: keep the agreed state,
		// so the next push, from the same base, merges again on the hub. A
		// refused delete whose file cannot come back leaves the device's
		// care, and the pull brings it as the hub's.
		var drop int64
		if p.Hash == "" {
			drop = f.File
		}
		return answerOnly(drop)
	}
	return err
}

// refuseHere records the answer to push p, whose head is at a path this
// device does not hold (ignored here, or one its file system refuses), as
// a refusal of the device's own: the file leaves its care, as a pulled
// head at such a path does, and its change is not pushed again until it
// changes once more (docs/specs/sync-protocol.md, Pulling and applying).
func (l *loop) refuseHere(ctx context.Context, p store.LocalPush, at string, reason api.RefuseReason) error {
	l.skip(at)
	return l.s.cfg.DB.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error {
		return tx.AnswerPush(ctx, l.vaultID, p.PushID, api.PushStatusRefused, reason)
	})
}

// dropOther removes local file prev, agreed with a hub file that another
// local file now holds, if it still holds its agreed content; otherwise it
// is left to be pushed as a file of its own.
func (l *loop) dropOther(ctx context.Context, prev store.Agreed) error {
	v, err := l.s.cfg.Vaults.Vault(l.vaultID)
	if err != nil {
		return err
	}
	rec := func(ctx context.Context, tx *store.Tx, _ store.File) error {
		return tx.DropAgreed(ctx, l.vaultID, prev.HubFile)
	}
	_, err = v.Apply(ctx, vault.Expect{Path: prev.Path, Hash: prev.Hash}, vault.Head{Path: prev.Path, Deleted: true},
		vault.Author{Source: api.VersionSourceSync, Name: "sync"}, rec)
	if errors.Is(err, vault.ErrChanged) {
		return l.s.cfg.DB.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error { return rec(ctx, tx, store.File{}) })
	}
	return err
}

func agreedOf(vaultID int64, f api.LogFile, fileID int64) store.Agreed {
	return store.Agreed{VaultID: vaultID, HubFile: f.File, FileID: fileID, Seq: f.Seq, Revision: f.Revision,
		Path: f.Path, Hash: f.Hash, Conflicted: f.Conflicted}
}

func (l *loop) download(ctx context.Context, sess *session, hash string) error {
	err := sess.blobs(l.s).Download(ctx, hash)
	if errors.Is(err, blobxfer.ErrUnauthorized) {
		return errRevoked
	}
	return err
}

// pull applies the heads after the cursor, page by page. It reports
// whether it applied any, and whether a file waits: for its local change
// to be pushed first, or for another file's head to free its path. A file
// that waits holds the cursor below it, and the pages after it are still
// applied, since the head it waits for may be among them.
func (l *loop) pull(ctx context.Context, sess *session) (bool, bool, error) {
	db := l.s.cfg.DB
	remote, err := db.Remote(ctx, l.vaultID)
	if err != nil {
		return false, false, err
	}
	after, hold := remote.Cursor, int64(-1)
	pulled := false
	for {
		limit := int64(pullPage)
		page, err := sess.client.Log().Pull(ctx, &api.LogPullParams{Vault: remote.SyncID, After: after, Limit: &limit})
		if err != nil {
			return false, false, err
		}
		pending, err := l.pendingFiles(ctx)
		if err != nil {
			return false, false, err
		}
		for _, f := range page.Files {
			applied, wait, err := l.applyHead(ctx, sess, f, pending)
			if err != nil {
				return false, false, err
			}
			pulled = pulled || applied
			if wait && hold < 0 {
				hold = f.Seq - 1
			}
		}
		l.setHubSeq(page.Next)
		l.mu.Lock()
		hubSeq := l.status.HubSeq
		l.mu.Unlock()
		cursor := page.Next
		if hold >= 0 {
			cursor = hold
		}
		if err := db.Tx(ctx, func(tx *store.Tx) error { return tx.SetCursor(ctx, l.vaultID, cursor, hubSeq) }); err != nil {
			return false, false, err
		}
		if !page.More {
			return pulled, hold >= 0, l.unpark(ctx)
		}
		after = page.Next
	}
}

// pendingFiles is the local files with a change Pending would push.
func (l *loop) pendingFiles(ctx context.Context) (map[int64]bool, error) {
	changes, err := l.s.cfg.DB.Pending(ctx, l.vaultID)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, c := range changes {
		out[c.FileID] = true
	}
	return out, nil
}

// skip records a pulled path the device does not hold.
func (l *loop) skip(p string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.skipped[p] = true
}

// applyHead applies one pulled head (docs/specs/sync-protocol.md, Pulling
// and applying). It reports whether it changed the vault, and whether the
// file waits for a local change to be pushed first.
func (l *loop) applyHead(ctx context.Context, sess *session, f api.LogFile, pending map[int64]bool) (bool, bool, error) {
	db := l.s.cfg.DB
	a, err := db.AgreedByHubFile(ctx, l.vaultID, f.File)
	agreed := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return false, false, err
	}
	if agreed && a.Seq >= f.Seq {
		return false, false, nil
	}
	if vault.CheckPath(f.Path) != nil || vault.IsSettings(f.Path) || l.ignored(f.Path) {
		l.skip(f.Path)
		return false, false, nil
	}
	if agreed && a.Revision == f.Revision && a.Path == f.Path && a.Hash == f.Hash {
		// Only the conflict flag changed.
		return false, false, db.Tx(ctx, func(tx *store.Tx) error { return tx.PutAgreed(ctx, agreedOf(l.vaultID, f, a.FileID)) })
	}
	v, err := l.s.cfg.Vaults.Vault(l.vaultID)
	if err != nil {
		return false, false, err
	}
	var exp vault.Expect
	if agreed {
		local, err := db.FileByID(ctx, a.FileID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && (local.Path != a.Path || local.Hash != a.Hash)) {
			return false, l.waitOrSkip(a.FileID, f.Path, pending), nil
		} else if err != nil {
			return false, false, err
		}
		exp = vault.Expect{Path: a.Path, Hash: a.Hash}
		if !f.Deleted && f.Path != a.Path {
			if _, err := l.clearPath(ctx, v, f.Path, a.FileID, pending); err != nil {
				return false, false, err
			}
		}
	} else {
		if f.Deleted {
			return false, false, nil // a file this device never had
		}
		if local, err := db.FileByPath(ctx, l.vaultID, f.Path); err == nil {
			free, err := l.clearPath(ctx, v, f.Path, 0, pending)
			if err != nil {
				return false, false, err
			}
			if !free {
				if _, err := db.AgreedByFile(ctx, local.ID); err == nil {
					return false, true, nil // another file's copy, changed here: it is pushed first
				}
				return false, l.waitOrSkip(local.ID, f.Path, pending), nil
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			return false, false, err
		}
		exp = vault.Expect{Path: f.Path}
	}
	if !f.Deleted {
		if err := l.download(ctx, sess, f.Hash); err != nil {
			return false, false, err
		}
	}
	rec := func(ctx context.Context, tx *store.Tx, file store.File) error {
		if f.Deleted {
			// The file's unpushed conflict versions go on as pushes.
			return tx.DropAgreed(ctx, l.vaultID, f.File)
		}
		return tx.PutAgreed(ctx, agreedOf(l.vaultID, f, file.ID))
	}
	_, err = v.Apply(ctx, exp, vault.Head{Path: f.Path, Hash: f.Hash, Deleted: f.Deleted}, fromHub(f.Author, f.DeviceName), rec)
	var ae *api.Error
	switch {
	case errors.Is(err, vault.ErrChanged):
		return false, true, nil
	case errors.As(err, &ae) && ae.Code == api.CodeInvalidParams, vault.Unholdable(err):
		l.skip(f.Path) // a path this device's vault cannot hold
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	return true, false, nil
}

// clearPath makes room at p for a pulled head, other than local file
// self's: a copy of another hub file there, agreed and unchanged, is left
// over from before that file's own head, later in the pull, moves or
// deletes it. It is parked beside p, its agreed state following it, so a
// swap or a ring of moves never waits on itself, and that head then moves
// it on (docs/specs/sync-protocol.md, Pulling and applying). A copy with
// a change of the device's own stays: it is pushed first. It reports
// whether p is free.
func (l *loop) clearPath(ctx context.Context, v *vault.Vault, p string, self int64, pending map[int64]bool) (bool, error) {
	db := l.s.cfg.DB
	occ, err := db.FileByPath(ctx, l.vaultID, p)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return true, nil
	case err != nil:
		return false, err
	case occ.ID == self:
		return true, nil
	}
	a, err := db.AgreedByFile(ctx, occ.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (pending[occ.ID] || a.Path != occ.Path || a.Hash != occ.Hash)) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	a.Path = vault.Beside(p, fmt.Sprintf(".sync-%d", a.HubFile))
	_, err = v.Apply(ctx, vault.Expect{Path: p, Hash: occ.Hash}, vault.Head{Path: a.Path, Hash: occ.Hash},
		vault.Author{Source: api.VersionSourceSync, Name: "sync"},
		func(ctx context.Context, tx *store.Tx, _ store.File) error { return tx.PutAgreed(ctx, a) })
	if errors.Is(err, vault.ErrChanged) {
		return false, nil
	}
	if err == nil {
		l.parked[occ.ID] = parkedFile{from: p, to: a.Path, agreed: true}
		l.s.cfg.Log.Debug("parked a file for a head", "vault", l.vaultID, "file", occ.ID, "from", p, "to", a.Path)
	}
	return err == nil, err
}

// unpark moves back the parked files nothing moved on (their own head
// waited, or their answer could not apply), where their path is still
// free, so a parked name never reaches the hub as a move.
func (l *loop) unpark(ctx context.Context) error {
	db := l.s.cfg.DB
	v, err := l.s.cfg.Vaults.Vault(l.vaultID)
	if err != nil {
		return err
	}
	for id, pk := range l.parked {
		f, err := db.FileByID(ctx, id)
		if err != nil || f.Path != pk.to {
			delete(l.parked, id) // moved on, or gone
			continue
		}
		back := pk.from
		for n := 1; ; n++ {
			_, err := db.FileByPath(ctx, l.vaultID, back)
			if errors.Is(err, store.ErrNotFound) {
				break
			} else if err != nil {
				return err
			}
			// Taken meanwhile: the protocol's own way out of two files
			// wanting one path, a number.
			back = vault.Beside(pk.from, fmt.Sprintf(" %d", n))
		}
		rec := func(ctx context.Context, tx *store.Tx, _ store.File) error {
			if !pk.agreed {
				return nil
			}
			a, err := tx.AgreedByFileTx(ctx, id)
			if err != nil {
				return nil
			}
			if back != pk.from {
				return nil // a move to push, from the agreed state as it was
			}
			a.Path = pk.from
			return tx.PutAgreed(ctx, a)
		}
		_, err = v.Apply(ctx, vault.Expect{Path: pk.to, Hash: f.Hash}, vault.Head{Path: back, Hash: f.Hash},
			vault.Author{Source: api.VersionSourceSync, Name: "sync"}, rec)
		switch {
		case errors.Is(err, vault.ErrChanged):
			continue // its disk is ahead of its index: tried again next time
		case err != nil:
			return err
		}
		l.s.cfg.Log.Debug("moved a parked file back", "vault", l.vaultID, "file", id, "to", back)
		delete(l.parked, id)
	}
	return nil
}

// waitOrSkip decides for a pulled file whose local copy changed: it waits
// for the change to be pushed, or, when the hub refused that change, it is
// skipped, so the cursor moves on.
func (l *loop) waitOrSkip(fileID int64, p string, pending map[int64]bool) bool {
	if pending[fileID] {
		return true
	}
	l.skip(p)
	return false
}

func (l *loop) ignored(p string) bool {
	v, err := l.s.cfg.Vaults.Vault(l.vaultID)
	return err == nil && v.Ignores(p)
}

// refreshStatus recomputes the counts sync.status shows.
func (l *loop) refreshStatus(ctx context.Context) error {
	db := l.s.cfg.DB
	remote, err := db.Remote(ctx, l.vaultID)
	if err != nil {
		return err
	}
	changes, err := db.Pending(ctx, l.vaultID)
	if err != nil {
		return err
	}
	lost, err := db.PendingConflicts(ctx, l.vaultID)
	if err != nil {
		return err
	}
	// A conflict push without an answer is no change of a file, but it
	// is still to go.
	sends, err := db.Unanswered(ctx, l.vaultID)
	if err != nil {
		return err
	}
	unsent := 0
	for _, p := range sends {
		if p.Conflict {
			unsent++
		}
	}
	rows, err := db.AgreedRows(ctx, l.vaultID)
	if err != nil {
		return err
	}
	refused, err := l.refusals(ctx)
	if err != nil {
		return err
	}
	conflicts := 0
	for _, a := range rows {
		if a.Conflicted {
			conflicts++
		}
	}
	h, err := db.SyncHub(ctx, remote.Hub)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.status.URL, l.status.HubVault, l.status.Cursor = h.URL, remote.Name, remote.Cursor
	l.status.HubSeq = max(l.status.HubSeq, remote.HubSeq)
	l.status.Pending, l.status.Conflicts, l.status.Refused = int64(len(changes)+len(lost)+unsent), int64(conflicts), refused
	l.status.Skipped = l.status.Skipped[:0]
	for p := range l.skipped {
		l.status.Skipped = append(l.status.Skipped, p)
	}
	slices.Sort(l.status.Skipped)
	if !remote.SyncedAt.IsZero() {
		at := remote.SyncedAt
		l.status.SyncedAt = &at
	}
	l.mu.Unlock()
	return nil
}

// refusals lists the files whose newest push was refused and that are
// still in the refused state.
func (l *loop) refusals(ctx context.Context) ([]api.SyncRefusal, error) {
	db := l.s.cfg.DB
	pushes, err := db.Pushes(ctx, l.vaultID)
	if err != nil {
		return nil, err
	}
	newest := map[int64]store.LocalPush{}
	for _, p := range pushes {
		newest[p.FileID] = p
	}
	out := []api.SyncRefusal{}
	for _, p := range newest {
		if p.Status != api.PushStatusRefused {
			continue
		}
		f, err := db.FileByID(ctx, p.FileID)
		still := (errors.Is(err, store.ErrNotFound) && p.Hash == "") || (err == nil && f.Path == p.Path && f.Hash == p.Hash)
		if still {
			out = append(out, api.SyncRefusal{Path: p.Path, Reason: p.Reason, At: p.At})
		}
	}
	slices.SortFunc(out, func(a, b api.SyncRefusal) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	return out, nil
}

// snapshot returns the loop's status.
func (l *loop) snapshot() api.SyncStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.status
	st.Refused = slices.Clone(st.Refused)
	st.Skipped = slices.Clone(st.Skipped)
	return st
}
