package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// revokedSync is a fake sync domain whose Join always answers revoked.
type revokedSync struct{ *fakeSync }

func (f revokedSync) Join(_ context.Context, p *api.SyncJoinParams) (*api.SyncStatus, error) {
	_ = f.record(p)
	return nil, api.Revoked("the hub refused this device's token: pair again")
}

func TestHubJoinPairsAgainOnceWhenRevoked(t *testing.T) {
	fake := &fakeSync{
		hubs:    []api.SyncHub{{ID: 1, Hub: "h1", URL: "https://hub.example", Device: 4, PairedAt: when}},
		pairing: api.Pairing{Pairing: "p1", Code: "7K4M-Q2XD", Hub: "h1", URL: "https://hub.example", ExpiresAt: when},
		states:  []api.PairingState{api.PairingStateApproved},
	}
	var o rpctest.Options
	o.Services = func(s *api.Services) { s.Hub, s.Sync = &fakeHub{}, revokedSync{fake} }
	srv := rpctest.StartWith(t, o)
	if _, err := runNotesctl(t, "--socket", srv.Socket, "vault", "add", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	err := notesErr(t, srv, "hub", "join", "https://hub.example", "Notes", "--device", "laptop")
	if !strings.Contains(err.Error(), "refused") {
		t.Fatalf("hub join whose second join is revoked too = %v, want its error", err)
	}
	join := &api.SyncJoinParams{VaultID: 1, URL: "https://hub.example", Name: "Notes"}
	checkCalls(t, fake.took(),
		&api.SyncHubsParams{},
		join,
		&api.SyncPairParams{URL: "https://hub.example", Name: ptr("laptop")},
		&api.SyncPairedParams{Pairing: "p1"},
		join, // the second try is the last: no loop
	)
}

// approver approves every pairing that appears on the hub, until stop.
func approver(t *testing.T, hub *api.Client) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Go(func() {
		for ctx.Err() == nil {
			pending, err := hub.Hub().Pending(ctx, &api.HubPendingParams{})
			if err != nil {
				return
			}
			for _, p := range pending {
				if !p.Approved {
					_, _ = hub.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code})
				}
			}
		}
	})
	return func() { cancel(); wg.Wait() }
}

// rejoinSetup is a hub serving the vaults named hubVaults and a device
// with one empty vault per name in deviceVaults, their IDs 1, 2, ….
type rejoinSetup struct {
	hub, dev       *rpctest.Server
	hubC, devC     *api.Client
	hubIDs         map[string]int64
	deviceVaultIDs []int64
}

func newRejoinSetup(t *testing.T, hubVaults, deviceVaults []string) *rejoinSetup {
	t.Helper()
	s := &rejoinSetup{hubIDs: map[string]int64{}}
	s.hub = rpctest.StartWith(t, rpctest.Options{HubListen: true})
	s.hubC = s.hub.Dial(t)
	for _, name := range hubVaults {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		v, err := s.hubC.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.hubC.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: v.ID}); err != nil {
			t.Fatal(err)
		}
		s.hubIDs[name] = v.ID
	}
	s.dev = rpctest.StartWith(t, rpctest.Options{Syncer: syncer.Config{Quiet: time.Millisecond, MaxQuiet: 10 * time.Millisecond,
		Idle: time.Second, Backoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond}})
	s.devC = s.dev.Dial(t)
	for _, name := range deviceVaults {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		v, err := s.devC.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
		if err != nil {
			t.Fatal(err)
		}
		s.deviceVaultIDs = append(s.deviceVaultIDs, v.ID)
	}
	return s
}

// join runs notesctl hub join on the device, approving its pairings.
func (s *rejoinSetup) join(t *testing.T, vault, hubVault string) string {
	t.Helper()
	defer approver(t, s.hubC)()
	return notes(t, s.dev, "--vault", vault, "hub", "join", s.hub.HubURL, hubVault, "--device", "laptop")
}

// revokeDevices revokes every device the hub knows.
func (s *rejoinSetup) revokeDevices(t *testing.T) {
	t.Helper()
	ds, err := s.hubC.Hub().Devices(t.Context(), &api.HubDevicesParams{})
	if err != nil || len(ds) == 0 {
		t.Fatalf("Devices = %v, %v", ds, err)
	}
	for _, d := range ds {
		if err := s.hubC.Hub().Revoke(t.Context(), &api.HubRevokeParams{Device: d.ID}); err != nil {
			t.Fatal(err)
		}
	}
}

// waitRevoked waits until the device's vault syncs in state revoked.
func (s *rejoinSetup) waitRevoked(t *testing.T, vault int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, s.dev.Socket, "watch")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.devC.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &vault}); len(st) == 1 && st[0].State == api.SyncStateRevoked {
		return
	}
	for {
		select {
		case ev := <-c.Notifications():
			if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
				if ch, ok := e.(api.SyncChanged); ok && ch.Status.VaultID == vault && ch.Status.State == api.SyncStateRevoked {
					return
				}
			}
		case <-ctx.Done():
			t.Fatal("the device's sync did not become revoked")
		}
	}
}

// hubSeq is the log's latest sequence number of the hub's vault.
func (s *rejoinSetup) hubSeq(t *testing.T, name string) int64 {
	t.Helper()
	vs, err := s.hubC.Hub().Vaults(t.Context(), &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Name == name {
			return v.Seq
		}
	}
	t.Fatalf("the hub serves no vault %q", name)
	return 0
}

// syncsAgain checks that the device's vault syncs: a note it writes
// reaches the hub's log.
func (s *rejoinSetup) syncsAgain(t *testing.T, vault int64, hubVault string) {
	t.Helper()
	before := s.hubSeq(t, hubVault)
	if _, err := s.devC.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: vault, Path: "after.md", Content: "back\n"}); err != nil {
		t.Fatal(err)
	}
	// The loop learns of the note from an event: wait for cycles until it
	// has pushed it.
	for range 50 {
		if err := s.dev.Syncer.WaitIdle(t.Context(), vault); err != nil {
			t.Fatal(err)
		}
		st, err := s.devC.Sync().Status(t.Context(), &api.SyncStatusParams{VaultID: &vault})
		if err != nil || len(st) != 1 || st[0].State == api.SyncStateRevoked {
			t.Fatalf("Status = %+v, %v; want a vault that syncs", st, err)
		}
		if st[0].Pending == 0 && s.hubSeq(t, hubVault) > before {
			return
		}
	}
	t.Fatalf("the note did not reach the hub's log (at %d)", s.hubSeq(t, hubVault))
}

func TestHubJoinPairsAgainAfterRevocation(t *testing.T) {
	s := newRejoinSetup(t, []string{"Notes"}, []string{"garden"})
	want := "approve this device on the hub: notesctl hub approve "
	out := s.join(t, "garden", "Notes")
	if !strings.Contains(out, want) || !strings.Contains(out, "syncing Notes at "+strings.TrimSuffix(s.hub.HubURL, "/")+": ") {
		t.Fatalf("hub join = %q; want the hub vault and its URL named in the last line", out)
	}
	s.revokeDevices(t)
	s.waitRevoked(t, s.deviceVaultIDs[0])
	notes(t, s.dev, "--vault", "garden", "sync", "leave")
	out = s.join(t, "garden", "Notes")
	if !strings.Contains(out, "refused this device's token; pairing again") || strings.Count(out, "approve this device on the hub") != 1 {
		t.Fatalf("hub join after a revocation = %q; want 1 new pairing", out)
	}
	s.syncsAgain(t, s.deviceVaultIDs[0], "Notes")
}

func TestHubJoinRevivesAVaultThatWasRevoked(t *testing.T) {
	s := newRejoinSetup(t, []string{"Alpha", "Beta"}, []string{"garden", "work"})
	s.join(t, "garden", "Alpha")
	s.revokeDevices(t)
	s.waitRevoked(t, s.deviceVaultIDs[0])
	out := s.join(t, "work", "Beta")
	if !strings.Contains(out, "pairing again") {
		t.Fatalf("hub join after a revocation = %q; want a new pairing", out)
	}
	s.syncsAgain(t, s.deviceVaultIDs[0], "Alpha") // garden never left
	s.syncsAgain(t, s.deviceVaultIDs[1], "Beta")
}
