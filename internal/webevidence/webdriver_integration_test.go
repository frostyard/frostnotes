//go:build integration

package webevidence_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/evidence"
)

// A WebDriver client of the few calls the evidence needs, over geckodriver
// (https://www.w3.org/TR/webdriver2/), as spike 1 drove WebKitWebDriver.

// elementKey names a web element in WebDriver's JSON.
const elementKey = "element-6066-11e4-a52e-4f735466cecf"

// The WebDriver key codes the evidence types.
const (
	keyEnter   = "\ue007"
	keyEscape  = "\ue00c"
	keyEnd     = "\ue010"
	keyHome    = "\ue011"
	keyControl = "\ue009"
)

// client bounds every WebDriver call: a page that never answers fails the
// evidence instead of holding it.
var client = &http.Client{Timeout: 60 * time.Second}

// browser is one Firefox session.
type browser struct {
	t       testing.TB
	name    string
	base    string
	session string
	zoom    float64 // the full zoom that makes a phone's width
	log     *lockedBuffer
}

// minWidth is the narrowest window Firefox makes, in CSS pixels, at any
// device pixel ratio and in kiosk mode.
const minWidth = 500

// startBrowser starts geckodriver and a headless Firefox ESR whose page is
// width × height CSS pixels. Firefox makes no window narrower than
// minWidth, so a phone's width is a wider window at a full zoom, which
// reaches notes' frame as it does the shell; the zoom is a ratio Firefox
// keeps exactly (60 app units over a whole number of device pixels), and
// set again after each navigation, which resets it.
func startBrowser(t testing.TB, name string, width, height int) *browser {
	t.Helper()
	firefox := os.Getenv("FIREFOX")
	if firefox == "" {
		firefox = "/opt/firefox/firefox"
	}
	gecko, err := exec.LookPath("geckodriver")
	if err != nil {
		t.Skip("geckodriver is not installed; make m4-evidence runs in the nsl machine (dev/nsl/provision.sh)")
	}
	addr := evidence.FreeAddr(t)
	port := addr[strings.LastIndexByte(addr, ':')+1:]
	b := &browser{t: t, name: name, base: "http://" + addr, log: &lockedBuffer{}}
	// System access lets the harness set the browser's full zoom.
	cmd := exec.Command(gecko, "--host", "127.0.0.1", "--port", port, "--allow-system-access")
	cmd.Stdout, cmd.Stderr = b.log, b.log
	// Its own process group, with Firefox in it: the cleanup ends both.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	eventually(t, 30*time.Second, "geckodriver", func() bool {
		_, err := b.try(http.MethodGet, "/status", nil)
		return err == nil
	})
	v := b.call(http.MethodPost, "/session", map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{
		"browserName": "firefox",
		"moz:firefoxOptions": map[string]any{
			"binary": firefox,
			"args":   []string{"-headless"},
			"prefs": map[string]any{
				// A page's beforeunload would hold up navigating away.
				"dom.disable_beforeunload":  true,
				"browser.zoom.siteSpecific": false,
			},
		},
	}}})
	var s struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(v, &s); err != nil || s.SessionID == "" {
		t.Fatalf("new session: %s (%v)", v, err)
	}
	b.session = "/session/" + s.SessionID
	t.Cleanup(func() { _, _ = b.try(http.MethodDelete, b.session, nil) })

	b.zoom = 1
	if width < minWidth {
		b.zoom = 60 / math.Floor(60*float64(width)/minWidth)
	}
	// The window's rect is its outside: add what its chrome takes.
	winW := int(math.Ceil(float64(width) * b.zoom))
	b.call(http.MethodPost, b.session+"/window/rect", map[string]any{"width": winW, "height": height})
	var size [4]float64
	b.script(&size, "return [window.innerWidth, window.innerHeight, window.outerWidth, window.outerHeight];")
	chrome := size[3] - size[1]
	b.call(http.MethodPost, b.session+"/window/rect", map[string]any{"width": winW, "height": int(math.Ceil(float64(height)*b.zoom + chrome))})
	b.applyZoom()
	var page [2]float64
	b.script(&page, "return [window.innerWidth, window.innerHeight];")
	if math.Abs(page[0]-float64(width)) > 1 || math.Abs(page[1]-float64(height)) > 1 {
		t.Fatalf("the page is %v × %v, not %d × %d", page[0], page[1], width, height)
	}
	return b
}

// applyZoom sets the tab's full zoom, which a navigation resets.
func (b *browser) applyZoom() {
	b.t.Helper()
	if b.zoom == 1 {
		return
	}
	b.call(http.MethodPost, b.session+"/moz/context", map[string]any{"context": "chrome"})
	b.script(nil, `Services.wm.getMostRecentWindow("navigator:browser").gBrowser.selectedBrowser.browsingContext.fullZoom = arguments[0];`, b.zoom)
	b.call(http.MethodPost, b.session+"/moz/context", map[string]any{"context": "content"})
}

// try sends one WebDriver command and returns its value.
func (b *browser) try(method, path string, body any) (jsontext.Value, error) {
	var in io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		in = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(b.t.Context(), method, b.base+path, in)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Value jsontext.Value `json:"value"`
	}
	if err := json.UnmarshalRead(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(out.Value, &e)
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, e.Error, e.Message)
	}
	return out.Value, nil
}

// call is try, failing the test on an error.
func (b *browser) call(method, path string, body any) jsontext.Value {
	b.t.Helper()
	v, err := b.try(method, path, body)
	if err != nil {
		b.t.Fatalf("%v\ngeckodriver:\n%s", err, b.log)
	}
	return v
}

// navigate loads url in the top frame, at the browser's zoom.
func (b *browser) navigate(url string) {
	b.t.Helper()
	b.call(http.MethodPost, b.session+"/url", map[string]any{"url": url})
	b.applyZoom()
}

// find returns the first element css matches in the current frame; an
// XPath starts with "//".
func (b *browser) find(css string) (string, error) {
	using := "css selector"
	if strings.HasPrefix(css, "//") {
		using = "xpath"
	}
	v, err := b.try(http.MethodPost, b.session+"/element", map[string]any{"using": using, "value": css})
	if err != nil {
		return "", err
	}
	var el map[string]string
	if err := json.Unmarshal(v, &el); err != nil {
		return "", err
	}
	return el[elementKey], nil
}

// waitFind waits for css (or an XPath) to match, and returns the element.
func (b *browser) waitFind(css string, within time.Duration) string {
	b.t.Helper()
	var id string
	b.await(within, css, func() bool {
		var err error
		id, err = b.find(css)
		return err == nil && id != ""
	})
	return id
}

// click clicks an element.
func (b *browser) click(id string) {
	b.t.Helper()
	b.call(http.MethodPost, b.session+"/element/"+id+"/click", map[string]any{})
}

// typeInto sends text to an element as key presses.
func (b *browser) typeInto(id, text string) {
	b.t.Helper()
	b.call(http.MethodPost, b.session+"/element/"+id+"/value", map[string]any{"text": text})
}

// chord presses keys together and lets them go, as WebDriver actions:
// what reaches the focused element of the current frame.
func (b *browser) chord(keys ...string) {
	b.t.Helper()
	var acts []map[string]any
	for _, k := range keys {
		acts = append(acts, map[string]any{"type": "keyDown", "value": k})
	}
	for i := len(keys) - 1; i >= 0; i-- {
		acts = append(acts, map[string]any{"type": "keyUp", "value": keys[i]})
	}
	b.call(http.MethodPost, b.session+"/actions", map[string]any{"actions": []map[string]any{
		{"type": "key", "id": "keyboard", "actions": acts},
	}})
	b.call(http.MethodDelete, b.session+"/actions", nil)
}

// keys types text as key presses into the focused element of the current
// frame, as WebDriver actions; no modifier is held.
func (b *browser) keys(text string) {
	b.t.Helper()
	var acts []map[string]any
	for _, r := range text {
		acts = append(acts, map[string]any{"type": "keyDown", "value": string(r)}, map[string]any{"type": "keyUp", "value": string(r)})
	}
	b.call(http.MethodPost, b.session+"/actions", map[string]any{"actions": []map[string]any{
		{"type": "key", "id": "keyboard", "actions": acts},
	}})
	b.call(http.MethodDelete, b.session+"/actions", nil)
}

// frame switches into an iframe element; top switches back to the page.
func (b *browser) frame(id string) {
	b.t.Helper()
	b.call(http.MethodPost, b.session+"/frame", map[string]any{"id": map[string]string{elementKey: id}})
}

func (b *browser) top() {
	b.t.Helper()
	b.call(http.MethodPost, b.session+"/frame", map[string]any{"id": nil})
}

// script runs JavaScript in the current frame and decodes its result into out (unless nil).
func (b *browser) script(out any, js string, args ...any) {
	b.t.Helper()
	if args == nil {
		args = []any{}
	}
	v := b.call(http.MethodPost, b.session+"/execute/sync", map[string]any{"script": js, "args": args})
	if out != nil {
		if err := json.Unmarshal(v, out); err != nil {
			b.t.Fatalf("script result %s: %v", v, err)
		}
	}
}

// truthy runs js in the current frame and reports whether it returned true.
func (b *browser) truthy(js string, args ...any) bool {
	b.t.Helper()
	var ok bool
	b.script(&ok, js, args...)
	return ok
}

// eventually polls ok until it holds or within passes: the browser and
// the processes run on their own, and the evidence waits on what they show.
func eventually(t testing.TB, within time.Duration, what string, ok func() bool) {
	t.Helper()
	if !holds(within, ok) {
		t.Fatalf("gave up waiting for %s after %v", what, within)
	}
}

// holds polls ok for up to within.
func holds(within time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

// await is eventually for what the page shows: on giving up, it keeps a
// screenshot and says what the frame's editor and focus were.
func (b *browser) await(within time.Duration, what string, ok func() bool) {
	b.t.Helper()
	if holds(within, ok) {
		return
	}
	var state string
	if _, err := b.try(http.MethodPost, b.session+"/execute/sync", map[string]any{"script": "return 1;", "args": []any{}}); err == nil {
		b.script(&state, `const el = document.querySelector(".cm-content");
			const a = document.activeElement;
			return "editor: " + JSON.stringify(el ? el.innerText : null) + "; focus: " + (a ? a.tagName + "." + a.className : "none");`)
	}
	b.t.Fatalf("gave up waiting for %s after %v (%s; screenshot %s)", what, within, state, b.screenshot(what))
}

// screenshot keeps the page as it is in M4_EVIDENCE_DIR, or the test's temporary folder.
func (b *browser) screenshot(what string) string {
	dir := os.Getenv("M4_EVIDENCE_DIR")
	if dir == "" {
		dir = b.t.TempDir()
	}
	v, err := b.try(http.MethodGet, b.session+"/screenshot", nil)
	if err != nil {
		return "none: " + err.Error()
	}
	var data string
	if err := json.Unmarshal(v, &data); err != nil {
		return "none: " + err.Error()
	}
	png, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "none: " + err.Error()
	}
	name := strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\'' || r == '"' {
			return '-'
		}
		return r
	}, b.name+"-"+what) + ".png"
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "none: " + err.Error()
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		return "none: " + err.Error()
	}
	return path
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
