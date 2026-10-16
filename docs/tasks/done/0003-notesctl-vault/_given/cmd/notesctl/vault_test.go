package main

// CONTRACT TEST for task card T-0003 (docs/tasks). Do not edit.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/rpctest"
)

func TestVaultCommands(t *testing.T) {
	srv := rpctest.Start(t)
	dir := t.TempDir()
	work, notes := filepath.Join(dir, "work"), filepath.Join(dir, "notes")
	for _, d := range []string{work, notes} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := runNotesctl(t, append([]string{"--socket", srv.Socket}, args...)...)
		if err != nil {
			t.Fatalf("notesctl %v: %v", args, err)
		}
		return out
	}

	if out := run("vault", "list"); out != "no vaults\n" {
		t.Fatalf("empty list = %q", out)
	}
	if out := run("vault", "add", work); out != fmt.Sprintf("added vault 1 \"work\" at %s\n", work) {
		t.Fatalf("add = %q", out)
	}
	t.Chdir(dir) // a relative PATH is made absolute by notesctl
	if out := run("vault", "add", "--name", "Personal Notes", "notes"); out != fmt.Sprintf("added vault 2 \"Personal Notes\" at %s\n", notes) {
		t.Fatalf("add --name with a relative path = %q", out)
	}
	want := fmt.Sprintf("2\tPersonal Notes\t%s\n1\twork\t%s\n", notes, work)
	if out := run("vault", "list"); out != want {
		t.Fatalf("list = %q, want %q", out, want)
	}
	if out := run("vault", "ls"); out != want {
		t.Fatalf("ls = %q, want %q", out, want)
	}

	var vaults []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(run("--json", "vault", "list")), &vaults); err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 2 || vaults[0].Name != "Personal Notes" || vaults[1].Path != work {
		t.Fatalf("list --json = %+v", vaults)
	}

	if out := run("vault", "rm", "1"); out != "removed vault 1\n" {
		t.Fatalf("rm = %q", out)
	}
	if out := run("vault", "list"); out != fmt.Sprintf("2\tPersonal Notes\t%s\n", notes) {
		t.Fatalf("list after rm = %q", out)
	}
}

func TestVaultCommandErrors(t *testing.T) {
	srv := rpctest.Start(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"vault", "add", "/no/such/dir"}, "invalidParams"},
		{[]string{"vault", "add"}, "accepts 1 arg"},
		{[]string{"vault", "rm", "x"}, "vault ID"},
		{[]string{"vault", "rm", "99"}, "does not exist"},
	} {
		_, err := runNotesctl(t, append([]string{"--socket", srv.Socket}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("notesctl %v: error %v, want one containing %q", tc.args, err, tc.want)
		}
	}
}
