package lodgeapp_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/lodge/kit/kittest"
	kit "github.com/frostyard/lodge/kit/lodgeapp"
	"github.com/frostyard/lodge/kit/manifest"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/lodgeapp"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// notes is a notesd serving one vault with Plan.md, on a lodge app socket.
type notes struct {
	srv   *rpctest.Server
	lodge *kittest.Lodge
	local *api.Client
	vault int64
}

func start(t *testing.T) *notes {
	t.Helper()
	srv := rpctest.Start(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Plan.md"), []byte("plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := srv.Dial(t)
	v, err := local.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Note().Read(t.Context(), &api.NoteReadParams{VaultID: v.ID, Path: "Plan.md"}); err != nil {
		t.Fatal(err)
	}
	l := kittest.Start(t, lodgeapp.ID, lodgeapp.Server(lodgeapp.Config{RPC: srv.RPC, Router: srv.Router,
		Blobs: srv.Blobs, Hub: srv.Hub, Version: "test"}))
	return &notes{srv: srv, lodge: l, local: local, vault: v.ID}
}

// The socket keeps lodge's rules: the peer check, one caller kind per
// path, /health, and the routes the manifest promises.
func TestTheSocketKeepsLodgesRules(t *testing.T) {
	n := start(t)
	data, err := os.ReadFile("../../packaging/lodge/app.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != lodgeapp.ID || m.User != "frostnotes" || !m.Content || !m.Device || !m.MCP || !m.RemoteImages || m.ContentLinks {
		t.Fatalf("app.json = %+v, not lodge-app.md's", m)
	}
	kittest.CheckRules(t, n.lodge, m)
	h, err := n.lodge.Health(t.Context())
	if err != nil || !h.OK || h.Version != "test" || h.Protocol != api.Protocol {
		t.Fatalf("health = %+v, %v", h, err)
	}
	if code, _ := n.lodge.Get(t, "/nothing", kittest.X()); code != http.StatusForbidden {
		t.Errorf("an unknown path = %d, want 403: it admits no caller", code)
	}
}

// dialSession opens /api as caller and says hello.
func dialSession(t *testing.T, n *notes, c kit.Caller) (*api.Client, *api.Hello) {
	t.Helper()
	nc, err := n.lodge.DialAPI(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	cl := api.NewClient(nc)
	t.Cleanup(func() { _ = cl.Close() })
	h, err := cl.RPC().Hello(t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "web"})
	if err != nil {
		t.Fatal(err)
	}
	return cl, h
}

// A session over the socket reads, writes as its node, receives events,
// and may not call what only local callers may.
func TestASessionReadsWritesAndListens(t *testing.T) {
	n := start(t)
	ctx := t.Context()
	cl, hello := dialSession(t, n, kittest.Session())
	if hello.Caller != "session" || ptr(hello.Login) != kittest.Login || ptr(hello.DisplayName) != kittest.DisplayName ||
		ptr(hello.Node) != kittest.NodeName {
		t.Fatalf("hello = %+v", hello)
	}
	if _, err := cl.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	note, err := cl.Note().Read(ctx, &api.NoteReadParams{VaultID: n.vault, Path: "Plan.md"})
	if err != nil || note.Content != "plan\n" {
		t.Fatalf("read = %+v, %v", note, err)
	}
	author, agent := "mallory", api.VersionSourceAgent
	if _, err := cl.Note().Write(ctx, &api.NoteWriteParams{VaultID: n.vault, Path: "Plan.md", Base: note.Hash,
		Content: "plan, from the phone\n", Source: &agent, Author: &author}); err != nil {
		t.Fatal(err)
	}
	vs, err := n.local.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: n.vault, Path: "Plan.md"})
	if err != nil || vs[0].Author != kittest.NodeName+" (web)" || vs[0].Source != api.VersionSourceEditor {
		t.Fatalf("the session's write is by %+v, %v; want its node", vs[0], err)
	}
	if !waitChanged(t, cl, n.vault, "Plan.md") {
		t.Fatal("the session got no note.changed for its write")
	}
	if err := cl.Call(ctx, "vault.add", map[string]any{"path": t.TempDir()}, nil); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("vault.add as a session = %v, want methodNotFound", err)
	}
	if err := cl.RPC().Ping(ctx, &api.RPCPingParams{}); err != nil {
		t.Errorf("rpc.ping = %v", err)
	}
	// Display headers name the node; nothing else about the session changes.
	other := kittest.Session()
	other.DisplayName, other.NodeName = "", "laptop"
	cl2, hello2 := dialSession(t, n, other)
	if hello2.DisplayName != nil || ptr(hello2.Node) != "laptop" {
		t.Fatalf("hello without a display name = %+v", hello2)
	}
	if _, err := cl2.Note().Read(ctx, &api.NoteReadParams{VaultID: n.vault, Path: "Plan.md"}); err != nil {
		t.Errorf("read on a second session = %v", err)
	}
}

func waitChanged(t *testing.T, c *api.Client, vault int64, path string) bool {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case ev := <-c.Notifications():
			e, err := api.DecodeEvent(ev.Event, ev.Data)
			if ch, ok := e.(api.NoteChanged); err == nil && ok && ch.VaultID == vault && ch.Path == path {
				return true
			}
		case <-timeout:
			return false
		}
	}
}

func ptr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// /content/blobs/ answers lodge-content with the file by its hash, the
// headers that keep it passive, and its limits at and past their values.
func TestContentServesBlobsWithinTheirLimits(t *testing.T) {
	n := start(t)
	put := func(data []byte) string {
		t.Helper()
		sum := sha256.Sum256(data)
		h, err := n.srv.Blobs.Put(t.Context(), bytes.NewReader(data))
		if err != nil || h != hex.EncodeToString(sum[:]) {
			t.Fatalf("Put = %s, %v", h, err)
		}
		return h
	}
	get := func(path, rng string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := n.lodge.Do(req, kittest.Content())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	small := put([]byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>"))
	resp := get("/content/blobs/"+small+".svg", "")
	h := resp.Header
	if resp.StatusCode != http.StatusOK || h.Get("Content-Type") != "image/svg+xml" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; sandbox" ||
		h.Get("Cache-Control") != "private, max-age=31536000, immutable" || h.Get("Content-Disposition") != "" {
		t.Fatalf("GET a blob = %s %v", resp.Status, h)
	}
	if d := get("/content/blobs/"+small+".svg?download=1", "").Header.Get("Content-Disposition"); d != `attachment; filename="`+small+`.svg"` {
		t.Errorf("with ?download=1, Content-Disposition = %q", d)
	}
	for _, bad := range []string{"/content/blobs/" + small, "/content/blobs/" + strings.ToUpper(small) + ".svg",
		"/content/blobs/" + small + ".toolongext", "/content/" + small + ".svg", "/content/blobs/" + strings.Repeat("0", 64) + ".png"} {
		if code := get(bad, "").StatusCode; code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", bad, code)
		}
	}
	if code, _ := n.lodge.Get(t, "/content/blobs/"+small+".svg", kittest.X()); code != http.StatusForbidden {
		t.Errorf("a blob as a session = %d, want 403: content is lodge-content's", code)
	}
	if code, _ := n.lodge.Request(t, http.MethodPut, "/content/blobs/"+small+".svg", kittest.Content()); code != http.StatusMethodNotAllowed {
		t.Errorf("PUT a blob = %d, want 405", code)
	}

	// 64 MiB answers whole; one byte more needs a range.
	whole := bytes.Repeat([]byte{'a'}, lodgeapp.MaxWhole)
	atLimit := put(whole)
	past := put(append(whole, 'b'))
	if resp := get("/content/blobs/"+atLimit+".bin", ""); resp.StatusCode != http.StatusOK || resp.ContentLength != lodgeapp.MaxWhole {
		t.Errorf("a 64 MiB blob = %s, %d bytes; want it whole", resp.Status, resp.ContentLength)
	}
	if code := get("/content/blobs/"+past+".bin", "").StatusCode; code != http.StatusRequestEntityTooLarge {
		t.Errorf("a blob past 64 MiB without a range = %d, want 413", code)
	}
	// A 206 carries at most 8 MiB: a range of exactly that is whole, a
	// longer or open one is cut.
	for rng, want := range map[string]string{
		"bytes=0-8388607": "bytes 0-8388607/67108865",
		"bytes=0-8388608": "bytes 0-8388607/67108865",
		"bytes=10-":       "bytes 10-8388617/67108865",
		"bytes=-5":        "bytes 67108860-67108864/67108865",
	} {
		resp := get("/content/blobs/"+past+".bin", rng)
		if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != want {
			t.Errorf("Range %s = %s %q, want 206 %q", rng, resp.Status, resp.Header.Get("Content-Range"), want)
		}
		if n, _ := io.Copy(io.Discard, resp.Body); n > lodgeapp.MaxRange {
			t.Errorf("Range %s carried %d bytes", rng, n)
		}
	}
	for _, rng := range []string{"bytes=0-1,4-5", "bytes=99999999-", "items=0-1", "bytes=5-1"} {
		if code := get("/content/blobs/"+small+".svg", rng).StatusCode; code != http.StatusRequestedRangeNotSatisfiable {
			t.Errorf("Range %s = %d, want 416", rng, code)
		}
	}
}

// relay is lodge's /device/notes on loopback for a device's notesd: it
// forwards to the socket's /device/ with lodge's device headers, which name
// the node c says.
func relay(t *testing.T, l *kittest.Lodge, c kit.Caller) string {
	t.Helper()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", l.Socket)
	}}
	t.Cleanup(tr.CloseIdleConnections)
	rp := &httputil.ReverseProxy{Transport: tr, Rewrite: func(pr *httputil.ProxyRequest) {
		pr.Out.URL.Scheme, pr.Out.URL.Host, pr.Out.Host = "http", "lodge", "lodge"
		pr.Out.URL.Path = "/device" + strings.TrimPrefix(pr.In.URL.Path, "/device/notes")
		pr.Out.URL.RawPath = ""
		c.SetHeaders(pr.Out.Header)
	}}
	srv := httptest.NewServer(rp)
	t.Cleanup(srv.Close)
	return srv.URL + "/device/notes"
}

// A device pairs, pushes and pulls through /device/ exactly as it does on
// the hub's loopback address; lodge's peer headers only name it beside its
// pending pairing.
func TestADeviceSyncsThroughDevice(t *testing.T) {
	n := start(t)
	ctx := t.Context()
	if _, err := n.local.Hub().Serve(ctx, &api.HubServeParams{VaultID: n.vault}); err != nil {
		t.Fatal(err)
	}
	hubURL := relay(t, n.lodge, kittest.Device())
	dev := rpctest.StartWith(t, rpctest.Options{Syncer: syncer.Config{Device: "laptop", Quiet: time.Millisecond,
		MaxQuiet: 10 * time.Millisecond, Idle: time.Second, Backoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond}})
	dc := dev.Dial(t)
	devWatch, hubWatch := watch(t, dev), watch(t, n.srv)
	dir := t.TempDir()
	dv, err := dc.Vault().Add(ctx, &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	p, err := dc.Sync().Pair(ctx, &api.SyncPairParams{URL: hubURL})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := n.local.Hub().Pending(ctx, &api.HubPendingParams{})
	if err != nil || len(pending) != 1 || pending[0].Addr != kittest.NodeName+" ("+kittest.Login+")" || pending[0].Name != "laptop" {
		t.Fatalf("pending = %+v, %v; want the node lodge named", pending, err)
	}
	if _, err := n.local.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		t.Fatal(err)
	}
	if r, err := dc.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing}); err != nil || r.State != api.PairingStateApproved {
		t.Fatalf("paired = %+v, %v", r, err)
	}
	if _, err := dc.Sync().Join(ctx, &api.SyncJoinParams{VaultID: dv.ID, URL: hubURL, Name: filepath.Base(n.vaultDir(t))}); err != nil {
		t.Fatal(err)
	}
	if !waitChanged(t, devWatch, dv.ID, "Plan.md") {
		t.Fatal("the device did not pull Plan.md")
	}
	note, err := dc.Note().Read(ctx, &api.NoteReadParams{VaultID: dv.ID, Path: "Plan.md"})
	if err != nil || note.Content != "plan\n" {
		t.Fatalf("the device's Plan.md = %+v, %v", note, err)
	}
	if _, err := dc.Note().Write(ctx, &api.NoteWriteParams{VaultID: dv.ID, Path: "Plan.md", Base: note.Hash, Content: "plan\nfrom the laptop\n"}); err != nil {
		t.Fatal(err)
	}
	if !waitChanged(t, hubWatch, n.vault, "Plan.md") {
		t.Fatal("the device's push did not reach the hub")
	}
	if got, err := os.ReadFile(filepath.Join(n.vaultDir(t), "Plan.md")); err != nil || string(got) != "plan\nfrom the laptop\n" {
		t.Fatalf("the hub's Plan.md = %q, %v", got, err)
	}

	// The device's token decides; lodge's display headers never do.
	for _, c := range []kit.Caller{kittest.Device(), {Kind: kit.Device}, {Kind: kit.Device, NodeName: "hub", Login: "root@example.com"}} {
		code, _ := n.lodge.Get(t, "/device/sync/blobs/"+strings.Repeat("0", 64), c)
		if code != http.StatusUnauthorized {
			t.Errorf("a blob without a token, as %+v = %d, want 401", c, code)
		}
	}
	for _, path := range []string{"/device/", "/device/other", "/device/syncx", "/device/notes/sync"} {
		if code, _ := n.lodge.Get(t, path, kittest.Device()); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
}

func (n *notes) vaultDir(t *testing.T) string {
	t.Helper()
	vs, err := n.local.Vault().List(t.Context(), &api.VaultListParams{})
	if err != nil || len(vs) != 1 {
		t.Fatalf("vaults = %+v, %v", vs, err)
	}
	return vs[0].Path
}

// watch returns a local client of srv subscribed to its events.
func watch(t *testing.T, srv *rpctest.Server) *api.Client {
	t.Helper()
	c := srv.Dial(t)
	if _, err := c.Events().Subscribe(t.Context(), &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	return c
}
