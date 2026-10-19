package rpcserver_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/store"
)

// rawConn speaks the wire format directly.
type rawConn struct {
	t  *testing.T
	nc net.Conn
	sc *bufio.Scanner
}

func dialRaw(t *testing.T, socket string) *rawConn {
	t.Helper()
	nc, err := dialConn(socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nc.Close() })
	return &rawConn{t: t, nc: nc, sc: bufio.NewScanner(nc)}
}

func (r *rawConn) send(line string) {
	r.t.Helper()
	if _, err := r.nc.Write([]byte(line + "\n")); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rawConn) recv() string {
	r.t.Helper()
	_ = r.nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !r.sc.Scan() {
		r.t.Fatalf("no reply: %v", r.sc.Err())
	}
	return r.sc.Text()
}

func TestWireRules(t *testing.T) {
	srv := rpctest.Start(t)
	r := dialRaw(t, srv.Socket)

	r.send(`{"jsonrpc":"2.0","id":1,"method":"vault.list"}`)
	if got := r.recv(); !strings.Contains(got, `"code":1004`) || !strings.Contains(got, `"id":1`) {
		t.Fatalf("call before hello = %s, want notReady", got)
	}
	r.send(`{"jsonrpc":"2.0","id":"h","method":"rpc.hello","params":{"protocol":999,"client":"t"}}`)
	if got := r.recv(); !strings.Contains(got, `"code":1003`) {
		t.Fatalf("wrong protocol = %s, want protocolMismatch", got)
	}
	// hello and a pipelined call: the call must see the connection ready.
	r.send(`{"jsonrpc":"2.0","id":2,"method":"rpc.hello","params":{"protocol":1,"client":"t"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"vault.list"}`)
	if got := r.recv(); got != `{"jsonrpc":"2.0","id":2,"result":{"protocol":1,"server":"notesd rpctest","caller":"local"}}` {
		t.Fatalf("hello = %s", got)
	}
	if got := r.recv(); got != `{"jsonrpc":"2.0","id":3,"result":[]}` {
		t.Fatalf("vault.list = %s", got)
	}
	for _, tc := range []struct{ req, want string }{
		{`not json`, `"code":-32700`},
		{`{"jsonrpc":"1.0","id":4,"method":"x"}`, `"code":-32600`},
		{`{"jsonrpc":"2.0","id":5,"method":"no.such"}`, `"code":-32601`},
		{`{"jsonrpc":"2.0","id":6,"method":"vault.remove","params":{}}`, `missing required param \"id\"`},
		{`{"jsonrpc":"2.0","id":7,"method":"vault.remove","params":{"id":1,"extra":true}}`, `"code":-32602`},
		{`{"jsonrpc":"2.0","id":8,"method":"vault.remove","params":[1]}`, `params must be an object`},
	} {
		r.send(tc.req)
		if got := r.recv(); !strings.Contains(got, tc.want) {
			t.Errorf("%s -> %s, want %s", tc.req, got, tc.want)
		}
	}
}

// A connection serves only the methods that admit its caller kind
// (ADR-0008, ADR-0026): a browser session lists vaults, and a local-only
// method looks like one that does not exist.
func TestCallerKindLimitsMethods(t *testing.T) {
	srv := rpctest.Start(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go srv.RPC.ServeConn(t.Context(), server, srv.Router, api.Identity{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"})
	r := &rawConn{t: t, nc: client, sc: bufio.NewScanner(client)}

	r.send(`{"jsonrpc":"2.0","id":1,"method":"rpc.hello","params":{"protocol":1,"client":"browser"}}`)
	if got := r.recv(); !strings.Contains(got, `"caller":"session","login":"ana@example.com","node":"phone"`) {
		t.Fatalf("hello as a session = %s, want its caller, login and node", got)
	}
	r.send(`{"jsonrpc":"2.0","id":2,"method":"vault.list"}`)
	if got := r.recv(); !strings.Contains(got, `"result"`) {
		t.Fatalf("vault.list as a session = %s, want a result", got)
	}
	r.send(`{"jsonrpc":"2.0","id":3,"method":"vault.add","params":{"path":"/tmp/x"}}`)
	if got := r.recv(); !strings.Contains(got, `"code":-32601`) {
		t.Fatalf("vault.add as a session = %s, want methodNotFound", got)
	}
	r.send(`{"jsonrpc":"2.0","id":4,"method":"rpc.ping"}`)
	if got := r.recv(); got != `{"jsonrpc":"2.0","id":4,"result":null}` {
		t.Fatalf("rpc.ping = %s, want a null result", got)
	}
}

func TestEventsResumeAfterReconnect(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	emit := func(id int64) {
		t.Helper()
		if err := srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.Emit(ctx, api.VaultRemoved{ID: id}) }); err != nil {
			t.Fatal(err)
		}
	}
	emit(1)
	emit(2)

	c := srv.Dial(t)
	since := int64(1)
	sub, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{SinceSeq: &since})
	if err != nil || sub.Seq != 2 || sub.Resync {
		t.Fatalf("Subscribe = %+v, %v", sub, err)
	}
	emit(3)
	for _, wantSeq := range []int64{2, 3} {
		select {
		case ev := <-c.Notifications():
			e, err := api.DecodeEvent(ev.Event, ev.Data)
			if err != nil || ev.Seq != wantSeq || e.(api.VaultRemoved).ID != wantSeq {
				t.Fatalf("event = %+v (%v), want seq %d", ev, err, wantSeq)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no event seq %d", wantSeq)
		}
	}
	if _, err := c.Events().Subscribe(ctx, nil); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("second subscribe = %v, want conflict", err)
	}
}

func TestListenRejectsLiveSocket(t *testing.T) {
	srv := rpctest.Start(t)
	if _, err := rpcserver.Listen(srv.Socket); err == nil || !strings.Contains(err.Error(), "already listening") {
		t.Fatalf("Listen on a live socket = %v", err)
	}
}

func TestRepeatedHello(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	if _, err := c.RPC().Hello(context.Background(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "again"}); err != nil {
		t.Fatalf("repeated hello: %v", err)
	}
}

// Events reach a connection only when their domain admits its caller: a
// device on a hub hears the log's announcements and none of the hub's
// local note or vault events.
func TestEventsReachOnlyTheCallersTheirDomainAdmits(t *testing.T) {
	srv := rpctest.Start(t)
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	go srv.RPC.ServeConn(t.Context(), server, srv.Router, api.Identity{Caller: api.CallerDevice})
	r := &rawConn{t: t, nc: client, sc: bufio.NewScanner(client)}
	r.send(`{"jsonrpc":"2.0","id":1,"method":"rpc.hello","params":{"protocol":1,"client":"device"}}`)
	r.recv()
	r.send(`{"jsonrpc":"2.0","id":2,"method":"events.subscribe"}`)
	if got := r.recv(); !strings.Contains(got, `"result"`) {
		t.Fatalf("subscribe as a device = %s", got)
	}
	local, err := api.NewEnvelope(0, api.NoteRemoved{VaultID: 1, Path: "secret.md"})
	if err != nil {
		t.Fatal(err)
	}
	device, err := api.NewEnvelope(0, api.LogAppended{Vault: "0123456789abcdef0123456789abcdef", Seq: 7})
	if err != nil {
		t.Fatal(err)
	}
	srv.Broker.Publish([]api.EventEnvelope{local, device})
	got := r.recv()
	if !strings.Contains(got, `"event":"log.appended"`) || strings.Contains(got, "secret.md") {
		t.Fatalf("first event to a device = %s, want log.appended alone", got)
	}
}
