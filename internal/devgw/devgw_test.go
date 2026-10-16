package devgw

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/websocket"
)

func echo(nc net.Conn) {
	defer nc.Close()
	r := bufio.NewReader(nc)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		_, _ = nc.Write([]byte("echo " + line))
	}
}

func dial(t *testing.T, url, origin string) (*websocket.Conn, error) {
	t.Helper()
	return websocket.Dial(url, "", origin)
}

func TestHandler(t *testing.T) {
	srv := httptest.NewServer(Handler("0123456789abcdef", Origins, echo))
	defer srv.Close()
	base := "ws" + strings.TrimPrefix(srv.URL, "http")

	ws, err := dial(t, base+"/?token=0123456789abcdef", "http://127.0.0.1:5173")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Write([]byte("{\"id\":1}\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := ws.Read(buf)
	if err != nil || string(buf[:n]) != "echo {\"id\":1}\n" {
		t.Fatalf("read %q, %v", buf[:n], err)
	}
	_ = ws.Close()

	for _, tc := range []struct{ query, origin string }{
		{"/?token=wrong", "http://127.0.0.1:5173"},
		{"/", "http://127.0.0.1:5173"},
		{"/?token=0123456789abcdef", "https://evil.example"},
		{"/?token=0123456789abcdef", "http://127.0.0.1:5174"},
	} {
		if ws, err := dial(t, base+tc.query, tc.origin); err == nil {
			_ = ws.Close()
			t.Errorf("connected with %s from %s", tc.query, tc.origin)
		}
	}
}

func TestListenOnlyOnLoopback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	log := slog.New(slog.DiscardHandler)
	if err := ListenAndServe(ctx, "0.0.0.0:0", "0123456789abcdef", echo, log); err == nil {
		t.Error("listened on all interfaces")
	}
	if err := ListenAndServe(ctx, "127.0.0.1:0", "short", echo, log); err == nil {
		t.Error("accepted a short token")
	}
}
