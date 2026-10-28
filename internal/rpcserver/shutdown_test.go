package rpcserver_test

import (
	"errors"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// rpc.shutdown answers its caller first and then stops the server: the
// caller has its result, and Serve returns after it.
func TestShutdownAnswersThenStops(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	select {
	case <-srv.Stopped():
		t.Fatal("the server stopped before rpc.shutdown")
	default:
	}
	if err := c.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); err != nil {
		t.Fatalf("rpc.shutdown = %v; want its result", err)
	}
	<-srv.Stopped() // Serve returned: the listener closed and every connection ended
	if c2, _, err := api.Dial(t.Context(), srv.Socket, "late"); err == nil {
		_ = c2.Close()
		t.Fatal("the socket answers after rpc.shutdown")
	}
}

// Only the local caller may stop notesd: a session or an agent gets
// methodNotFound, and the server goes on.
func TestShutdownIsLocalOnly(t *testing.T) {
	srv := rpctest.Start(t)
	for _, id := range []api.Identity{
		{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"},
		{Caller: api.CallerDevice},
		{Caller: api.CallerAgent, Token: "claude", Scope: &api.Scope{Vault: 1, Write: true}},
	} {
		c := srv.DialAs(t, id)
		if err := c.RPC().Shutdown(t.Context(), &api.RPCShutdownParams{}); !errors.Is(err, api.ErrMethodNotFound) {
			t.Fatalf("rpc.shutdown as %s = %v; want methodNotFound", id.Caller, err)
		}
	}
	select {
	case <-srv.Stopped():
		t.Fatal("a caller that may not shut down notesd stopped it")
	default:
	}
	if err := srv.Dial(t).RPC().Ping(t.Context(), &api.RPCPingParams{}); err != nil {
		t.Fatalf("rpc.ping after the refused calls = %v", err)
	}
}

// A server with no Shutdown function cannot be stopped by a call.
func TestShutdownWithoutAHookIsUnavailable(t *testing.T) {
	s := rpcserver.New(rpcserver.Options{})
	if err := s.Shutdown(t.Context(), &api.RPCShutdownParams{}); !errors.Is(err, api.ErrUnavailable) {
		t.Fatalf("Shutdown = %v; want unavailable", err)
	}
}

func TestHelloReportsTheBuildWhenKnown(t *testing.T) {
	hello := func(o rpctest.Options) *api.Hello {
		h, err := rpctest.StartWith(t, o).Dial(t).RPC().Hello(t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "t"})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	if h := hello(rpctest.Options{Build: "abc123"}); h.Build == nil || *h.Build != "abc123" {
		t.Fatalf("hello build = %v; want abc123", h.Build)
	}
	if h := hello(rpctest.Options{}); h.Build != nil {
		t.Fatalf("hello build = %q; want none", *h.Build)
	}
}
