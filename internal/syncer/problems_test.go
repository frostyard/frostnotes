package syncer_test

import (
	"context"
	"sync/atomic"
	"testing"

	kitrpctest "github.com/frostyard/lodge/kit/rpctest"
	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/problem"
	"github.com/frostyard/frostnotes/internal/problem/problemtest"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// faultyLog is a hub's log domain whose pull answers invalidParams once
// faulty is set: a hub that no longer understands the device.
type faultyLog struct {
	api.LogService
	faulty *atomic.Bool
}

func (l faultyLog) Pull(ctx context.Context, p *api.LogPullParams) (*api.LogPage, error) {
	if l.faulty.Load() {
		return nil, api.InvalidParams("the hub cannot read this pull")
	}
	return l.LogService.Pull(ctx, p)
}

// A session that ends on a protocol fault of the hub is recorded in the
// device's journal as sync.protocol_error, with notesd's version, once
// for the run of sessions that keep failing on it (ADR-0036).
func TestAProtocolFaultOfTheHubIsRecorded(t *testing.T) {
	var faulty atomic.Bool
	ho := rpctest.Options{HubListen: true, Options: kitrpctest.Options[api.Services]{Services: func(s *api.Services) {
		s.Log = faultyLog{LogService: s.Log, faulty: &faulty}
	}}}
	h := start(t, ho, vaultDir(t, map[string]string{"A.md": "a\n"}))
	if _, err := h.c.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: h.vault}); err != nil {
		t.Fatal(err)
	}
	j := problemtest.Listen(t)
	o := deviceOptions("laptop")
	o.Syncer.Problems = problems.New(problemtest.AppID, "1.2.3", problems.ComponentDaemon, problems.WithSocket(j.Path))
	d := pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "laptop")
	settle(t, h, d)
	j.None(t) // a session that works records nothing

	faulty.Store(true)
	h.write("B.md", "b\n") // the hub announces it, and the device pulls
	problemtest.Check(t, j.Next(t), problemtest.Want{Version: "1.2.3", Class: "error", Code: problem.SyncProtocolError})
}
