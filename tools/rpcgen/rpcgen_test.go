package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSchema(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const minimalRPC = "domain: rpc\nprotocol: 1\ncallers: [local]\nmethods:\n  - name: hello\n"

func TestGoNames(t *testing.T) {
	for in, want := range map[string]string{
		"id": "ID", "vaultId": "VaultID", "displayName": "DisplayName",
		"mcp": "MCP", "rpc": "RPC", "sinceSeq": "SinceSeq", "ids": "IDs", "fileIds": "FileIDs",
	} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		want  string
	}{
		"unknown type": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\ntypes:\n  - name: T\n    fields:\n      - {name: x, type: Missing}\n"},
			"unknown type Missing",
		},
		"unknown error": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\nmethods:\n  - name: m\n    errors: [nope]\n"},
			`unknown error "nope"`,
		},
		"no callers": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\n"},
			`domain "a" must list its callers`,
		},
		"unknown caller": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local, anyone]\n"},
			`caller "anyone" must be one of`,
		},
		"repeated caller": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local, local]\n"},
			`caller "local" must be one of`,
		},
		"unknown yaml key": {
			map[string]string{"rpc.yaml": minimalRPC + "bogus: 1\n"},
			"field bogus not found",
		},
		"comma in unquoted flow doc": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\ntypes:\n  - name: T\n    fields:\n      - {name: x, type: int, doc: one, two}\n"},
			"not found",
		},
		"bad field case": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\ntypes:\n  - name: T\n    fields:\n      - {name: Bad, type: int}\n"},
			"must be unique lowerCamel",
		},
		"duplicate error code": {
			map[string]string{"rpc.yaml": minimalRPC, "errors.yaml": "errors:\n  - {name: a, code: 1}\n  - {name: b, code: 1}\n"},
			"reuses code 1",
		},
		"no protocol": {
			map[string]string{"a.yaml": "domain: a\ncallers: [local]\n"},
			"no file sets protocol",
		},
		"reserved client method": {
			map[string]string{"rpc.yaml": minimalRPC, "close.yaml": "domain: close\ncallers: [local]\n"},
			"collides with the hand-written Client.Close",
		},
		"hand-written api name": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\ntypes:\n  - name: Frame\n"},
			"collides with a hand-written identifier",
		},
		"generated name collision": {
			map[string]string{"rpc.yaml": minimalRPC, "a.yaml": "domain: a\ncallers: [local]\ntypes:\n  - name: AGetParams\nmethods:\n  - name: get\n"},
			"collides",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeSchema(t, tc.files))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestRepositorySchema generates from the real schema; make gen-check
// separately proves the committed outputs match.
func TestRepositorySchema(t *testing.T) {
	s, err := Load("../../schema/rpc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := genGo(s); err != nil {
		t.Fatal(err)
	}
	ts := string(genTS(s))
	for _, want := range []string{"export class Client", `"vault.list"`, "export const PROTOCOL = "} {

		if !strings.Contains(ts, want) {
			t.Errorf("TypeScript output lacks %q", want)
		}
	}
	if md := string(genMarkdown(s)); !strings.Contains(md, "### `rpc.hello`") {
		t.Error("Markdown output lacks rpc.hello")
	}
}

func TestCheckDetectsStaleOutput(t *testing.T) {
	dir := writeSchema(t, map[string]string{"rpc.yaml": minimalRPC})
	out := t.TempDir()
	args := []string{"-schema", dir, "-go", filepath.Join(out, "g.go"), "-ts", filepath.Join(out, "a.ts"), "-md", filepath.Join(out, "a.md")}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	if err := run(append([]string{"-check"}, args...)); err != nil {
		t.Fatalf("fresh output reported stale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(out, "a.ts"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(append([]string{"-check"}, args...)); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("edited output not reported stale: %v", err)
	}
}
