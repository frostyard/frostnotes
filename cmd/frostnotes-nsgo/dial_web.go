//go:build js && wasm

package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"syscall/js"
	"time"

	"github.com/coder/websocket"
	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/nsgo-dev/nsgo/core"
)

// Lodge admits the same-origin socket using the node and login from WhoIs.
// Local previews use notesd's existing gateway. The token travels in the
// fragment to keep it out of the static server's request logs.
func platformDialer() session.Dialer {
	location := js.Global().Get("location")
	scheme := "wss"
	if location.Get("protocol").String() == "http:" {
		scheme = "ws"
	}
	host := location.Get("host").String()
	endpoint := url.URL{Scheme: scheme, Host: host, Path: "/app/notes/api"}
	inLodge := strings.HasPrefix(location.Get("pathname").String(), "/app/notes/")
	local := !inLodge && (host == "127.0.0.1:5173" || host == "localhost:5173")
	if local {
		params, _ := url.ParseQuery(strings.TrimPrefix(location.Get("hash").String(), "#"))
		endpoint = url.URL{Scheme: "ws", Host: "127.0.0.1:46321", Path: "/", RawQuery: url.Values{"token": {params.Get("token")}}.Encode()}
	}
	return func(ctx context.Context) (*api.Client, *api.Hello, error) {
		if local && endpoint.Query().Get("token") == "" {
			return nil, nil, errors.New("Open the preview with the gateway token in #token=…")
		}
		ws, _, err := websocket.Dial(ctx, endpoint.String(), nil)
		if err != nil {
			return nil, nil, errors.New("Could not connect to the notesd WebSocket. Check the gateway token or Lodge session.")
		}
		// The session owns the connection after the dial deadline expires.
		nc := websocket.NetConn(context.Background(), ws, websocket.MessageText)
		ws.SetReadLimit(api.MaxMessageSize + 1)
		c := api.NewClient(nc)
		hello, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: session.ClientName})
		if err != nil {
			_ = c.Close()
			return nil, nil, err
		}
		go keepAlive(c)
		return c, hello, nil
	}
}

func keepAlive(c *api.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := c.RPC().Ping(ctx, &api.RPCPingParams{})
			cancel()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func guardBrowserDraft(cx *core.Scope, dirty func() bool) {
	handler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		args[0].Call("preventDefault")
		args[0].Set("returnValue", "")
		return nil
	})
	window := js.Global()
	cx.Effect(func() {
		if dirty() {
			window.Call("addEventListener", "beforeunload", handler)
		} else {
			window.Call("removeEventListener", "beforeunload", handler)
		}
	})
	cx.Cleanup(func() { window.Call("removeEventListener", "beforeunload", handler); handler.Release() })
}

// ownAppearance reports whether the client keeps its own choice of
// appearance. Lodge sets the web build's (app/src/nsgo-lodge.ts).
const ownAppearance = false

// Lodge supplies its own vault catalog and manages the server's filesystem.
func (w *workspace) settingsButton() []core.Node               { return nil }
func (w *workspace) settingsView(*core.Scope) core.Node        { return nil }
func (w *workspace) bindSettingsKey()                          {}
func (w *workspace) vaultWelcome() core.Node                   { return nil }
func (w *workspace) vaultDialog(*core.Scope, string) core.Node { return nil }
func isVaultAction(string) bool                                { return false }
func (w *workspace) loadSync()                                 {}
func (w *workspace) connectionLabel() string                   { return w.connection.Get() }
func desktopLifetime() func()                                  { return func() {} }
