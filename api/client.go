package api

import (
	"context"
	"fmt"
	"net"

	"github.com/frostyard/lodge/kit/jsonrpc"
)

// The wire limits and types (jsonrpc).
const (
	MaxMessageSize = jsonrpc.MaxMessageSize
	EventBuffer    = jsonrpc.EventBuffer
	EventMethod    = jsonrpc.EventMethod
)

// ErrEventsOverflow means the caller stopped reading Client.Notifications.
var ErrEventsOverflow = jsonrpc.ErrEventsOverflow

// Frame is one JSON-RPC 2.0 message in either direction.
type Frame = jsonrpc.Frame[ErrorCode]

// Client is a notesd connection. It is safe for concurrent use; calls are
// multiplexed by request ID.
type Client struct {
	*jsonrpc.Client[ErrorCode]
}

// Dial connects to the notesd socket and completes rpc.hello.
func Dial(ctx context.Context, socketPath, clientName string) (*Client, *Hello, error) {
	conn, err := dialSocket(ctx, socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to notesd: %w", err)
	}
	c := NewClient(conn)
	h, err := c.RPC().Hello(ctx, &RPCHelloParams{Protocol: Protocol, Client: clientName})
	if err != nil {
		_ = c.Close()
		return nil, nil, err
	}
	return c, h, nil
}

// NewClient starts a client on an established connection. The caller must
// call rpc.hello before anything else; Dial does.
func NewClient(conn net.Conn) *Client { return &Client{jsonrpc.NewClient[ErrorCode](conn)} }
