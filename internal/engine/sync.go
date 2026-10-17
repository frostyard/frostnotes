package engine

import (
	"context"

	"github.com/frostyard/frostnotes/api"
)

// The sync domains (docs/specs/sync-protocol.md) are internal/hub's and
// internal/syncer's; an engine built without them answers unavailable.

// Hub implements the hub domain.
func (e *Engine) Hub() api.HubService {
	if e.hub == nil {
		return unbuiltHub{}
	}
	return e.hub.Service()
}

// Sync implements the sync domain.
func (e *Engine) Sync() api.SyncService {
	if e.sync == nil {
		return unbuiltSync{}
	}
	return e.sync.Service()
}

// Log implements the log domain.
func (e *Engine) Log() api.LogService {
	if e.hub == nil {
		return unbuiltLog{}
	}
	return e.hub.Log()
}

func unbuilt(method string) error {
	return api.Unavailable("%s needs the hub or the syncer, which this notesd does not run", method)
}

// unbuiltHub answers every hub method unavailable (on a hub).
type unbuiltHub struct{}

var _ api.HubService = unbuiltHub{}

func (unbuiltHub) Status(context.Context, *api.HubStatusParams) (*api.HubStatus, error) {
	return nil, unbuilt("hub.status")
}

func (unbuiltHub) Serve(context.Context, *api.HubServeParams) (*api.HubVault, error) {
	return nil, unbuilt("hub.serve")
}

func (unbuiltHub) Vaults(context.Context, *api.HubVaultsParams) ([]api.HubVault, error) {
	return nil, unbuilt("hub.vaults")
}

func (unbuiltHub) Pending(context.Context, *api.HubPendingParams) ([]api.PendingPairing, error) {
	return nil, unbuilt("hub.pending")
}

func (unbuiltHub) Approve(context.Context, *api.HubApproveParams) (*api.PendingPairing, error) {
	return nil, unbuilt("hub.approve")
}

func (unbuiltHub) Devices(context.Context, *api.HubDevicesParams) ([]api.HubDevice, error) {
	return nil, unbuilt("hub.devices")
}

func (unbuiltHub) Revoke(context.Context, *api.HubRevokeParams) error {
	return unbuilt("hub.revoke")
}

// unbuiltSync answers every sync method unavailable (on a device).
type unbuiltSync struct{}

var _ api.SyncService = unbuiltSync{}

func (unbuiltSync) Pair(context.Context, *api.SyncPairParams) (*api.Pairing, error) {
	return nil, unbuilt("sync.pair")
}

func (unbuiltSync) Paired(context.Context, *api.SyncPairedParams) (*api.PairResult, error) {
	return nil, unbuilt("sync.paired")
}

func (unbuiltSync) Hubs(context.Context, *api.SyncHubsParams) ([]api.SyncHub, error) {
	return nil, unbuilt("sync.hubs")
}

func (unbuiltSync) Join(context.Context, *api.SyncJoinParams) (*api.SyncStatus, error) {
	return nil, unbuilt("sync.join")
}

func (unbuiltSync) Leave(context.Context, *api.SyncLeaveParams) error {
	return unbuilt("sync.leave")
}

func (unbuiltSync) Status(context.Context, *api.SyncStatusParams) ([]api.SyncStatus, error) {
	return nil, unbuilt("sync.status")
}

func (unbuiltSync) Conflicts(context.Context, *api.SyncConflictsParams) ([]api.SyncConflict, error) {
	return nil, unbuilt("sync.conflicts")
}

func (unbuiltSync) Resolve(context.Context, *api.SyncResolveParams) error {
	return unbuilt("sync.resolve")
}

func (unbuiltSync) Reset(context.Context, *api.SyncResetParams) (*api.SyncStatus, error) {
	return nil, unbuilt("sync.reset")
}

func (unbuiltSync) Verify(context.Context, *api.SyncVerifyParams) (*api.VerifyReport, error) {
	return nil, unbuilt("sync.verify")
}

func (unbuiltSync) History(context.Context, *api.SyncHistoryParams) ([]api.Revision, error) {
	return nil, unbuilt("sync.history")
}

// unbuiltLog answers every log method unavailable (for devices).
type unbuiltLog struct{}

var _ api.LogService = unbuiltLog{}

func (unbuiltLog) Vaults(context.Context, *api.LogVaultsParams) ([]api.LogVault, error) {
	return nil, unbuilt("log.vaults")
}

func (unbuiltLog) Join(context.Context, *api.LogJoinParams) (*api.Replica, error) {
	return nil, unbuilt("log.join")
}

func (unbuiltLog) Missing(context.Context, *api.LogMissingParams) ([]string, error) {
	return nil, unbuilt("log.missing")
}

func (unbuiltLog) Push(context.Context, *api.LogPushParams) ([]api.PushAnswer, error) {
	return nil, unbuilt("log.push")
}

func (unbuiltLog) Pull(context.Context, *api.LogPullParams) (*api.LogPage, error) {
	return nil, unbuilt("log.pull")
}

func (unbuiltLog) Match(context.Context, *api.LogMatchParams) ([]api.LogMatch, error) {
	return nil, unbuilt("log.match")
}

func (unbuiltLog) History(context.Context, *api.LogHistoryParams) ([]api.Revision, error) {
	return nil, unbuilt("log.history")
}

func (unbuiltLog) Pushes(context.Context, *api.LogPushesParams) ([]api.PushRecord, error) {
	return nil, unbuilt("log.pushes")
}

func (unbuiltLog) Resolve(context.Context, *api.LogResolveParams) (*api.LogFile, error) {
	return nil, unbuilt("log.resolve")
}
