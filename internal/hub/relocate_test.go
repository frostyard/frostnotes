package hub_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/hub"
)

var nonce = base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func (h *testHub) relocation(body string) *http.Response {
	h.t.Helper()
	return h.do(http.MethodPost, "/sync/hub", "", strings.NewReader(body), nil)
}

// The hub records one URL for its devices, reports it in hub.status and
// log.hub, and refuses one a device could not use.
func TestTheHubRecordsItsURL(t *testing.T) {
	h := newHub(t)
	ctx := t.Context()
	const moved = "https://lodge.example.ts.net/device/notes"
	st, err := h.c.Hub().SetURL(ctx, &api.HubSetURLParams{URL: "https://Lodge.Example.ts.net/device/notes/"})
	if err != nil || st.URL == nil || *st.URL != moved {
		t.Fatalf("SetURL = %+v, %v; want %s, its host in lower case and without its trailing /", st, err, moved)
	}
	token, _ := h.pair("phone")
	dev, err := h.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	got, err := dev.Log().Hub(ctx, &api.LogHubParams{})
	if err != nil || got.Hub != h.vaultHub() || got.URL == nil || *got.URL != moved {
		t.Fatalf("log.hub = %+v, %v", got, err)
	}
	for _, bad := range []string{"http://lodge.example.ts.net/device/notes", "ftp://x", "https://", "https://u:p@x/", "https://x/?q", "notes"} {
		if _, err := h.c.Hub().SetURL(ctx, &api.HubSetURLParams{URL: bad}); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("SetURL(%q) = %v, want invalidParams", bad, err)
		}
	}
	if st, err := h.c.Hub().SetURL(ctx, &api.HubSetURLParams{URL: ""}); err != nil || st.URL != nil {
		t.Fatalf("forgetting the URL = %+v, %v", st, err)
	}
	if got, err := dev.Log().Hub(ctx, &api.LogHubParams{}); err != nil || got.URL != nil {
		t.Fatalf("log.hub after forgetting = %+v, %v", got, err)
	}
}

func (h *testHub) vaultHub() string {
	h.t.Helper()
	st, err := h.c.Hub().Status(h.t.Context(), &api.HubStatusParams{})
	if err != nil {
		h.t.Fatal(err)
	}
	return st.Hub
}

// POST /sync/hub answers the hub's ID, its recorded URL and an HMAC of the
// nonce that only a holder of the device token's hash can make.
func TestTheRelocationProof(t *testing.T) {
	h := newHub(t)
	token, device := h.pair("phone")
	body := `{"device": ` + itoa(device) + `, "nonce": "` + nonce + `"}`
	if resp := h.relocation(body); resp.StatusCode != http.StatusConflict {
		t.Fatalf("with no recorded URL = %s, want 409", resp.Status)
	}
	const moved = "https://lodge.example.ts.net/device/notes"
	if _, err := h.c.Hub().SetURL(t.Context(), &api.HubSetURLParams{URL: moved}); err != nil {
		t.Fatal(err)
	}
	resp := h.relocation(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /sync/hub = %s", resp.Status)
	}
	got := decode[hub.Relocation](t, resp.Body)
	if got.Hub != h.vaultHub() || got.URL != moved || len(got.Proof) != 43 {
		t.Fatalf("answer = %+v", got)
	}
	if want := hub.Proof(hub.TokenKey(token), got.Hub, moved, nonce); got.Proof != want {
		t.Fatalf("proof = %s, want %s from the device's own token", got.Proof, want)
	}
	if hub.Proof(hub.TokenKey(token+"x"), got.Hub, moved, nonce) == got.Proof ||
		hub.Proof(hub.TokenKey(token), got.Hub, moved+"x", nonce) == got.Proof {
		t.Fatal("the proof does not depend on the token and the URL")
	}
	if strings.Contains(got.Proof, token) || strings.Contains(got.Proof, hub.TokenHash(token)) {
		t.Fatal("the answer holds the token or its hash")
	}

	// An unknown device gets the same answer as a paired one, with a proof
	// no token makes: the endpoint does not tell which devices exist.
	unknown := h.relocation(`{"device": 999, "nonce": "` + nonce + `"}`)
	if unknown.StatusCode != http.StatusOK {
		t.Fatalf("an unknown device = %s, want 200 as for a paired one", unknown.Status)
	}
	if u := decode[hub.Relocation](t, unknown.Body); u.Hub != got.Hub || u.URL != moved || len(u.Proof) != 43 ||
		u.Proof == got.Proof || u.Proof == hub.Proof(hub.TokenKey(token), got.Hub, moved, nonce) {
		t.Errorf("an unknown device's answer = %+v", u)
	}
	// Refusals: a malformed request.
	for name, bad := range map[string]string{
		"a short nonce":     `{"device": 1, "nonce": "` + nonce[:42] + `"}`,
		"a long nonce":      `{"device": 1, "nonce": "` + nonce + `A"}`,
		"a padded nonce":    `{"device": 1, "nonce": "` + base64.URLEncoding.EncodeToString(make([]byte, 32)) + `"}`,
		"a standard nonce":  `{"device": 1, "nonce": "` + strings.Repeat("+", 43) + `"}`,
		"no device":         `{"nonce": "` + nonce + `"}`,
		"an unknown member": `{"device": 1, "nonce": "` + nonce + `", "token": "x"}`,
		"not JSON":          `device=1`,
	} {
		if resp := h.relocation(bad); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %s, want 400", name, resp.Status)
		}
	}
	// The body's limit, at it and past it: padding inside the JSON.
	pad := func(n int) string {
		b := `{"device": ` + itoa(device) + `, "nonce": "` + nonce + `"`
		return b + strings.Repeat(" ", n-len(b)-1) + "}"
	}
	if resp := h.relocation(pad(1024)); resp.StatusCode != http.StatusOK {
		t.Errorf("a 1 KiB request = %s, want 200", resp.Status)
	}
	if resp := h.relocation(pad(1025)); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a request past 1 KiB = %s, want 413", resp.Status)
	}
	// A browser's request is refused, as on every hub endpoint.
	if resp := h.do(http.MethodPost, "/sync/hub", "", strings.NewReader(body), http.Header{"Origin": {"https://evil.example"}}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("with a browser's Origin = %s, want 403", resp.Status)
	}
	if err := h.c.Hub().Revoke(t.Context(), &api.HubRevokeParams{Device: device}); err != nil {
		t.Fatal(err)
	}
	revoked := h.relocation(body)
	if revoked.StatusCode != http.StatusOK {
		t.Fatalf("a revoked device = %s, want 200 as for a paired one", revoked.Status)
	}
	if r := decode[hub.Relocation](t, revoked.Body); r.Proof == hub.Proof(hub.TokenKey(token), r.Hub, moved, nonce) {
		t.Error("a revoked device's token still makes its proof")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestSameURL(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"https://Lodge.Example.ts.net/device/notes", "HTTPS://lodge.example.ts.net/device/notes/", true},
		{"http://127.0.0.1:7880", "http://127.0.0.1:7880/", true},
		{"https://lodge.example.ts.net/device/notes", "https://lodge.example.ts.net/device/Notes", false},
		{"https://lodge.example.ts.net/device/notes", "https://lodge.example.ts.net:8443/device/notes", false},
		{"https://lodge.example.ts.net/device/notes", "http://lodge.example.ts.net/device/notes", false},
		{"https://lodge.example.ts.net/device/notes", "https://relay.example.ts.net/device/notes", false},
		{"https://a.example/x", "https://a.example/x?y", false},
		{"https://u@a.example/x", "https://a.example/x", false},
	} {
		if got := hub.SameURL(c.a, c.b); got != c.same {
			t.Errorf("SameURL(%q, %q) = %t, want %t", c.a, c.b, got, c.same)
		}
	}
}
