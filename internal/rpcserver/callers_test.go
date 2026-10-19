package rpcserver_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// admitted is specs/lodge-app.md's Methods and callers, written out: each
// domain's callers, and the methods that narrow them.
func admitted(method string) []api.Caller {
	local, session, device, agent := api.CallerLocal, api.CallerSession, api.CallerDevice, api.CallerAgent
	switch method {
	case "vault.add", "vault.remove", "hub.serve", "hub.approve", "hub.setURL":
		return []api.Caller{local}
	}
	domain, _, _ := strings.Cut(method, ".")
	switch domain {
	case "rpc":
		return []api.Caller{local, session, device, agent}
	case "events":
		return []api.Caller{local, session, device}
	case "note":
		return []api.Caller{local, session, agent}
	case "vault", "hub":
		return []api.Caller{local, session}
	case "log":
		return []api.Caller{device}
	case "sync":
		return []api.Caller{local}
	}
	return nil
}

// Every method answers methodNotFound to exactly the caller kinds the
// spec does not admit, as a method that does not exist does. Params are
// empty, so an admitted call stops at its params or finds nothing to do.
func TestEveryMethodAdmitsItsCallers(t *testing.T) {
	srv := rpctest.Start(t)
	kinds := []api.Identity{
		{Caller: api.CallerLocal},
		{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"},
		{Caller: api.CallerDevice},
		{Caller: api.CallerAgent, Token: "claude", Scope: &api.Scope{Vault: 1, Write: true}},
	}
	for _, id := range kinds {
		c := srv.DialAs(t, id)
		for _, m := range api.Methods {
			if m == "rpc.hello" {
				continue // DialAs said hello
			}
			want := admitted(m)
			if want == nil {
				t.Errorf("%s is not in the spec's table", m)
				continue
			}
			err := c.Call(t.Context(), m, map[string]any{}, nil)
			refused := errors.Is(err, api.ErrMethodNotFound)
			if refused == slices.Contains(want, id.Caller) {
				t.Errorf("%s as %s = %v; the spec admits %v", m, id.Caller, err, want)
			}
			if refused && !strings.Contains(err.Error(), "does not exist") {
				t.Errorf("%s as %s = %v, want the answer for a method that does not exist", m, id.Caller, err)
			}
		}
	}
}
