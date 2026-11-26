package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHelper puts a request helper in the test's libexec directory. It
// answers as frostnotes-request-background does: with the counter it read,
// autostart only while the off file is missing, and the response given.
func fakeHelper(t *testing.T, response string) {
	t.Helper()
	helper := filepath.Join(os.Getenv("FROSTNOTES_LIBEXEC_DIR"), "frostnotes-request-background")
	writeFile(t, helper, `#!/bin/sh
dir="$XDG_STATE_HOME/frostnotes"
mkdir -p "$dir"
gen=$(cat "$dir/background-gen" 2>/dev/null || echo 0)
auto=true
[ -e "$XDG_CONFIG_HOME/frostnotes/background-off" ] && auto=false
printf '{"at":"t","autostartRequested":%s,"response":`+response+`,"background":true,"autostart":%s,"error":null,"generation":%s}\n' "$auto" "$auto" "$gen" >"$dir/background.json"
`)
	if err := os.Chmod(helper, 0o755); err != nil {
		t.Fatal(err)
	}
}

// background off asks the portal, waits for its answer about the new
// setting and says that notesd runs on; background on asks again.
func TestBackgroundOffAndOn(t *testing.T) {
	cfg, _ := inFlatpak(t)
	fakeHelper(t, "0")
	out, err := runNotesctl(t, "background", "off")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"asking the desktop's Background portal…\n",
		"background: off, notesd does not start at login; the desktop allows it to run in the background, and does not start it at login\n",
		"the notesd that runs keeps running until you log out, or run: notesctl shutdown\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("background off lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(cfg, "background-off")); err != nil {
		t.Errorf("no off file: %v", err)
	}
	out, err = runNotesctl(t, "background")
	if err != nil || out != "background: off, notesd does not start at login; the desktop allows it to run in the background, and does not start it at login\n" {
		t.Errorf("background = %v:\n%s", err, out)
	}
	out, err = runNotesctl(t, "background", "on")
	if err != nil || !strings.Contains(out, "background: on, notesd starts at login when the desktop allows it; the desktop allows it to run in the background and starts it at login\n") || strings.Contains(out, "keeps running") {
		t.Errorf("background on = %v:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(cfg, "background-off")); !os.IsNotExist(err) {
		t.Errorf("the off file stayed: %v", err)
	}
}

// A refusal names the permission store's command: with no window, GNOME
// Settings does not list the app.
func TestBackgroundRefusalSaysHowToAllowIt(t *testing.T) {
	inFlatpak(t)
	fakeHelper(t, "1")
	out, err := runNotesctl(t, "background", "on")
	if err != nil || !strings.Contains(out, "the desktop refused") || !strings.Contains(out, "flatpak permission-set background background org.frostyard.Frostnotes yes") {
		t.Errorf("background on = %v:\n%s", err, out)
	}
}

// An answer about an older setting is not this change's.
func TestBackgroundWithAnAnswerOfAnOlderSetting(t *testing.T) {
	_, state := inFlatpak(t)
	helper := filepath.Join(os.Getenv("FROSTNOTES_LIBEXEC_DIR"), "frostnotes-request-background")
	writeFile(t, helper, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(helper, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(state, "background-gen"), "4\n")
	writeFile(t, filepath.Join(state, "background.json"), `{"at":"t","autostartRequested":true,"response":0,"background":true,"autostart":true,"generation":4}`)
	out, err := runNotesctl(t, "background", "off")
	if err != nil || !strings.Contains(out, "background: off, notesd does not start at login; the desktop's answer is not in yet") {
		t.Errorf("background off = %v:\n%s", err, out)
	}
}

func TestBackgroundBelongsToTheFlatpak(t *testing.T) {
	outsideFlatpak(t)
	for _, args := range [][]string{{"background"}, {"background", "off"}} {
		if _, err := runNotesctl(t, args...); err == nil || !strings.Contains(err.Error(), "Flatpak") {
			t.Errorf("%q outside the Flatpak = %v", args, err)
		}
	}
}

func TestBackgroundTakesOnlyOnOrOff(t *testing.T) {
	inFlatpak(t)
	for _, args := range [][]string{{"background", "maybe"}, {"background", "on", "off"}} {
		if _, err := runNotesctl(t, args...); err == nil {
			t.Errorf("%q succeeded", args)
		}
	}
}
