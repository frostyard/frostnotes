//go:build linux || darwin

package main

import (
	"context"
	"os"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/nsgo-dev/nsgo/core"
)

func platformDialer() session.Dialer {
	return func(ctx context.Context) (*api.Client, *api.Hello, error) {
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return nil, nil, err
		}
		return session.SocketDialer(paths.Socket)(ctx)
	}
}

// ownAppearance reports whether the client keeps its own choice of
// appearance. A desktop does.
const ownAppearance = true

func guardBrowserDraft(*core.Scope, func() bool) {}
