package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// runNotesctl runs notesctl with args through clix, as main does, and
// returns what it wrote to stdout.
func runNotesctl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	clix.Stdout = &out
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(&out)
	err := (&clix.App{Version: "test"}).Run(root)
	return out.String(), err
}

func TestHelloText(t *testing.T) {
	srv := rpctest.Start(t)
	out, err := runNotesctl(t, "--socket", srv.Socket, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "notesd rpctest (protocol 1)\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestHelloJSON(t *testing.T) {
	srv := rpctest.Start(t)
	out, err := runNotesctl(t, "--socket", srv.Socket, "--json", "hello")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Protocol int    `json:"protocol"`
		Server   string `json:"server"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output %q is not JSON: %v", out, err)
	}
	if got.Protocol != 1 || got.Server != "notesd rpctest" {
		t.Fatalf("JSON = %+v", got)
	}
}

func TestHelloWithoutNotesd(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "notesd.sock")
	_, err := runNotesctl(t, "--socket", missing, "hello")
	if err == nil || !strings.Contains(err.Error(), "connect to notesd") {
		t.Fatalf("error = %v, want one mentioning connect to notesd", err)
	}
}
