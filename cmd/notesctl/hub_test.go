package main

// CONTRACT TEST for task card T-0030 (docs/tasks). Do not edit.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// recorder keeps the params of every call a fake domain gets.
type recorder struct {
	mu    sync.Mutex
	calls []any
	err   error // every method returns it when set
}

func (r *recorder) record(p any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, p)
	return r.err
}

// took returns the calls made since the last took, and resets.
func (r *recorder) took() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.calls
	r.calls = nil
	return c
}

// fakeHub serves the hub domain from its fields.
type fakeHub struct {
	recorder
	status   api.HubStatus
	vault    api.HubVault
	vaults   []api.HubVault
	pending  []api.PendingPairing
	approved api.PendingPairing
	devices  []api.HubDevice
}

func (f *fakeHub) Status(_ context.Context, p *api.HubStatusParams) (*api.HubStatus, error) {
	return &f.status, f.record(p)
}
func (f *fakeHub) Serve(_ context.Context, p *api.HubServeParams) (*api.HubVault, error) {
	return &f.vault, f.record(p)
}
func (f *fakeHub) Vaults(_ context.Context, p *api.HubVaultsParams) ([]api.HubVault, error) {
	return f.vaults, f.record(p)
}
func (f *fakeHub) Pending(_ context.Context, p *api.HubPendingParams) ([]api.PendingPairing, error) {
	return f.pending, f.record(p)
}
func (f *fakeHub) Approve(_ context.Context, p *api.HubApproveParams) (*api.PendingPairing, error) {
	return &f.approved, f.record(p)
}
func (f *fakeHub) Devices(_ context.Context, p *api.HubDevicesParams) ([]api.HubDevice, error) {
	return f.devices, f.record(p)
}
func (f *fakeHub) Revoke(_ context.Context, p *api.HubRevokeParams) error { return f.record(p) }

// fakeSync serves the sync domain from its fields. Paired answers the
// states in order, then the last one again.
type fakeSync struct {
	recorder
	pairing   api.Pairing
	states    []api.PairingState
	hubs      []api.SyncHub
	status    []api.SyncStatus
	conflicts []api.SyncConflict
	report    api.VerifyReport
	history   []api.Revision
}

func (f *fakeSync) Pair(_ context.Context, p *api.SyncPairParams) (*api.Pairing, error) {
	return &f.pairing, f.record(p)
}
func (f *fakeSync) Paired(_ context.Context, p *api.SyncPairedParams) (*api.PairResult, error) {
	err := f.record(p)
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.states[0]
	if len(f.states) > 1 {
		f.states = f.states[1:]
	}
	return &api.PairResult{State: state, Hub: f.pairing.Hub}, err
}
func (f *fakeSync) Hubs(_ context.Context, p *api.SyncHubsParams) ([]api.SyncHub, error) {
	return f.hubs, f.record(p)
}
func (f *fakeSync) Join(_ context.Context, p *api.SyncJoinParams) (*api.SyncStatus, error) {
	return &f.status[0], f.record(p)
}
func (f *fakeSync) Leave(_ context.Context, p *api.SyncLeaveParams) error { return f.record(p) }
func (f *fakeSync) Status(_ context.Context, p *api.SyncStatusParams) ([]api.SyncStatus, error) {
	return f.status, f.record(p)
}
func (f *fakeSync) Conflicts(_ context.Context, p *api.SyncConflictsParams) ([]api.SyncConflict, error) {
	return f.conflicts, f.record(p)
}
func (f *fakeSync) Resolve(_ context.Context, p *api.SyncResolveParams) error { return f.record(p) }
func (f *fakeSync) Reset(_ context.Context, p *api.SyncResetParams) (*api.SyncStatus, error) {
	return &f.status[0], f.record(p)
}
func (f *fakeSync) Verify(_ context.Context, p *api.SyncVerifyParams) (*api.VerifyReport, error) {
	return &f.report, f.record(p)
}
func (f *fakeSync) History(_ context.Context, p *api.SyncHistoryParams) ([]api.Revision, error) {
	return f.history, f.record(p)
}

func (f *fakeSync) ReadRevision(_ context.Context, p *api.SyncReadRevisionParams) (*api.RevisionContent, error) {
	return nil, f.record(p)
}

// syncServer runs notesd with the fakes as its hub and sync domains and
// registers one vault per name, so their IDs are 1, 2, ….
func syncServer(t *testing.T, hub *fakeHub, dev *fakeSync, vaults ...string) *rpctest.Server {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Hub: hub, Sync: dev})
	for _, name := range vaults {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := runNotesctl(t, "--socket", srv.Socket, "vault", "add", dir); err != nil {
			t.Fatal(err)
		}
	}
	return srv
}

func checkCalls(t *testing.T, got []any, want ...any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %s, want %s", showAll(got), showAll(want))
	}
}

func showAll(vs []any) string {
	var parts []string
	for _, v := range vs {
		parts = append(parts, show(v))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

var when = time.Date(2026, 10, 16, 21, 10, 0, 0, time.UTC)

func TestHubStatus(t *testing.T) {
	hub := &fakeHub{status: api.HubStatus{Hub: "0123456789abcdef0123456789abcdef", Listen: ptr("127.0.0.1:7777"),
		Vaults: 2, Devices: 3, Connected: 1}}
	srv := syncServer(t, hub, &fakeSync{})
	want := "hub 0123456789abcdef0123456789abcdef\nlistening on 127.0.0.1:7777\n2 vaults, 3 devices, 1 connected\n"
	if out := notes(t, srv, "hub", "status"); out != want {
		t.Fatalf("hub status = %q, want %q", out, want)
	}
	checkCalls(t, hub.took(), &api.HubStatusParams{})
	hub.status = api.HubStatus{}
	want = "hub: none yet (serve a vault first)\nnot listening: start notesd with --hub ADDR\n0 vaults, 0 devices, 0 connected\n"
	if out := notes(t, srv, "hub", "status"); out != want {
		t.Fatalf("hub status of a notesd that never served = %q, want %q", out, want)
	}
	var got api.HubStatus
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "hub", "status")), &got); err != nil || got != hub.status {
		t.Fatalf("hub status --json = %+v, %v", got, err)
	}
}

func TestHubServeAndVaults(t *testing.T) {
	hub := &fakeHub{vault: api.HubVault{VaultID: 2, Sync: "s2", Name: "work", Seq: 0, ServedAt: when}}
	srv := syncServer(t, hub, &fakeSync{}, "garden", "work")
	if out := notes(t, srv, "--vault", "work", "hub", "serve"); out != "serving work as s2\n" {
		t.Fatalf("hub serve = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubServeParams{VaultID: 2})
	if err := notesErr(t, srv, "hub", "serve"); !strings.Contains(err.Error(), "--vault") {
		t.Fatalf("hub serve with two vaults and no --vault = %v", err)
	}
	if out := notes(t, srv, "hub", "vaults"); out != "no vaults served\n" {
		t.Fatalf("hub vaults with none = %q", out)
	}
	hub.took()
	hub.vaults = []api.HubVault{hub.vault, {VaultID: 1, Sync: "s1", Name: "garden", Seq: 42, ServedAt: when}}
	if out := notes(t, srv, "hub", "vaults"); out != "work\ts2\t0\ngarden\ts1\t42\n" {
		t.Fatalf("hub vaults = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubVaultsParams{})
}

func TestHubPendingAndApprove(t *testing.T) {
	hub := &fakeHub{approved: api.PendingPairing{Code: "7K4M-Q2XD", Name: "laptop", Approved: true}}
	srv := syncServer(t, hub, &fakeSync{})
	if out := notes(t, srv, "hub", "pending"); out != "no pending pairings\n" {
		t.Fatalf("hub pending with none = %q", out)
	}
	hub.took()
	hub.pending = []api.PendingPairing{
		{Code: "7K4M-Q2XD", Name: "laptop", Addr: "100.64.0.2:51234", CreatedAt: when.Add(-time.Minute), ExpiresAt: when},
		{Code: "AB12-CD34", Name: "phone", Addr: "100.64.0.3:40000", CreatedAt: when, ExpiresAt: when.Add(time.Minute), Approved: true},
	}
	want := "7K4M-Q2XD\tlaptop\t100.64.0.2:51234\t2026-10-16T21:10:00Z\nAB12-CD34\tphone\t100.64.0.3:40000\t2026-10-16T21:11:00Z\tapproved\n"
	if out := notes(t, srv, "hub", "pending"); out != want {
		t.Fatalf("hub pending = %q, want %q", out, want)
	}
	hub.took()
	if out := notes(t, srv, "hub", "approve", "7k4m q2xd"); out != "approved laptop (7K4M-Q2XD)\n" {
		t.Fatalf("hub approve = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubApproveParams{Code: "7k4m q2xd"})
	hub.err = api.NotFound("no pending pairing has code ZZZZ-ZZZZ")
	if err := notesErr(t, srv, "hub", "approve", "ZZZZ-ZZZZ"); !strings.Contains(err.Error(), "ZZZZ-ZZZZ") {
		t.Fatalf("approving an unknown code = %v", err)
	}
}

func TestHubDevicesAndRevoke(t *testing.T) {
	hub := &fakeHub{}
	srv := syncServer(t, hub, &fakeSync{})
	if out := notes(t, srv, "hub", "devices"); out != "no paired devices\n" {
		t.Fatalf("hub devices with none = %q", out)
	}
	hub.took()
	seen := when.Add(time.Hour)
	hub.devices = []api.HubDevice{
		{ID: 1, Name: "laptop", PairedAt: when, SeenAt: &seen, Connected: true},
		{ID: 2, Name: "desk", PairedAt: when},
		{ID: 3, Name: "old phone", PairedAt: when, SeenAt: &seen, Connected: true, Revoked: true},
	}
	want := "1\tlaptop\t2026-10-16T21:10:00Z\t2026-10-16T22:10:00Z\tconnected\n" +
		"2\tdesk\t2026-10-16T21:10:00Z\tnever\toffline\n" +
		"3\told phone\t2026-10-16T21:10:00Z\t2026-10-16T22:10:00Z\trevoked\n"
	if out := notes(t, srv, "hub", "devices"); out != want {
		t.Fatalf("hub devices = %q, want %q", out, want)
	}
	hub.took()
	if out := notes(t, srv, "hub", "revoke", "3"); out != "revoked device 3\n" {
		t.Fatalf("hub revoke = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubRevokeParams{Device: 3})
	if err := notesErr(t, srv, "hub", "revoke", "laptop"); !strings.Contains(err.Error(), "laptop") {
		t.Fatalf("hub revoke with a name = %v", err)
	}
	if len(hub.took()) != 0 {
		t.Fatal("a revoke with a bad ID still called the hub domain")
	}
}

func TestHubJoinPairsThenJoins(t *testing.T) {
	dev := &fakeSync{
		pairing: api.Pairing{Pairing: "p1", Code: "7K4M-Q2XD", Hub: "h1", URL: "https://hub.example", ExpiresAt: when},
		states:  []api.PairingState{api.PairingStatePending, api.PairingStatePending, api.PairingStateApproved},
		status:  []api.SyncStatus{{VaultID: 1, URL: "https://hub.example", HubVault: "Notes", State: api.SyncStateSyncing}},
	}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	out := notes(t, srv, "hub", "join", "https://hub.example/", "Notes", "--device", "laptop")
	want := "approve this device on the hub: notesctl hub approve 7K4M-Q2XD\n" +
		"paired with hub h1\n" +
		"syncing Notes at https://hub.example: syncing\n"
	if out != want {
		t.Fatalf("hub join = %q, want %q", out, want)
	}
	checkCalls(t, dev.took(),
		&api.SyncHubsParams{},
		&api.SyncPairParams{URL: "https://hub.example", Name: ptr("laptop")},
		&api.SyncPairedParams{Pairing: "p1"},
		&api.SyncPairedParams{Pairing: "p1"},
		&api.SyncPairedParams{Pairing: "p1"},
		&api.SyncJoinParams{VaultID: 1, URL: "https://hub.example", Name: "Notes"},
	)
}

func TestHubJoinWithAPairedHub(t *testing.T) {
	dev := &fakeSync{
		hubs:   []api.SyncHub{{ID: 1, Hub: "h1", URL: "https://hub.example", Device: 4, PairedAt: when}},
		status: []api.SyncStatus{{VaultID: 2, URL: "https://hub.example", HubVault: "Work", State: api.SyncStateIdle}},
	}
	srv := syncServer(t, &fakeHub{}, dev, "garden", "work")
	if out := notes(t, srv, "--vault", "work", "hub", "join", "https://hub.example", "Work"); out != "syncing Work at https://hub.example: idle\n" {
		t.Fatalf("hub join with a paired hub = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncHubsParams{}, &api.SyncJoinParams{VaultID: 2, URL: "https://hub.example", Name: "Work"})
}

func TestHubJoinExpired(t *testing.T) {
	dev := &fakeSync{
		pairing: api.Pairing{Pairing: "p1", Code: "7K4M-Q2XD", Hub: "h1", URL: "https://hub.example", ExpiresAt: when},
		states:  []api.PairingState{api.PairingStateExpired},
	}
	srv := syncServer(t, &fakeHub{}, dev, "garden")
	err := notesErr(t, srv, "hub", "join", "https://hub.example", "Notes")
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("hub join whose code expired = %v", err)
	}
	for _, c := range dev.took() {
		if _, ok := c.(*api.SyncJoinParams); ok {
			t.Fatal("hub join joined after the pairing expired")
		}
	}
}
