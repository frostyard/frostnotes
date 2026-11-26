package flatpakapp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/config"
)

// flatpak is an environment inside the app's Flatpak, in temp directories.
func flatpak(t *testing.T) (Getenv, string, string) {
	t.Helper()
	cfg, state := t.TempDir(), t.TempDir()
	env := map[string]string{"FLATPAK_ID": config.AppID, "XDG_CONFIG_HOME": cfg, "XDG_STATE_HOME": state, "HOME": t.TempDir(), "FROSTNOTES_LIBEXEC_DIR": t.TempDir()}
	return func(k string) string { return env[k] }, filepath.Join(cfg, dir), filepath.Join(state, dir)
}

func writeTo(t *testing.T, path, text string) {
	t.Helper()
	if err := WriteFile(path, []byte(text)); err != nil {
		t.Fatal(err)
	}
}

func TestDirs(t *testing.T) {
	env := func(k string) string {
		if k == "HOME" {
			return "/home/x"
		}
		if k == "XDG_STATE_HOME" {
			return "relative" // not absolute: ignored
		}
		return ""
	}
	c, _ := ConfigDir(env)
	s, _ := StateDir(env)
	if c != "/home/x/.config/frostnotes" || s != "/home/x/.local/state/frostnotes" {
		t.Errorf("%q %q", c, s)
	}
	if _, err := ConfigDir(func(string) string { return "" }); err == nil {
		t.Error("no HOME gave a directory")
	}
}

func TestNotesdLog(t *testing.T) {
	env, _, state := flatpak(t)
	path := filepath.Join(state, "notesd.log")
	log, ok := ReadNotesdLog(env)
	if !ok || log.LastLine != "" || log.PortalRefused {
		t.Fatalf("a missing log: %+v %v", log, ok)
	}
	writeTo(t, path, strings.Repeat("x\n", 10000)+"last line\n\n  \n")
	if log, _ := ReadNotesdLog(env); log.LastLine != "last line" || log.Path != path {
		t.Errorf("%+v", log)
	}
	writeTo(t, path, PortalFailure+"; see notesd.log\n")
	if log, _ := ReadNotesdLog(env); !log.PortalRefused {
		t.Errorf("%+v", log)
	}
	if _, ok := ReadNotesdLog(func(string) string { return "" }); ok {
		t.Error("read outside the Flatpak")
	}
}

func TestBackgroundSetting(t *testing.T) {
	env, cfg, state := flatpak(t)
	b, ok := GetBackground(env)
	if !ok || !b.KeepRunning || b.Portal != nil {
		t.Fatalf("%+v %v", b, ok)
	}
	b, err := SetBackground(env, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.KeepRunning || b.Generation != 0 { // no helper to start
		t.Errorf("%+v", b)
	}
	if _, err := os.Stat(filepath.Join(cfg, offFile)); err != nil {
		t.Error("no off file")
	}
	if generation(filepath.Join(state, genFile)) != 1 {
		t.Error("the counter did not move")
	}
	helper := filepath.Join(env("FROSTNOTES_LIBEXEC_DIR"), "frostnotes-request-background")
	writeTo(t, helper, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(helper, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err = SetBackground(env, true)
	if err != nil || !b.KeepRunning || b.Generation != 2 {
		t.Fatalf("%+v %v", b, err)
	}
	if _, err := SetBackground(func(string) string { return "" }, true); err == nil {
		t.Error("set outside the Flatpak")
	}
}

func TestPortalAnswers(t *testing.T) {
	env, _, state := flatpak(t)
	answer := func(gen int, errText string) string {
		return `{"at":"t","autostartRequested":true,"generation":` + strconv.Itoa(gen) + `,"error":"` + errText + `"}`
	}
	writeTo(t, filepath.Join(state, answerFile), answer(3, "answer"))
	if b, _ := GetBackground(env); b.Portal == nil || *b.Portal.Error != "answer" {
		t.Fatalf("%+v", b.Portal)
	}
	writeTo(t, filepath.Join(state, "background-failure-3.json"), answer(3, "tie"))
	if b, _ := GetBackground(env); *b.Portal.Error != "answer" {
		t.Error("a failure won a tie")
	}
	writeTo(t, filepath.Join(state, "background-failure-4.json"), answer(4, "newer"))
	writeTo(t, filepath.Join(state, "background-failure-x.json"), answer(9, "no number"))
	if b, _ := GetBackground(env); *b.Portal.Error != "newer" {
		t.Errorf("%v", *b.Portal.Error)
	}
}

func TestNotesdLogKeepsTheLastLines(t *testing.T) {
	env, _, state := flatpak(t)
	var b strings.Builder
	for i := range 15 {
		b.WriteString("line " + strconv.Itoa(i) + "\n\n")
	}
	writeTo(t, filepath.Join(state, "notesd.log"), b.String())
	log, _ := ReadNotesdLog(env)
	if len(log.Lines) != tailLines || log.Lines[0] != "line 5" || log.Lines[tailLines-1] != "line 14" || log.LastLine != "line 14" {
		t.Errorf("%q, last %q", log.Lines, log.LastLine)
	}
}

// Ask runs the helper and waits for it, so a process that exits after it,
// as notesctl does, does not end the portal's request.
func TestAskWaitsForTheHelpersAnswer(t *testing.T) {
	env, cfg, state := flatpak(t)
	helper := filepath.Join(env("FROSTNOTES_LIBEXEC_DIR"), "frostnotes-request-background")
	// The fake answers as the real one does: the counter it read, and
	// autostart only while the off file is missing.
	writeTo(t, helper, `#!/bin/sh
dir="$XDG_STATE_HOME/frostnotes"
gen=$(cat "$dir/background-gen" 2>/dev/null || echo 0)
auto=true
[ -e "$XDG_CONFIG_HOME/frostnotes/background-off" ] && auto=false
echo "asked with autostart $auto"
printf '{"at":"t","autostartRequested":%s,"response":0,"background":true,"autostart":%s,"error":null,"generation":%s}\n' "$auto" "$auto" "$gen" >"$dir/background.json"
`)
	if err := os.Chmod(helper, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", env("XDG_STATE_HOME"))
	t.Setenv("XDG_CONFIG_HOME", env("XDG_CONFIG_HOME"))
	gen, err := Change(env, false)
	if err != nil || gen != 1 {
		t.Fatalf("change: %d %v", gen, err)
	}
	if data, err := os.ReadFile(filepath.Join(cfg, offFile)); err != nil || !strings.Contains(string(data), "notesctl background on") {
		t.Errorf("the off file: %q %v", data, err)
	}
	if err := Ask(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	b, _ := GetBackground(env)
	if b.KeepRunning || b.Portal == nil || b.Portal.Generation == nil || *b.Portal.Generation != gen || *b.Portal.Autostart {
		t.Fatalf("%+v %+v", b, b.Portal)
	}
	if data, _ := os.ReadFile(filepath.Join(state, logFile)); !strings.Contains(string(data), "asked with autostart false") {
		t.Errorf("background.log: %q", data)
	}
}

func TestAskAndChangeBelongToTheFlatpak(t *testing.T) {
	none := func(string) string { return "" }
	if _, err := Change(none, false); err == nil {
		t.Error("changed outside the Flatpak")
	}
	if err := Ask(t.Context(), none); err == nil {
		t.Error("asked outside the Flatpak")
	}
	env, _, _ := flatpak(t) // no helper in its libexec directory
	if err := Ask(t.Context(), env); err == nil {
		t.Error("asked without a helper")
	}
}
