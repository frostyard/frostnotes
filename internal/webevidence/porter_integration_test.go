//go:build integration

package webevidence_test

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frostyard/frostnotes/api"
)

// Porter's scope (plan 0007): its own folder, except its routines, and
// today's daily note.
const porterScope = "vault=1;write=Porter/;except=Porter/Routines/;daily"

// What a refused write says, with the folders Porter may write.
const (
	outsideWrites  = "outside this token's writes"
	porterMayWrite = "this token may write under Porter/ except Porter/Routines/, and append to today's daily note"
)

// issue is lodgectl's token issue: the token alone on stdout, trimmed. The
// token is a secret: it is never logged.
func (w *world) issue(t *testing.T, name string, extra ...string) string {
	t.Helper()
	args := append([]string{"token", "issue", name, "--app", "notes", "--scope", porterScope}, extra...)
	cmd := exec.Command(w.lodgectl, args...)
	cmd.Env = append(os.Environ(), "LODGE_ROOT="+w.root)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// stdout may hold a token: only stderr is shown.
		t.Fatalf("lodgectl token issue %s: %v\n%s\nlodge:\n%s", name, err, stderr.String(), w.log)
	}
	token := strings.TrimSpace(stdout.String())
	if token == "" {
		t.Fatalf("lodgectl token issue %s printed no token", name)
	}
	return token
}

// bearer adds a token to every request.
type bearer struct {
	token string
	base  http.RoundTripper
}

func (b *bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// viaSocket is a transport that dials the unix socket at path, whatever the
// URL's host.
func viaSocket(path string) *http.Transport {
	return &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}}
}

// toolCall calls a tool and returns the result with its text.
func toolCall(t *testing.T, cs *sdk.ClientSession, tool string, args map[string]any) (*sdk.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return res, text.String()
}

// latest is the newest version of a note on the hub.
func (w *world) latest(t *testing.T, path string) api.Version {
	t.Helper()
	vs, err := w.hub.C.Note().Versions(t.Context(), &api.NoteVersionsParams{VaultID: w.hub.Vault, Path: path})
	if err != nil || len(vs) == 0 {
		t.Fatalf("the versions of %s = %v, %v", path, vs, err)
	}
	return vs[0]
}

// versionAuthors are the authors of every version of a note on the hub.
func (w *world) versionAuthors(t *testing.T, path string) []string {
	t.Helper()
	vs, err := w.hub.C.Note().Versions(t.Context(), &api.NoteVersionsParams{VaultID: w.hub.Vault, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, v := range vs {
		out = append(out, v.Author)
	}
	return out
}

// TestIPorterThroughLodge is plan 0007's Phase 4 on loopback: tokens of
// Porter's scope, issued by lodge's development build's lodgectl, and a
// real MCP client through lodge to notesd, once on the listener with a
// tailnet token and once on the agent socket with an agent's token. Each
// route writes Porter/Memory.md and Porter/Briefs/<date>.md, is refused at
// Porter/Routines/ in two spellings of its case and at a user's note,
// appends to today's daily note, and leaves versions that name its token.
func TestIPorterThroughLodge(t *testing.T) {
	w := startWorld(t)
	ctx := t.Context()
	day := time.Now().Format(time.DateOnly)

	// The vault's daily-note settings and template, and Porter's routine.
	// notesd reads the settings from the vault's files at each append.
	for p, c := range map[string]string{
		".obsidian/daily-notes.json": `{"folder": "Daily", "template": "Templates/Daily"}`,
		"Templates/Daily.md":         "# {{title}}\n",
	} {
		full := filepath.Join(w.hub.Dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const routine = "# Morning\n\nCheck the beds.\n"
	if _, err := w.hub.C.Note().Create(ctx, &api.NoteCreateParams{VaultID: w.hub.Vault, Path: "Porter/Routines/Morning.md", Content: routine}); err != nil {
		t.Fatal(err)
	}
	// A note whose folder is spelled in another case: on a case-sensitive
	// disk it is a folder of its own, one the case-folded check still holds.
	const spelled, spelledPath = "# Spelled\n\nA note in porter/routines.\n", "porter/routines/Spelled.md"
	if _, err := w.hub.C.Note().Create(ctx, &api.NoteCreateParams{VaultID: w.hub.Vault, Path: spelledPath, Content: spelled}); err != nil {
		t.Fatal(err)
	}
	routineBefore, gardenBefore, spelledBefore := w.disk("Porter/Routines/Morning.md"), w.disk("Garden.md"), w.disk(spelledPath)
	if routineBefore != routine || gardenBefore != garden || spelledBefore != spelled {
		t.Fatalf("the seeded notes on disk are %q, %q and %q", routineBefore, gardenBefore, spelledBefore)
	}

	// Lodge's agent socket admits a listed user: the one running the test.
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if me.Username == "root" || me.Username == "lodge" {
		t.Fatalf("lodge refuses %q as an agent: run the evidence as another user", me.Username)
	}
	cfgPath := filepath.Join(w.root, "etc/lodge/config.json")
	cfg := map[string]jsontext.Value{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("%s: %v", cfgPath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	agents, err := json.Marshal([]string{me.Username})
	if err != nil {
		t.Fatal(err)
	}
	cfg["agents"] = agents
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	w.ctl(t, "reload")
	sock := filepath.Join(w.root, "run/lodge/agent.sock")
	eventually(t, 30*time.Second, "lodge's agent socket", func() bool {
		_, err := os.Stat(sock)
		return err == nil
	})

	// Two tokens of Porter's scope: one for the listener (the tailnet's
	// rules), one for the agent socket and this user.
	const tailnetName, agentName = "porter-notes-tailnet", "porter-notes"
	tailnetToken := w.issue(t, tailnetName)
	agentToken := w.issue(t, agentName, "--agent", me.Username)
	t.Logf("lodgectl issued 2 tokens with Porter's scope %s: %s for the listener and %s for the agent socket (user %s)",
		porterScope, tailnetName, agentName, me.Username)

	routes := []struct {
		name, token, endpoint string
		client                *http.Client
		first                 bool // the route that creates, not appends
	}{
		{"listener", tailnetName, w.url + "/mcp/notes",
			&http.Client{Transport: &bearer{token: tailnetToken, base: http.DefaultTransport}}, true},
		{"agent-socket", agentName, "http://lodge/mcp/notes",
			&http.Client{Transport: &bearer{token: agentToken, base: viaSocket(sock)}}, false},
	}
	for _, r := range routes {
		t.Run(r.name, func(t *testing.T) {
			client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "evidence"}, nil)
			cs, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: r.endpoint, HTTPClient: r.client}, nil)
			if err != nil {
				t.Fatalf("%s: connecting through %s: %v\nlodge:\n%s", r.name, r.endpoint, err, w.log)
			}
			t.Cleanup(func() { _ = cs.Close() })
			t.Logf("%s: MCP session open on %s as %s", r.name, r.endpoint, r.token)

			// Porter's memory and today's brief: created, then appended to.
			memory, brief := "Porter/Memory.md", "Porter/Briefs/"+day+".md"
			for _, p := range []string{memory, brief} {
				var tool string
				var args map[string]any
				text := "- written through the " + r.name + "\n"
				if r.first {
					tool, args = "create_note", map[string]any{"path": p, "content": "# " + p + "\n\n" + text}
				} else {
					tool, args = "edit_note", map[string]any{"path": p, "append": text}
				}
				if res, out := toolCall(t, cs, tool, args); res.IsError {
					t.Fatalf("%s: %s %s: %s", r.name, tool, p, out)
				}
				if !strings.Contains(w.disk(p), text) {
					t.Errorf("%s: %s lacks the line written through the %s", r.name, p, r.name)
				}
				v := w.latest(t, p)
				if v.Source != api.VersionSourceAgent || v.Author != r.token {
					t.Errorf("%s: the latest version of %s is %s by %q; want agent by %q", r.name, p, v.Source, v.Author, r.token)
				}
				t.Logf("%s: %s %s: written, version by %s (%s)", r.name, tool, p, v.Author, v.Source)
			}

			// Refused at Porter's routines in two spellings of the case.
			refuse := func(tool string, args map[string]any) {
				t.Helper()
				p := args["path"].(string)
				res, text := toolCall(t, cs, tool, args)
				if !res.IsError || !strings.Contains(text, outsideWrites) || !strings.Contains(text, porterMayWrite) {
					t.Errorf("%s: %s %s = error %v, %q; want a refusal naming %q", r.name, tool, p, res.IsError, text, porterMayWrite)
					return
				}
				t.Logf("%s: %s %s: refused (%s; %s)", r.name, tool, p, outsideWrites, porterMayWrite)
			}
			// edit_note reads the note first, and on a case-sensitive disk
			// porter/routines/Morning.md is no note, so the edit in the second
			// spelling is of the seeded note in a folder of that spelling.
			refuse("edit_note", map[string]any{"path": "Porter/Routines/Morning.md", "append": "- changed\n"})
			refuse("edit_note", map[string]any{"path": spelledPath, "append": "- changed\n"})
			for _, p := range []string{"Porter/Routines/New.md", "porter/routines/New.md"} {
				refuse("create_note", map[string]any{"path": p, "content": "new\n"})
			}
			// And at a user's note.
			refuse("edit_note", map[string]any{"path": "Garden.md", "append": "- changed\n"})

			// Today's daily note, created from the template on the first append.
			line := "- captured through the " + r.name
			res, text := toolCall(t, cs, "append_to_daily_note", map[string]any{"text": line})
			if res.IsError {
				t.Fatalf("%s: append_to_daily_note: %s", r.name, text)
			}
			if want := `"created":` + map[bool]string{true: "true", false: "false"}[r.first]; !strings.Contains(text, want) {
				t.Errorf("%s: append_to_daily_note = %s; want %s", r.name, text, want)
			}
			daily := "Daily/" + day + ".md"
			v := w.latest(t, daily)
			if v.Source != api.VersionSourceAgent || v.Author != r.token {
				t.Errorf("%s: the latest version of %s is %s by %q; want agent by %q", r.name, daily, v.Source, v.Author, r.token)
			}
			t.Logf("%s: append_to_daily_note %s: appended, %s the note, version by %s (%s)", r.name, daily,
				map[bool]string{true: "created", false: "did not create"}[r.first], v.Author, v.Source)

			// The refusals changed nothing.
			if got := w.disk("Porter/Routines/Morning.md"); got != routineBefore {
				t.Errorf("%s: Porter/Routines/Morning.md on disk is %q; want it as before", r.name, got)
			}
			if got := w.disk(spelledPath); got != spelledBefore {
				t.Errorf("%s: %s on disk is %q; want it as before", r.name, spelledPath, got)
			}
			if got := w.disk("Garden.md"); got != gardenBefore {
				t.Errorf("%s: Garden.md on disk is %q; want it as before", r.name, got)
			}
			if err := filepath.WalkDir(w.hub.Dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(w.hub.Dir, p)
				rel = filepath.ToSlash(rel)
				if lower := strings.ToLower(rel); strings.HasPrefix(lower, "porter/routines/") && rel != "Porter/Routines/Morning.md" && rel != spelledPath && !d.IsDir() {
					t.Errorf("%s: %s is in the hub's vault; the routines folders must hold only Morning.md and Spelled.md", r.name, rel)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: Porter/Routines/Morning.md, %s and Garden.md are byte for byte as before; no New.md in Porter/Routines/ in any case", r.name, spelledPath)
		})
	}

	// A token works only on the route it was issued for.
	initialize := func(client *http.Client, url string) int {
		t.Helper()
		const body = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"evidence","version":"1"}}}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	if got := initialize(&http.Client{Transport: &bearer{token: agentToken, base: http.DefaultTransport}}, w.url+"/mcp/notes"); got != http.StatusUnauthorized {
		t.Errorf("an agent token on the listener's /mcp/notes answered %d; want 401", got)
	} else {
		t.Logf("cross check: the agent token %s on the listener's /mcp/notes: 401", agentName)
	}
	if got := initialize(&http.Client{Transport: &bearer{token: tailnetToken, base: viaSocket(sock)}}, "http://lodge/mcp/notes"); got != http.StatusUnauthorized {
		t.Errorf("a tailnet token on the agent socket answered %d; want 401", got)
	} else {
		t.Logf("cross check: the tailnet token %s on the agent socket's /mcp/notes: 401", tailnetName)
	}

	// Both routes' lines are in today's note, in order, and its versions name both tokens.
	daily := "Daily/" + day + ".md"
	want := "# " + day + "\n- captured through the listener\n- captured through the agent-socket\n"
	if got := w.disk(daily); got != want {
		t.Errorf("%s on disk is %q; want %q", daily, got, want)
	}
	authors := w.versionAuthors(t, daily)
	for _, name := range []string{tailnetName, agentName} {
		found := false
		for _, a := range authors {
			found = found || a == name
		}
		if !found {
			t.Errorf("the versions of %s are by %v; want one by %s", daily, authors, name)
		}
	}
	t.Logf("%s holds the template's heading and both appended lines in order; its versions are by %v", daily, authors)
}
