//go:build integration

package appevidence_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/evidence"
)

// TestIHotkeys is M3's exit evidence 2: every hotkey in the spec's
// keyboard map runs its command in the built app. The page lays out a
// workspace where the command applies and asks for the keys; xdotool
// presses them, so they reach the page through X, GTK and WebKit as a
// person's would.
func TestIHotkeys(t *testing.T) {
	a := startApp(t, "hotkeys")
	for {
		l := a.next(t, 2*time.Minute)
		if l.Spike != "" {
			t.Fatalf("the driver stopped: %s\n%s", l.raw, a.stderr)
		}
		if l.Say == "press" {
			a.focus(t)
			xdotool(t, "key", "--clearmodifiers", combo(l.Modifiers, l.Key))
		}
		if l.Say != "report" {
			continue
		}
		var said struct {
			Report jsontext.Value `json:"report"`
		}
		if err := json.Unmarshal(l.raw, &said); err != nil {
			t.Fatalf("the report: %v\n%s", err, l.raw)
		}
		var r struct {
			OK       bool     `json:"ok"`
			Error    string   `json:"error"`
			Bindings int      `json:"bindings"`
			Ran      int      `json:"ran"`
			NotYet   []string `json:"notYet"`
			Failed   []string `json:"failed"`
			Notices  []string `json:"notices"`
			Results  []struct {
				Command string   `json:"command"`
				Keys    string   `json:"keys"`
				Applied bool     `json:"applied"`
				Ran     bool     `json:"ran"`
				Runs    []string `json:"runs"`
				Seen    []string `json:"seen"`
			} `json:"results"`
		}
		if err := json.Unmarshal(said.Report, &r); err != nil {
			t.Fatalf("the report: %v\n%s", err, l.raw)
		}
		for _, x := range r.Results {
			t.Logf("%-26s %-38s applied %-5v ran %-5v %v; the page saw %q", x.Keys, x.Command, x.Applied, x.Ran, x.Runs, x.Seen)
		}
		t.Logf("%d of %d bindings ran their command; not in M3: %v", r.Ran, r.Bindings, r.NotYet)
		if !r.OK {
			t.Errorf("bindings that did not run their command: %v", r.Failed)
		}
		// Then the driver closes the window, and the app exits.
		select {
		case l, open := <-a.said:
			if open {
				t.Errorf("after Close window: %s", l.raw)
			}
		case <-time.After(20 * time.Second):
			t.Errorf("the app did not exit after Close window")
		}
		return
	}
}

// The note the writers share, as seeded: a line for each writer, apart, so
// no two writers change neighboring lines; MCP appends below the last.
const (
	note = "Writers.md"
	seed = "# Writers\n\ntab1:\n\ntab2:\n\nvim:\n\nhub:\n\nmcp:\n"
)

// TestIWriters is M3's exit evidence 3: each writer pair with the editor
// loses no token and keeps the cursor. Three notesd processes, a hub and
// devices a and b, share the note; the built app edits it on a in two
// tabs while vim saves it on a's disk, an agent appends to it through
// notesctl mcp, and b edits it, its changes reaching a through the hub.
func TestIWriters(t *testing.T) {
	appBinary(t)
	notesd := evidence.Build(t, "github.com/frostyard/frostnotes/cmd/notesd")
	notesctl := evidence.Build(t, "github.com/frostyard/frostnotes/cmd/notesctl")
	hubAddr := evidence.FreeAddr(t)
	hub := evidence.Start(t, notesd, "hub", "-hub", hubAddr)
	a := evidence.Start(t, notesd, "a")
	b := evidence.Start(t, notesd, "b")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	path := filepath.Join(a.Dir, note)
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.C.Hub().Serve(ctx, &api.HubServeParams{VaultID: hub.AddVault(ctx, t)}); err != nil {
		t.Fatal(err)
	}
	url := "http://" + hubAddr
	for _, d := range []*evidence.Notesd{a, b} {
		d.AddVault(ctx, t)
		d.Join(ctx, t, hub, url)
	}
	b.WaitFor(ctx, t, note)
	mcp := startMCP(t, notesctl, a.Socket)

	app := startApp(t, "writers", "FROSTNOTES_SOCKET="+a.Socket, "FROSTNOTES_DATA_DIR="+a.Data)
	o := &outside{tokens: map[string][]string{}}
	stop, stopWriters := context.WithCancel(ctx)
	defer stopWriters()
	var wg sync.WaitGroup
	started := false
	var l line
	for l = app.next(t, 2*time.Minute); l.Spike == ""; l = app.next(t, 2*time.Minute) {
		switch l.Say {
		case "type":
			if !started {
				started = true
				wg.Go(func() { vimSaves(stop, path, o) })
				wg.Go(func() { mcpAppends(stop, mcp, o) })
				wg.Go(func() { hubEdits(stop, ctx, b, o) })
			}
			app.focus(t)
			xdotool(t, "type", "--delay", "40", "--", l.Text)
		case "typed":
			stopWriters()
			wg.Wait()
			if hs := o.list("hub"); len(hs) > 0 {
				last := " " + hs[len(hs)-1]
				eventually(t, time.Minute, "b's last edit to reach a", func() bool { return strings.Contains(readFile(t, path), last) })
			}
			if err := os.WriteFile(filepath.Join(a.Dir, "Done.md"), []byte("done\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	var r struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Tokens int    `json:"tokens"`
		Moved  []struct {
			Tab    int    `json:"tab"`
			Before string `json:"before"`
			Head   int    `json:"head"`
			Want   int    `json:"want"`
		} `json:"moved"`
		Tabs []struct {
			Head int  `json:"head"`
			Want int  `json:"want"`
			Same bool `json:"same"`
		} `json:"tabs"`
		Notices []string `json:"notices"`
		Text    string   `json:"text"`
	}
	if err := json.Unmarshal(l.raw, &r); err != nil {
		t.Fatalf("the report: %v\n%s", err, l.raw)
	}
	if r.Error != "" {
		t.Fatalf("the driver stopped: %s\n%s", r.Error, app.stderr)
	}
	for _, n := range o.notes {
		t.Log(n)
	}
	if !r.OK {
		t.Errorf("the editor's cursors moved: %+v; at the end %+v", r.Moved, r.Tabs)
	}
	if len(r.Notices) > 0 {
		t.Errorf("the app showed notices: %q", r.Notices)
	}

	eventually(t, 15*time.Second, "a's file to hold what the editor shows", func() bool { return readFile(t, path) == r.Text })
	final := readFile(t, path)
	for label, tokens := range map[string][]string{
		"tab1:": typed('a', r.Tokens),
		"tab2:": typed('b', r.Tokens),
		"vim:":  o.list("vim"),
		"hub:":  o.list("hub"),
	} {
		want := strings.Join(append([]string{label}, tokens...), " ")
		if got := lineOf(final, label); got != want {
			t.Errorf("line %s = %q\nwant %q", label, got, want)
		}
	}
	if got, want := after(final, "mcp:"), o.list("mcp"); !slices.Equal(got, want) {
		t.Errorf("lines after mcp: = %q\nwant %q", got, want)
	}
	eventually(t, time.Minute, "b to converge", func() bool { return readFile(t, filepath.Join(b.Dir, note)) == final })
	limit := int64(1000)
	vs, err := a.C.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: a.Vault, Path: note, Limit: &limit})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := 0
	sources := map[api.VersionSource]int{}
	for _, v := range vs {
		sources[v.Source]++
		if v.Conflict {
			conflicts++
		}
	}
	if conflicts > 0 {
		t.Errorf("a kept %d conflict versions", conflicts)
	}
	t.Logf("typed %d tokens in each of two tabs, the cursors where they were before every token; meanwhile vim saved %d, MCP appended %d, b wrote %d through the hub; a and b hold all of them in order, with no conflict",
		r.Tokens, len(o.list("vim")), len(o.list("mcp")), len(o.list("hub")))
	t.Logf("a's versions of the note by source (the editor saves at most every 10 s while typing goes on, so the others landed on unsaved typing): %v", sources)
}

// --- the app

// line is one line the page printed: a request (say), or its report (spike).
type line struct {
	Say       string   `json:"say"`
	Modifiers []string `json:"modifiers"`
	Key       string   `json:"key"`
	Text      string   `json:"text"`
	Spike     string   `json:"spike"`
	raw       []byte
}

type app struct {
	cmd    *exec.Cmd
	said   chan line
	stderr *lockedBuffer
	window string
}

func appBinary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("FROSTNOTES_APP")
	if bin == "" {
		t.Skip("FROSTNOTES_APP names the built app; make m3-evidence sets it")
	}
	if os.Getenv("DISPLAY") == "" {
		t.Skip("the app needs an X display; make m3-evidence runs it under Xvfb")
	}
	return bin
}

// startApp runs the built app with a driving spike, its own XDG folders,
// and env.
func startApp(t *testing.T, spike string, env ...string) *app {
	t.Helper()
	bin := appBinary(t)
	home := t.TempDir()
	a := &app{said: make(chan line, 16), stderr: &lockedBuffer{}}
	a.cmd = exec.Command(bin)
	a.cmd.Env = append(os.Environ(), "FROSTNOTES_SPIKE="+spike,
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"))
	a.cmd.Env = append(a.cmd.Env, env...)
	a.cmd.Stderr = a.stderr
	out, err := a.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = a.cmd.Process.Kill()
		_ = a.cmd.Wait()
	})
	go func() {
		defer close(a.said)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			var l line
			if json.Unmarshal(sc.Bytes(), &l) != nil {
				continue // not the page's
			}
			l.raw = bytes.Clone(sc.Bytes())
			a.said <- l
		}
	}()
	return a
}

// next is the page's next line.
func (a *app) next(t *testing.T, within time.Duration) line {
	t.Helper()
	select {
	case l, ok := <-a.said:
		if !ok {
			t.Fatalf("the app exited without a report:\n%s", a.stderr)
		}
		return l
	case <-time.After(within):
		t.Fatalf("the app said nothing for %v:\n%s", within, a.stderr)
	}
	return line{}
}

// focus gives the app's window X's keyboard focus.
func (a *app) focus(t *testing.T) {
	t.Helper()
	if a.window == "" {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "xdotool", "search", "--sync", "--onlyvisible", "--pid", strconv.Itoa(a.cmd.Process.Pid)).Output()
		ids := strings.Fields(string(out))
		if err != nil || len(ids) == 0 {
			t.Fatalf("find the app's window: %v", err)
		}
		a.window = ids[0]
	}
	xdotool(t, "windowfocus", "--sync", a.window)
}

func xdotool(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "xdotool", args...).CombinedOutput(); err != nil {
		t.Fatalf("xdotool %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// combo is xdotool's name for a hotkey of the keyboard map.
func combo(modifiers []string, key string) string {
	var parts []string
	for _, m := range modifiers {
		parts = append(parts, strings.ToLower(m))
	}
	return strings.Join(append(parts, keysym(key)), "+")
}

// keysym is X's name for a key the keyboard map names as KeyboardEvent.key.
func keysym(key string) string {
	switch key {
	case "PageDown":
		return "Next"
	case "PageUp":
		return "Prior"
	case "ArrowLeft":
		return "Left"
	case "ArrowRight":
		return "Right"
	case "Enter":
		return "Return"
	case ",":
		return "comma"
	case ";":
		return "semicolon"
	case "/":
		return "slash"
	case "=":
		return "equal"
	case "-":
		return "minus"
	}
	if len(key) == 1 {
		return strings.ToLower(key)
	}
	return key // Tab, F1, F2
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// --- the outside writers

// outside records the tokens each outside writer wrote, in order.
type outside struct {
	mu     sync.Mutex
	tokens map[string][]string
	notes  []string
}

func (o *outside) add(who, token string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tokens[who] = append(o.tokens[who], token)
}

func (o *outside) note(format string, args ...any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.notes = append(o.notes, fmt.Sprintf(format, args...))
}

func (o *outside) list(who string) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.tokens[who])
}

// pause waits a quarter to three quarters of a second, or reports that
// the writers stop.
func pause(stop context.Context, rng *rand.Rand) bool {
	select {
	case <-stop.Done():
		return false
	case <-time.After(time.Duration(250+rng.IntN(500)) * time.Millisecond):
		return true
	}
}

// vimSaves appends tokens to the line "vim:" with vim, saving in place and
// by vim's rename by turns. Vim does not write over a file that changed
// since it read it (and still exits 0), so a token not in the file after
// vim exits is tried again.
func vimSaves(stop context.Context, path string, o *outside) {
	rng := rand.New(rand.NewPCG(1, 1))
	for i := 1; pause(stop, rng); {
		token := fmt.Sprintf("v%03d", i)
		backupcopy := []string{"no", "yes"}[i%2]
		out, err := exec.Command("vim", "-N", "-u", "NONE", "-i", "NONE", "-n", "-es",
			"-c", "set backupcopy="+backupcopy, "-c", `g/^vim:/s/$/ `+token+`/`, "-c", "wq", path).CombinedOutput()
		data, _ := os.ReadFile(path)
		if err != nil || !strings.HasSuffix(lineOf(string(data), "vim:"), " "+token) {
			o.note("vim did not save %s (backupcopy=%s): %v %s", token, backupcopy, err, out)
			continue
		}
		o.add("vim", token)
		i++
	}
}

// mcpAppends appends tokens as lines through notesctl mcp's edit_note.
func mcpAppends(stop context.Context, m *mcpClient, o *outside) {
	rng := rand.New(rand.NewPCG(2, 2))
	for i := 1; pause(stop, rng); i++ {
		token := fmt.Sprintf("m%03d", i)
		var res struct {
			Conflicts int `json:"conflicts"`
		}
		if err := m.call("edit_note", map[string]any{"path": note, "append": token}, &res); err != nil {
			o.note("mcp %s: %v", token, err)
			return
		}
		if res.Conflicts > 0 {
			o.note("mcp %s: %d conflicts", token, res.Conflicts)
		}
		o.add("mcp", token)
	}
}

// hubEdits appends tokens to the line "hub:" on device b, by note.write
// with the base it read, as b's editor would.
func hubEdits(stop, ctx context.Context, b *evidence.Notesd, o *outside) {
	rng := rand.New(rand.NewPCG(3, 3))
	for i := 1; pause(stop, rng); {
		token := fmt.Sprintf("h%03d", i)
		n, err := b.C.Note().Read(ctx, &api.NoteReadParams{VaultID: b.Vault, Path: note})
		if err != nil {
			o.note("b's read for %s: %v", token, err)
			continue
		}
		r, err := b.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: b.Vault, Path: note, Base: n.Hash, Content: appendToLine(n.Content, "hub:", " "+token)})
		if err != nil {
			o.note("b's write of %s: %v", token, err)
			continue
		}
		if r.Conflicts > 0 {
			o.note("b's write of %s: %d conflicts", token, r.Conflicts)
		}
		o.add("hub", token)
		i++
	}
}

// --- MCP over notesctl mcp's stdio

type mcpClient struct {
	in  io.WriteCloser
	out *bufio.Scanner
	id  int
}

func startMCP(t *testing.T, notesctl, socket string) *mcpClient {
	t.Helper()
	cmd := exec.Command(notesctl, "mcp")
	cmd.Env = append(os.Environ(), "FROSTNOTES_SOCKET="+socket)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = in.Close()
		_ = cmd.Wait()
	})
	m := &mcpClient{in: in, out: bufio.NewScanner(out)}
	m.out.Buffer(make([]byte, 64<<10), 16<<20)
	init := map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "evidence", "version": "1"}}
	if err := m.request("initialize", init, nil); err != nil {
		t.Fatalf("initialize notesctl mcp: %v", err)
	}
	if err := m.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *mcpClient) send(msg map[string]any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = m.in.Write(append(data, '\n'))
	return err
}

func (m *mcpClient) request(method string, params, result any) error {
	m.id++
	if err := m.send(map[string]any{"jsonrpc": "2.0", "id": m.id, "method": method, "params": params}); err != nil {
		return err
	}
	for m.out.Scan() {
		var r struct {
			ID     *int           `json:"id"`
			Result jsontext.Value `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(m.out.Bytes(), &r) != nil || r.ID == nil || *r.ID != m.id {
			continue // a notification
		}
		if r.Error != nil {
			return errors.New(r.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(r.Result, result)
	}
	return fmt.Errorf("notesctl mcp closed: %v", m.out.Err())
}

func (m *mcpClient) call(tool string, args map[string]any, out any) error {
	var res struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Structured jsontext.Value `json:"structuredContent"`
	}
	if err := m.request("tools/call", map[string]any{"name": tool, "arguments": args}, &res); err != nil {
		return err
	}
	if res.IsError {
		var text []string
		for _, c := range res.Content {
			text = append(text, c.Text)
		}
		return errors.New(strings.Join(text, " "))
	}
	return json.Unmarshal(res.Structured, out)
}

// --- the note's text

// lineOf is the line of text that starts with label.
func lineOf(text, label string) string {
	for l := range strings.Lines(text) {
		if strings.HasPrefix(l, label) {
			return strings.TrimSuffix(l, "\n")
		}
	}
	return ""
}

// appendToLine appends s to the line of text that starts with label.
func appendToLine(text, label, s string) string {
	var b strings.Builder
	for l := range strings.Lines(text) {
		if strings.HasPrefix(l, label) {
			body, nl := strings.CutSuffix(l, "\n")
			b.WriteString(body + s)
			if nl {
				b.WriteString("\n")
			}
			continue
		}
		b.WriteString(l)
	}
	return b.String()
}

// after is the non-empty lines after the line that starts with label.
func after(text, label string) []string {
	var out []string
	seen := false
	for l := range strings.Lines(text) {
		l = strings.TrimSuffix(l, "\n")
		switch {
		case !seen:
			seen = strings.HasPrefix(l, label)
		case l != "":
			out = append(out, l)
		}
	}
	return out
}

// typed is a tab's tokens: a001 to aNNN.
func typed(prefix byte, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%c%03d", prefix, i+1)
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

// eventually polls ok until it holds, or fails the test after within.
func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("gave up after %v waiting for %s", within, what)
		}
		<-tick.C
	}
}
