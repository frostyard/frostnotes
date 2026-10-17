package hub_test

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// testHub is a hub serving one vault with a.md.
type testHub struct {
	t     *testing.T
	srv   *rpctest.Server
	c     *api.Client
	vault api.HubVault
}

func newHub(t *testing.T) *testHub { return newHubWith(t, nil) }

// newHubWith is newHub with a hook at the hub's crash points.
func newHubWith(t *testing.T, crash func(point string) bool) *testHub {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := rpctest.StartWith(t, rpctest.Options{HubListen: true, Crash: crash})
	c := srv.Dial(t)
	v, err := c.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	// The scan must have indexed a.md before the vault is served.
	if _, err := c.Note().Read(t.Context(), &api.NoteReadParams{VaultID: v.ID, Path: "a.md"}); err != nil {
		t.Fatal(err)
	}
	hv, err := c.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: v.ID})
	if err != nil {
		t.Fatal(err)
	}
	return &testHub{t: t, srv: srv, c: c, vault: *hv}
}

func (h *testHub) do(method, path, token string, body io.Reader, header http.Header) *http.Response {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.srv.HubURL+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, r io.Reader) T {
	t.Helper()
	var v T
	if err := json.UnmarshalRead(r, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

type pairing struct {
	Pairing   string    `json:"pairing"`
	Secret    string    `json:"secret"`
	Code      string    `json:"code"`
	Hub       string    `json:"hub"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// pair runs the pairing exchange as a device would and returns the token.
func (h *testHub) pair(name string) (string, int64) {
	h.t.Helper()
	resp := h.do(http.MethodPost, "/sync/pair", "", strings.NewReader(`{"name": "`+name+`"}`), nil)
	if resp.StatusCode != http.StatusCreated {
		h.t.Fatalf("POST /sync/pair = %s", resp.Status)
	}
	p := decode[pairing](h.t, resp.Body)
	if len(p.Code) != 9 || p.Code[4] != '-' || len(p.Secret) != 43 || len(p.Hub) != 32 {
		h.t.Fatalf("pairing = %+v", p)
	}
	// Approval is local only, and takes the code as a user types it.
	typed := strings.ToLower(strings.ReplaceAll(p.Code, "-", " "))
	if _, err := h.c.Hub().Approve(h.t.Context(), &api.HubApproveParams{Code: typed}); err != nil {
		h.t.Fatal(err)
	}
	resp = h.do(http.MethodGet, "/sync/pair/"+p.Pairing, p.Secret, nil, nil)
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("collecting = %s", resp.Status)
	}
	got := decode[struct {
		State  string `json:"state"`
		Token  string `json:"token"`
		Hub    string `json:"hub"`
		Device int64  `json:"device"`
	}](h.t, resp.Body)
	if got.State != "approved" || !strings.HasPrefix(got.Token, "fnd_") || got.Hub != p.Hub || got.Device == 0 {
		h.t.Fatalf("collected = %+v", got)
	}
	if again := h.do(http.MethodGet, "/sync/pair/"+p.Pairing, p.Secret, nil, nil); again.StatusCode != http.StatusNotFound {
		h.t.Fatalf("collecting twice = %s, want 404: a token is handed out once", again.Status)
	}
	return got.Token, got.Device
}

// dial opens the /sync WebSocket with token as a device does.
func (h *testHub) dial(token string) (*api.Client, error) {
	h.t.Helper()
	cfg, err := websocket.NewConfig(strings.Replace(h.srv.HubURL, "http", "ws", 1)+"/sync", hub.DeviceOrigin)
	if err != nil {
		h.t.Fatal(err)
	}
	cfg.Header.Set("Authorization", "Bearer "+token)
	ws, err := cfg.DialContext(h.t.Context())
	if err != nil {
		return nil, err
	}
	ws.PayloadType = websocket.TextFrame
	c := api.NewClient(ws)
	h.t.Cleanup(func() { _ = c.Close() })
	if _, err := c.RPC().Hello(h.t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "test device"}); err != nil {
		return nil, err
	}
	return c, nil
}

func TestPairingAndTheTokenCheck(t *testing.T) {
	h := newHub(t)
	ctx := t.Context()
	if p, err := h.c.Hub().Pending(ctx, &api.HubPendingParams{}); err != nil || len(p) != 0 {
		t.Fatalf("pending with none = %+v, %v", p, err)
	}
	token, device := h.pair("laptop")
	ds, err := h.c.Hub().Devices(ctx, &api.HubDevicesParams{})
	if err != nil || len(ds) != 1 || ds[0].ID != device || ds[0].Name != "laptop" {
		t.Fatalf("devices = %+v, %v", ds, err)
	}
	if raw, _ := h.srv.DB.DeviceByToken(ctx, hub.TokenHash(token)); raw.TokenHash == token {
		t.Fatal("the hub stored the token, not its hash")
	}
	c, err := h.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := c.Log().Vaults(ctx, &api.LogVaultsParams{})
	if err != nil || len(vs) != 1 || vs[0].Sync != h.vault.Sync {
		t.Fatalf("log.vaults = %+v, %v", vs, err)
	}
	// A device reaches the log domain only: the hub's local domains look
	// like methods that do not exist.
	if _, err := c.Vault().List(ctx, &api.VaultListParams{}); err == nil {
		t.Fatal("a device listed the hub's vaults")
	}
	if _, err := c.Hub().Devices(ctx, &api.HubDevicesParams{}); err == nil {
		t.Fatal("a device listed the hub's devices")
	}
	if _, err := h.dial("fnd_wrong"); err == nil {
		t.Fatal("a wrong token opened /sync")
	}
	if resp := h.do(http.MethodHead, "/sync/blobs/"+strings.Repeat("0", 64), "fnd_wrong", nil, nil); resp.StatusCode != http.StatusUnauthorized ||
		resp.Header.Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("a blob request with a wrong token = %s", resp.Status)
	}
	for _, origin := range []string{"https://evil.example", "null"} {
		resp := h.do(http.MethodPost, "/sync/pair", "", strings.NewReader(`{"name": "x"}`), http.Header{"Origin": {origin}})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("a request with Origin %s = %s, want 403", origin, resp.Status)
		}
	}
	// Revoking closes the connection, and the token stops working.
	if err := h.c.Hub().Revoke(ctx, &api.HubRevokeParams{Device: device}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("revocation did not close the device's connection")
	}
	if _, err := h.dial(token); err == nil {
		t.Fatal("a revoked token opened /sync")
	}
}

func TestPairingRefusals(t *testing.T) {
	h := newHub(t)
	for name, body := range map[string]string{
		"not JSON":        "laptop",
		"an empty name":   `{"name": ""}`,
		"an unknown key":  `{"name": "a", "admin": true}`,
		"a control char":  `{"name": "a\u0007"}`,
		"a 65-byte name":  `{"name": "` + strings.Repeat("x", 65) + `"}`,
		"over a kibibyte": `{"name": "a", "pad": "` + strings.Repeat("x", 1100) + `"}`,
	} {
		resp := h.do(http.MethodPost, "/sync/pair", "", strings.NewReader(body), nil)
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: %s, want 400 or 413", name, resp.Status)
		}
	}
	for i := range 8 {
		if resp := h.do(http.MethodPost, "/sync/pair", "", strings.NewReader(`{"name": "d`+string(rune('0'+i))+`"}`), nil); resp.StatusCode != http.StatusCreated {
			t.Fatalf("pairing %d = %s", i, resp.Status)
		}
	}
	if resp := h.do(http.MethodPost, "/sync/pair", "", strings.NewReader(`{"name": "ninth"}`), nil); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("a ninth pending pairing = %s, want 429", resp.Status)
	}
	if _, err := h.c.Hub().Approve(t.Context(), &api.HubApproveParams{Code: "ZZZZ-ZZZZ"}); err == nil {
		t.Fatal("an unknown code was approved")
	}
	if resp := h.do(http.MethodGet, "/sync/pair/nope", "secret", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("collecting an unknown pairing = %s", resp.Status)
	}
}

func TestARepeatedPushGetsItsFirstAnswer(t *testing.T) {
	h := newHub(t)
	ctx := t.Context()
	token, _ := h.pair("laptop")
	c, err := h.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := c.Log().Join(ctx, &api.LogJoinParams{Vault: h.vault.Sync})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("from the device\n")
	hash := blobs.ID(content)
	resp := h.do(http.MethodPut, "/sync/blobs/"+hash, token, bytes.NewReader(content), nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload = %s", resp.Status)
	}
	item := api.PushItem{Push: 1, Path: "new.md", Hash: &hash, Source: api.VersionSourceEditor, Author: "app", At: time.Now()}
	push := func() api.PushAnswer {
		t.Helper()
		as, err := c.Log().Push(ctx, &api.LogPushParams{Vault: h.vault.Sync, Replica: rep.ID, Items: []api.PushItem{item}})
		if err != nil || len(as) != 1 {
			t.Fatalf("push = %+v, %v", as, err)
		}
		return as[0]
	}
	first := push()
	if first.Status != api.PushStatusAccepted || first.File == nil || first.File.Path != "new.md" {
		t.Fatalf("first answer = %+v", first)
	}
	vs, _ := c.Log().Vaults(ctx, &api.LogVaultsParams{})
	again := push()
	if again.Status != first.Status || again.File == nil || again.File.File != first.File.File {
		t.Fatalf("repeated answer = %+v, want %+v", again, first)
	}
	if after, _ := c.Log().Vaults(ctx, &api.LogVaultsParams{}); after[0].Seq != vs[0].Seq {
		t.Fatal("a repeated push appended to the log")
	}
	recs, err := c.Log().Pushes(ctx, &api.LogPushesParams{Vault: h.vault.Sync, Replica: rep.ID, Pushes: []int64{1, 2}})
	if err != nil || len(recs[0].Revisions) != 1 || len(recs[1].Revisions) != 0 {
		t.Fatalf("log.pushes = %+v, %v", recs, err)
	}
	// Another device's replica is not this one's.
	other, _ := h.pair("desk")
	c2, err := h.dial(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c2.Log().Push(ctx, &api.LogPushParams{Vault: h.vault.Sync, Replica: rep.ID, Items: []api.PushItem{item}}); err == nil {
		t.Fatal("a device pushed through another device's replica")
	}
	// A pushed path out of the vault is refused, not written.
	bad := item
	bad.Push, bad.Path = 2, "../escaped.md"
	as, err := c.Log().Push(ctx, &api.LogPushParams{Vault: h.vault.Sync, Replica: rep.ID, Items: []api.PushItem{bad}})
	if err != nil || as[0].Status != api.PushStatusRefused || as[0].Reason == nil || *as[0].Reason != api.RefuseReasonInvalid {
		t.Fatalf("a push out of the vault = %+v, %v", as, err)
	}
}
