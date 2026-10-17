package main

import (
	"io"
	"slices"
	"testing"

	"github.com/frostyard/frostnotes/internal/rpctest"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpSession runs notesctl mcp with args over pipes, as an MCP host
// launches it, and connects a client to it.
func mcpSession(t *testing.T, srv *rpctest.Server, args ...string) (*sdk.ClientSession, <-chan error) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	root := newRootCmd()
	root.SetArgs(append([]string{"--socket", srv.Socket, "mcp"}, args...))
	root.SetIn(inR)
	root.SetOut(outW)
	done := make(chan error, 1)
	go func() {
		done <- root.ExecuteContext(t.Context())
		_ = outW.Close()
	}()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "host", Version: "1"}, nil).
		Connect(t.Context(), &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, done
}

func TestMCPServesTheVault(t *testing.T) {
	srv := rpctest.Start(t)
	if _, err := runNotesctl(t, "--socket", srv.Socket, "vault", "add", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args  []string
		tools int
	}{{nil, 7}, {[]string{"--read-only"}, 4}} {
		cs, done := mcpSession(t, srv, tc.args...)
		res, err := cs.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Tools) != tc.tools || !slices.ContainsFunc(res.Tools, func(tool *sdk.Tool) bool { return tool.Name == "read_note" }) {
			t.Errorf("notesctl mcp %v offers %d tools, want %d", tc.args, len(res.Tools), tc.tools)
		}
		if name := cs.InitializeResult().ServerInfo.Name; name != "frostnotes" {
			t.Errorf("server name = %q", name)
		}
		// The host closing stdin ends the server cleanly.
		_ = cs.Close()
		if err := <-done; err != nil {
			t.Errorf("notesctl mcp %v ended with %v", tc.args, err)
		}
	}
}

func TestMCPNeedsAVault(t *testing.T) {
	srv := rpctest.Start(t)
	_, err := runNotesctl(t, "--socket", srv.Socket, "mcp")
	if err == nil {
		t.Fatal("notesctl mcp without a vault succeeded")
	}
}
