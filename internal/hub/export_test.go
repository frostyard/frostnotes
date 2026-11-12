package hub

import (
	"context"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// PushAs runs log.push as device d, from a context the caller holds: a test
// ends that context in the middle of the call, as a closing session ends it.
func (h *Hub) PushAs(ctx context.Context, d store.HubDevice, p *api.LogPushParams) ([]api.PushAnswer, error) {
	return logService{h}.Push(context.WithValue(ctx, deviceKey{}, d), p)
}
