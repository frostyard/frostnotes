//go:build !windows

package api

import (
	"context"
	"net"
)

// dialSocket connects to notesd's Unix socket.
func dialSocket(ctx context.Context, path string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", path)
}
