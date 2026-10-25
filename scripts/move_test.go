package scripts_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tests of move-hub-behind-lodge.sh (plan 0006, Phase 7) run it
// against a staged root: the trial hub's notesd.service as user m2hub, the
// overnight deployment's frostnotes.service with its test vault and
// drop-in, the vault on a CIFS mount in fstab, and, where a test stages
// it, lodge's former device with a synced copy of the hub's vault. Stub
// commands on PATH play systemd, runuser, getent, mount, lodgectl and
// notesctl, keep the units' state in the root, and log what the script
// runs.

const (
	trialHub = "0123456789abcdef0123456789abcdef"
	testHub  = "fedcba9876543210fedcba9876543210"
	stamp    = "20261026T090000Z"
	backup   = "var/backups/frostnotes-move-" + stamp
	fstab    = "# the file server\n//fileserver/share /srv/share cifs credentials=/etc/cifs-share,uid=m2hub,gid=m2hub,iocharset=utf8,x-systemd.automount 0 0\n"
	url      = "https://lodge.example.ts.net/device/notes"
)

const stubs = `#!/bin/bash
# One stub for every command, by the name it is called as.
R=$MOVE_ROOT
log() { printf '%s\n' "$*" >>"$R/stub/log"; }
u() { printf '%s' "$R/stub/units/$1"; }
case $(basename "$0") in
systemctl)
	case $1 in
	show) cat "$(u "$5")/$3" 2>/dev/null || { [ "$3" = LoadState ] && echo not-found; }; exit 0 ;;
	is-active) s=$(cat "$(u "$2")/active" 2>/dev/null || echo inactive); echo "$s"; [ "$s" = active ] ;;
	is-enabled) cat "$(u "$2")/enabled" 2>/dev/null || echo disabled ;;
	*) log "systemctl $*"
		cmd=$1; shift; now=; [ "${1:-}" = --now ] && { now=1; shift; }
		for unit in "$@"; do
			[ -d "$(u "$unit")" ] || continue
			case $cmd in
			enable) echo enabled >"$(u "$unit")/enabled"; if [ -n "$now" ]; then echo active >"$(u "$unit")/active"; fi ;;
			disable) echo disabled >"$(u "$unit")/enabled"; if [ -n "$now" ]; then echo inactive >"$(u "$unit")/active"; fi ;;
			start) echo active >"$(u "$unit")/active" ;;
			stop) echo inactive >"$(u "$unit")/active" ;;
			esac
		done ;;
	esac ;;
runuser)
	user=$2; shift 3; log "runuser $user $*"
	if [ -e "$R/stub/no-write" ] && [ "$1" = sh ]; then echo "Permission denied" >&2; exit 1; fi
	exec "$@" ;;
getent)
	case $2 in
	frostnotes) [ -e "$R/stub/no-frostnotes" ] && exit 2; echo "frostnotes:x:990:990::/var/lib/frostnotes:/usr/sbin/nologin" ;;
	m2hub) echo "m2hub:x:1001:1001::/home/m2hub:/bin/bash" ;;
	*) exit 2 ;;
	esac ;;
lodgectl) [ "$1" = status ] && { [ -e "$R/stub/lodge-down" ] && exit 1; exit 0; }; log "lodgectl $*" ;;
notesctl)
	if [ "$1" = help ]; then
		echo "Available Commands:"; echo "  status      Show the hub"
		[ -e "$R/stub/old-notesctl" ] || echo "  url         Record the URL devices should use for this hub"
		exit 0
	fi
	case ${FROSTNOTES_SOCKET:-} in
	/run/frostnotes-lodge/*) unit=frostnotes.service; data=$R/var/lib/frostnotes ;;
	*) unit=notesd.service; data=$R/home/m2hub/.local/share/frostnotes ;;
	esac
	[ "$(cat "$(u "$unit")/active" 2>/dev/null)" = active ] || { echo "cannot reach notesd" >&2; exit 1; }
	case "$1 $2" in
	"hub status") echo "hub $(cat "$data/hubid")"; if [ -e "$R/stub/listen" ]; then cat "$R/stub/listen"; else echo "listening on 127.0.0.1:7880"; fi ;;
	"hub vaults") cat "$data/hubvaultlist" ;;
	"vault list") cat "$data/vaultlist" ;;
	"hub url") log "notesctl hub url $3 (as $unit)" ;;
	"sync status") if [ -e "$data/syncstatus-$4-drain" ] && [ "$(cat "$(u lodge.service)/active" 2>/dev/null)" != active ]; then cat "$data/syncstatus-$4-drain"; elif [ -e "$data/syncstatus-$4" ]; then cat "$data/syncstatus-$4"; else echo "no synced vaults"; fi ;;
	"sync verify") if [ -e "$data/verify-fail-$4" ]; then echo "differs from the hub" >&2; exit 1; fi
		echo "ok: 687 files match the hub" ;;
	*) exit 2 ;;
	esac ;;
systemd-escape) echo srv-share.mount ;;
sleep) ;;
*) log "$(basename "$0") $*" ;;
esac
`

// stage builds a root and the stubs, and returns the root.
func stage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(p, content string) { writeFile(t, root, p, content) }
	unit := func(name string, props map[string]string) {
		for k, v := range props {
			write("stub/units/"+name+"/"+k, v+"\n")
		}
	}
	unit("lodge.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	unit("notesd.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled",
		"User": "m2hub", "Environment": "FROSTNOTES_SOCKET=/run/frostnotes/notesd.sock FROSTNOTES_SECRETS=file"})
	unit("frostnotes.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled",
		"Environment": "FROSTNOTES_DATA_DIR=/var/lib/frostnotes FROSTNOTES_SOCKET=/run/frostnotes-lodge/notesd.sock FROSTNOTES_SECRETS=file",
		"ExecStart":   "{ path=/opt/lodge/bin/notesd ; argv[]=/opt/lodge/bin/notesd -lodge ; ignore_errors=no }",
		"DropInPaths": "/etc/systemd/system/frostnotes.service.d/overnight.conf"})
	unit("frostnotes-lodge.socket", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	write("stub/log", "")
	write("etc/fstab", fstab)
	write("usr/share/lodge/apps/notes/app.json", `{"id": "notes"}`)
	write("usr/share/lodge/apps/notes/web/index.html", "<!doctype html>")
	// The trial hub's data, and the overnight deployment's test.
	write("home/m2hub/.local/share/frostnotes/notesd.db", "the hub's database")
	write("home/m2hub/.local/share/frostnotes/blobs/ab/abcdef", "a version")
	write("home/m2hub/.local/share/frostnotes/hubid", trialHub)
	write("home/m2hub/.local/share/frostnotes/vaultlist", "1\tMy Notes\t/srv/share/Notes/My Notes\n")
	write("home/m2hub/.local/share/frostnotes/hubvaultlist", "My Notes\tmy-notes\t8702\n")
	write("var/lib/frostnotes/notesd.db", "the overnight test's database")
	write("var/lib/frostnotes/hubid", testHub)
	write("var/lib/frostnotes/vaultlist", "1\tTest\t/var/lib/frostnotes-test/Test\n")
	write("var/lib/frostnotes/hubvaultlist", "Test\ttest\t12\n")
	// The vault, on the share.
	write("srv/share/Notes/My Notes/Plan.md", "# Plan\n")
	// The stubs: one script, many names; notesctl beside notesd.
	bin := filepath.Join(root, "stub", "bin")
	write("stub/bin/stub", stubs)
	if err := os.Chmod(filepath.Join(bin, "stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"systemctl", "runuser", "getent", "lodgectl", "notesctl", "mount", "umount", "chown", "systemd-escape", "sleep"} {
		if err := os.Symlink("stub", filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "opt", "lodge", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(bin, "notesctl"), filepath.Join(root, "opt", "lodge", "bin", "notesctl")); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFile(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, p string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// copyInStep is `notesctl sync status --vault 2` for lodge's copy of the
// hub's My Notes while it is level with the hub, at the hub's seq 8702.
const copyInStep = "2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8702/8702\t0 pending\t0 conflicted\n"

// stageHubCopy stages lodge's former device: its notesd joined the trial
// hub and keeps a synced copy of My Notes beside the test.
func stageHubCopy(t *testing.T, root string) {
	t.Helper()
	stageHubCopies(t, root, copyInStep)
}

// stageHubCopies stages the copy with status as `notesctl sync status
// --vault 2` prints it, and the copy's contents: the hub's note, and the
// device's own .frostnotes at the copy's root.
func stageHubCopies(t *testing.T, root, status string) {
	t.Helper()
	writeFile(t, root, "var/lib/frostnotes/vaultlist",
		"2\tMy Notes\t/var/lib/frostnotes/vaults/My Notes\n1\tTest\t/var/lib/frostnotes-test/Test\n")
	writeFile(t, root, "var/lib/frostnotes/syncstatus-2", status)
	writeFile(t, root, "var/lib/frostnotes/vaults/My Notes/Plan.md", "# Plan\n")
	writeFile(t, root, "var/lib/frostnotes/vaults/My Notes/.frostnotes/x", "the device's own")
}

// stageLodgectlBesideLodge takes lodgectl off the stubs' PATH and puts it in
// /opt/lodge/bin, beside the binary lodge.service names, as it is on the
// hub's machine.
func stageLodgectlBesideLodge(t *testing.T, root string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, "stub", "bin", "lodgectl")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "stub/units/lodge.service/ExecStart", "{ path=/opt/lodge/bin/lodge ; argv[]=/opt/lodge/bin/lodge ; ignore_errors=no }\n")
	if err := os.Symlink(filepath.Join(root, "stub", "bin", "stub"), filepath.Join(root, "opt", "lodge", "bin", "lodgectl")); err != nil {
		t.Fatal(err)
	}
}

// move runs the script against root and returns its output and exit code.
func move(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	return moveWithPath(t, root, os.Getenv("PATH"), args...)
}

// moveWithPath is move with path in place of the host's PATH after the
// stubs, so that a command the host has cannot stand in for one the test
// stages.
func moveWithPath(t *testing.T, root, path string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"move-hub-behind-lodge.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "MOVE_ROOT="+root, "MOVE_STAMP="+stamp,
		"PATH="+filepath.Join(root, "stub", "bin")+":"+path)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return string(out), ee.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return string(out), 0
}

// snapshot is every file and directory under root but the stubs' and the
// backups', by path: a file's content's hash.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() && (rel == "stub" || rel == filepath.FromSlash("var/backups")) {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// same reports how two snapshots differ, or "".
func same(before, after map[string]string) string {
	var diff []string
	for _, k := range slices.Sorted(maps.Keys(before)) {
		if v, ok := after[k]; !ok {
			diff = append(diff, "-"+k)
		} else if v != before[k] {
			diff = append(diff, "~"+k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[k]; !ok {
			diff = append(diff, "+"+k)
		}
	}
	return strings.Join(diff, " ")
}

// changes are the logged commands that change the machine.
func changes(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(readFile(t, root, "stub/log")), "\n") {
		if l != "" && !strings.HasPrefix(l, "runuser ") {
			out = append(out, l)
		}
	}
	return out
}

// inOrder fails unless every step is in got, in order.
func inOrder(t *testing.T, got []string, steps ...string) {
	t.Helper()
	last := -1
	for _, step := range steps {
		i := slices.Index(got, step)
		if i <= last {
			t.Fatalf("%q is missing or out of order in %q", step, got)
		}
		last = i
	}
}

func TestTheDryRunChecksAndChangesNothing(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	out, code := move(t, root)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	for _, want := range []string{
		"✓ lodge is installed and up",
		"✓ user frostnotes exists (uid 990, gid 990)",
		"frostnotes.service runs /opt/lodge/bin/notesd as frostnotes, data /var/lib/frostnotes, private socket /run/frostnotes-lodge/notesd.sock",
		"with drop-ins: /etc/systemd/system/frostnotes.service.d/overnight.conf",
		"✓ the hub's data is /home/m2hub/.local/share/frostnotes",
		"✓ the trial notesd is hub " + trialHub,
		"vault My Notes at /srv/share/Notes/My Notes",
		"holds the overnight deployment's test (vault Test): it goes aside to /var/lib/frostnotes.overnight-" + stamp + ", never merged",
		"vault mount /srv/share: credentials=/etc/cifs-share,uid=m2hub,gid=m2hub",
		"$ systemctl disable --now notesd.service",
		"uid= and gid= become 990 and 990",
		"notesctl hub url https://LODGE/device/notes",
		"Roll back with:",
		"Dry run: nothing changed.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the dry run's output lacks %q:\n%s", want, out)
		}
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the dry run changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 {
		t.Errorf("the dry run ran %q", c)
	}
}

func TestFindsTheTrialsSocketByNotesdsDefault(t *testing.T) {
	root := stage(t)
	writeFile(t, root, "stub/units/notesd.service/Environment", "XDG_RUNTIME_DIR=/run/user/1001 FROSTNOTES_SECRETS=file\n")
	if out, code := move(t, root); code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	if log := readFile(t, root, "stub/log"); !strings.Contains(log, "runuser m2hub env FROSTNOTES_SOCKET=/run/user/1001/frostnotes/notesd.sock") {
		t.Errorf("the trial notesd was not asked at its default socket:\n%s", log)
	}
}

func TestRefusesWhatItCannotReadUnambiguously(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, root string)
		args  []string
		want  string
	}{
		"other vaults where the test should be": {func(t *testing.T, root string) {
			writeFile(t, root, "var/lib/frostnotes/vaultlist", "1\tTest\t/x\n2\tWork\t/y\n")
		}, nil, "holds vaults other than the overnight test (Test Work )"},
		"a copy of the hub's vault that is behind": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8700/8702\t0 pending\t0 conflicted\n")
		}, nil, "is not in step with the hub (8700/8702 cursor/hubSeq)"},
		"a copy with a file the hub has not seen": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8702/8702\t1 pending\t0 conflicted\n")
		}, nil, "is not in step with the hub (1 pending)"},
		"a copy in conflict": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8702/8702\t0 pending\t1 conflicted\n")
		}, nil, "is not in step with the hub (1 conflicted)"},
		"a copy with a refused file": {func(t *testing.T, root string) {
			stageHubCopies(t, root, copyInStep+"\trefused Plan.md: the hub's is newer\n")
		}, nil, "is not in step with the hub (refused/skipped/error line)"},
		"a copy that does not verify": {func(t *testing.T, root string) {
			stageHubCopy(t, root)
			writeFile(t, root, "var/lib/frostnotes/verify-fail-2", "")
		}, nil, "vault My Notes in /var/lib/frostnotes does not verify against the hub"},
		"a vault that is not synced": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "no synced vaults\n")
		}, nil, "vault My Notes in /var/lib/frostnotes is not a copy of the hub's vaults: move /var/lib/frostnotes aside yourself"},
		"a copy that syncs with another hub": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tMy Notes\thttps://hub.example.ts.net\tidle\t8702/8702\t0 pending\t0 conflicted\n")
		}, nil, "syncs with https://hub.example.ts.net, not with this machine's trial hub (port 7880)"},
		"a copy that syncs on another port": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tMy Notes\thttp://127.0.0.1:7999\tidle\t8702/8702\t0 pending\t0 conflicted\n")
		}, nil, "syncs with http://127.0.0.1:7999, not with this machine's trial hub (port 7880)"},
		"a copy of a vault the hub does not serve": {func(t *testing.T, root string) {
			stageHubCopies(t, root, "2\tOther\thttp://127.0.0.1:7880\tidle\t8702/8702\t0 pending\t0 conflicted\n")
		}, nil, "vault My Notes in /var/lib/frostnotes is not a copy of the hub's vaults"},
		"the move already done": {func(t *testing.T, root string) {
			writeFile(t, root, "var/lib/frostnotes/hubid", trialHub)
		}, nil, "already serves hub " + trialHub},
		"data no notesd describes": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostnotes.service/active", "inactive\n")
		}, nil, "holds data and frostnotes.service is not running"},
		"a vault on no cifs mount": {func(t *testing.T, root string) {
			writeFile(t, root, "etc/fstab", "/dev/vda1 / ext4 defaults 0 1\n")
		}, nil, "vault /srv/share/Notes/My Notes is under no cifs mount"},
		"no frostnotes user": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/no-frostnotes", "")
		}, nil, "user frostnotes does not exist"},
		"the trial hub stopped": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/notesd.service/active", "inactive\n")
		}, nil, "notesd.service is inactive: start it"},
		"no trial hub": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/notesd.service/LoadState", "not-found\n")
		}, nil, "notesd.service, the trial's hub, is not installed"},
		"lodge down": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/lodge-down", "")
		}, nil, "lodge is not up"},
		"a moved hub's leftovers": {func(t *testing.T, root string) {
			writeFile(t, root, "home/m2hub/.local/share/frostnotes.moved-20261025T000000Z/notesd.db", "")
		}, nil, "looks moved already"},
		"an M2 notesctl": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/old-notesctl", "")
		}, nil, "has no 'hub url'"},
		"--apply without --url":   {func(*testing.T, string) {}, []string{"--apply"}, "--apply needs --url"},
		"a URL that is not https": {func(*testing.T, string) {}, []string{"--url", "http://lodge/device/notes"}, "--url must be https://"},
	} {
		t.Run(name, func(t *testing.T) {
			root := stage(t)
			tc.setup(t, root)
			before := snapshot(t, root)
			out, code := move(t, root, tc.args...)
			if code == 0 || !strings.Contains(out, tc.want) || !strings.Contains(out, "nothing changed") {
				t.Fatalf("exit %d, output lacks %q or \"nothing changed\":\n%s", code, tc.want, out)
			}
			if d := same(before, snapshot(t, root)); d != "" {
				t.Errorf("a refusal changed %s", d)
			}
			if c := changes(t, root); len(c) != 0 {
				t.Errorf("a refusal ran %q", c)
			}
		})
	}
}

func TestApplyMovesTheHubAndRollbackPutsItBack(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	out, code := move(t, root, "--apply", "--url", url)
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	// The hub's data serves from frostnotes' data directory; the test's is
	// aside, never merged; the original is renamed, kept.
	if got := readFile(t, root, "var/lib/frostnotes/hubid"); got != trialHub {
		t.Errorf("/var/lib/frostnotes is hub %s, want the trial's", got)
	}
	if got := readFile(t, root, "var/lib/frostnotes/notesd.db"); got != "the hub's database" {
		t.Errorf("/var/lib/frostnotes/notesd.db = %q", got)
	}
	if got := readFile(t, root, "var/lib/frostnotes.overnight-"+stamp+"/notesd.db"); got != "the overnight test's database" {
		t.Errorf("the test's data aside = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "home/m2hub/.local/share/frostnotes")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the trial's data directory is still in place: %v", err)
	}
	if got := readFile(t, root, "home/m2hub/.local/share/frostnotes.moved-"+stamp+"/blobs/ab/abcdef"); got != "a version" {
		t.Errorf("the trial's data, kept = %q", got)
	}
	// The mount is frostnotes', its other options kept; fstab's original is backed up.
	if got, want := readFile(t, root, "etc/fstab"), strings.Replace(fstab, "uid=m2hub,gid=m2hub", "uid=990,gid=990", 1); got != want {
		t.Errorf("fstab = %q, want %q", got, want)
	}
	if got := readFile(t, root, backup+"/fstab"); got != fstab {
		t.Errorf("the fstab backup = %q", got)
	}
	// The vault's contents are as they were, and the write check left nothing.
	if got := readFile(t, root, "srv/share/Notes/My Notes/Plan.md"); got != "# Plan\n" {
		t.Errorf("the vault's note is %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(root, "srv/share/.frostnotes-write-check-*")); len(left) != 0 {
		t.Errorf("the write check left %v", left)
	}
	// In order: notes in lodge stops, the trial stops, the mount, notes starts, devices move, lodge reloads.
	c := changes(t, root)
	inOrder(t, c,
		"systemctl stop frostnotes-lodge.socket frostnotes.service",
		"systemctl disable --now notesd.service",
		"chown -R frostnotes:frostnotes "+filepath.Join(root, "var/lib/frostnotes.new"),
		"systemctl daemon-reload",
		"systemctl stop srv-share.automount srv-share.mount",
		"systemctl start srv-share.automount",
		"systemctl enable --now frostnotes-lodge.socket frostnotes.service",
		"notesctl hub url "+url+" (as frostnotes.service)",
		"lodgectl reload",
	)
	for _, want := range []string{"✓ frostnotes wrote to /srv/share", "✓ frostnotes.service serves hub " + trialHub, "✓ its vaults are the hub's: /srv/share/Notes/My Notes"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}

	// The rollback: the trial hub and the overnight test come back as they
	// were, fstab too; the moved copy stays in the backup.
	out, code = move(t, root, "--rollback", filepath.Join(root, backup), "--apply")
	if code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if !strings.Contains(out, "✓ the hub's log is where the move left it (My Notes:8702 )") {
		t.Errorf("the rollback did not check the hub's log:\n%s", out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
	if got := readFile(t, root, backup+"/data-after-move/hubid"); got != trialHub {
		t.Errorf("the moved copy, kept in the backup = %q", got)
	}
	inOrder(t, changes(t, root)[len(c):],
		"systemctl disable --now frostnotes.service frostnotes-lodge.socket",
		"systemctl daemon-reload",
		"systemctl enable --now notesd.service",
		"systemctl start frostnotes-lodge.socket frostnotes.service",
		"systemctl enable frostnotes-lodge.socket frostnotes.service",
		"lodgectl reload",
	)
	for unit, want := range map[string]string{"notesd.service": "active\n", "frostnotes.service": "active\n", "frostnotes-lodge.socket": "active\n"} {
		if got := readFile(t, root, "stub/units/"+unit+"/active"); got != want {
			t.Errorf("%s after the rollback is %q", unit, got)
		}
	}
	if out, code := move(t, root, "--rollback", filepath.Join(root, backup), "--apply"); code == 0 || !strings.Contains(out, "rolled back already") {
		t.Errorf("a second rollback: exit %d:\n%s", code, out)
	}
}

func TestAFailedStepPrintsTheRollbackThatUndoesIt(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	writeFile(t, root, "stub/no-write", "")
	out, code := move(t, root, "--apply", "--url", url)
	if code == 0 || !strings.Contains(out, "frostnotes cannot write to /srv/share") ||
		!strings.Contains(out, "roll back with: move-hub-behind-lodge.sh --rollback "+filepath.Join(root, backup)+" --apply") {
		t.Fatalf("exit %d, without the failure or the rollback:\n%s", code, out)
	}
	if strings.Contains(out, "notesctl hub url "+url) || slices.Contains(changes(t, root), "systemctl enable --now frostnotes-lodge.socket frostnotes.service") {
		t.Fatalf("the move went on past the failed check:\n%s", out)
	}
	if out, code := move(t, root, "--rollback", filepath.Join(root, backup), "--apply"); code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
}

func TestRollbackRefusesOnceDevicesSyncedThroughLodge(t *testing.T) {
	root := stage(t)
	if out, code := move(t, root, "--apply", "--url", url); code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	// A device pushed through lodge: the moved hub's log is ahead.
	writeFile(t, root, "var/lib/frostnotes/hubvaultlist", "My Notes\tmy-notes\t8710\n")
	moved := snapshot(t, root)
	n := len(changes(t, root))
	out, code := move(t, root, "--rollback", filepath.Join(root, backup), "--apply")
	if code == 0 || !strings.Contains(out, "the hub's log moved since the move (then My Notes:8702 , now My Notes:8710 )") {
		t.Fatalf("exit %d, without the refusal:\n%s", code, out)
	}
	if d := same(moved, snapshot(t, root)); d != "" || len(changes(t, root)) != n {
		t.Fatalf("the refused rollback changed %s, ran %q", d, changes(t, root)[n:])
	}
	out, code = move(t, root, "--rollback", filepath.Join(root, backup), "--apply", "--force")
	if code != 0 || !strings.Contains(out, "rolling back anyway (--force)") {
		t.Fatalf("--force: exit %d:\n%s", code, out)
	}
	if got := readFile(t, root, "home/m2hub/.local/share/frostnotes/hubid"); got != trialHub {
		t.Errorf("the trial's data after the rollback = %q", got)
	}
	if !strings.Contains(out, "Devices that followed the hub to "+url+" stay offline") {
		t.Errorf("the rollback does not say what becomes of moved devices:\n%s", out)
	}
}

func TestALodgedCopyOfTheHubsVaultGoesAsideWithTheTest(t *testing.T) {
	root := stage(t)
	stageHubCopy(t, root)
	before := snapshot(t, root)
	out, code := move(t, root)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	for _, want := range []string{
		"✓ vault My Notes in /var/lib/frostnotes is a copy of the hub's vault My Notes (http://127.0.0.1:7880), pushed in full and verified at seq 8702",
		"✓ /var/lib/frostnotes holds the overnight deployment's test and its copies of the hub's vaults: it goes aside to /var/lib/frostnotes.overnight-" + stamp + ", never merged",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the dry run's output lacks %q:\n%s", want, out)
		}
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the dry run changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 {
		t.Errorf("the dry run ran %q", c)
	}

	// --apply sets the whole directory aside, the copy with the test, and
	// records the copies, so the operator revokes lodge's former device.
	out, code = move(t, root, "--apply", "--url", url)
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	if got := readFile(t, root, "var/lib/frostnotes/hubid"); got != trialHub {
		t.Errorf("/var/lib/frostnotes is hub %s, want the trial's", got)
	}
	aside := "var/lib/frostnotes.overnight-" + stamp
	if got := readFile(t, root, aside+"/notesd.db"); got != "the overnight test's database" {
		t.Errorf("the test's data aside = %q", got)
	}
	if got := readFile(t, root, aside+"/syncstatus-2"); got != copyInStep {
		t.Errorf("the copy's status aside = %q", got)
	}
	if got := readFile(t, root, backup+"/manifest"); !strings.Contains(got, "aside_copies=1") ||
		!strings.Contains(got, "lodge_stopped=1") {
		t.Errorf("the manifest does not record the copies and the stop of lodge:\n%s", got)
	}
	if !strings.Contains(out, "notesctl hub revoke ID") {
		t.Errorf("the move does not say to revoke lodge's former device:\n%s", out)
	}
	if !strings.Contains(out, "✓ lodge's copy of My Notes is in the hub at seq 8702") {
		t.Errorf("the move does not say lodge's copy drained:\n%s", out)
	}
	// Lodge goes down before its notesd's copies drain, and up again after
	// the URL is recorded.
	inOrder(t, changes(t, root),
		"systemctl stop lodge.service",
		"systemctl restart frostnotes.service",
		"systemctl stop frostnotes-lodge.socket frostnotes.service",
		"systemctl disable --now notesd.service",
		"notesctl hub url "+url+" (as frostnotes.service)",
		"systemctl start lodge.service",
	)

	// The rollback puts the test and its copies back where they were.
	out, code = move(t, root, "--rollback", filepath.Join(root, backup), "--apply")
	if code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
}

func TestFindsLodgectlBesideLodgesBinary(t *testing.T) {
	root := stage(t)
	stageLodgectlBesideLodge(t, root)
	lodgectl := filepath.Join(root, "opt", "lodge", "bin", "lodgectl")
	// A host lodgectl must not stand in for lodge's: PATH is the stubs' and
	// the system's own only.
	path := "/usr/bin:/bin"
	before := snapshot(t, root)
	out, code := moveWithPath(t, root, path)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	if want := "✓ lodge is installed and up (" + lodgectl + ")"; !strings.Contains(out, want) {
		t.Errorf("the dry run's output lacks %q:\n%s", want, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the dry run changed %s", d)
	}
	out, code = moveWithPath(t, root, path, "--apply", "--url", url)
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	if c := changes(t, root); !slices.Contains(c, "lodgectl reload") {
		t.Errorf("--apply does not reload lodge through %s: %q", lodgectl, c)
	}
}

func TestRefusesWithoutLodgectl(t *testing.T) {
	root := stage(t)
	if err := os.Remove(filepath.Join(root, "stub", "bin", "lodgectl")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "stub/units/lodge.service/ExecStart", "{ path=/opt/lodge/bin/lodge ; argv[]=/opt/lodge/bin/lodge ; ignore_errors=no }\n")
	before := snapshot(t, root)
	out, code := moveWithPath(t, root, "/usr/bin:/bin")
	want := "no lodgectl on PATH or beside lodge.service's binary (/opt/lodge/bin/lodge): set LODGECTL=/path/to/lodgectl"
	if code == 0 || !strings.Contains(out, want) || !strings.Contains(out, "nothing changed") {
		t.Fatalf("exit %d, output lacks %q or \"nothing changed\":\n%s", code, want, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("a refusal changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 {
		t.Errorf("a refusal ran %q", c)
	}
}

func TestTheDrainWaitsForACycleAfterTheRestart(t *testing.T) {
	root := stage(t)
	stageHubCopy(t, root)
	// After notesd restarted, its status shows zero counts until its loop
	// has counted from the database: a state that is not idle is not yet a
	// count, however level it looks.
	writeFile(t, root, "var/lib/frostnotes/syncstatus-2-drain",
		"2\tMy Notes\thttp://127.0.0.1:7880\tsyncing\t8702/8702\t0 pending\t0 conflicted\n")
	before := snapshot(t, root)
	out, code := move(t, root, "--apply", "--url", url)
	if code == 0 || !strings.Contains(out, "lodge's copy of My Notes did not drain into the hub (2\tMy Notes\thttp://127.0.0.1:7880\tsyncing\t8702/8702") {
		t.Fatalf("exit %d, the move took a status before a cycle as drained:\n%s", code, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the failed move changed %s", d)
	}
}

func TestRefusesACopyThatDoesNotDrain(t *testing.T) {
	root := stage(t)
	stageHubCopy(t, root)
	// The copy falls behind once lodge has stopped: it never reaches the
	// hub, and setting it aside would lose what it holds.
	writeFile(t, root, "var/lib/frostnotes/syncstatus-2-drain",
		"2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8700/8702\t0 pending\t0 conflicted\n")
	before := snapshot(t, root)
	// The dry run reads the copies, and prints what it would wait for.
	out, code := move(t, root)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	if want := "then wait for lodge's copy of My Notes to be idle and in step with the hub (notesctl sync status --vault 2), and verify it there"; !strings.Contains(out, want) {
		t.Errorf("the dry run does not say what it would wait for (%q):\n%s", want, out)
	}
	out, code = move(t, root, "--apply", "--url", url)
	want := "lodge's copy of My Notes did not drain into the hub (2\tMy Notes\thttp://127.0.0.1:7880\tidle\t8700/8702\t0 pending\t0 conflicted): nothing is replaced; roll back to restart lodge"
	if code == 0 || !strings.Contains(out, want) ||
		!strings.Contains(out, "roll back with: move-hub-behind-lodge.sh --rollback "+filepath.Join(root, backup)+" --apply") {
		t.Fatalf("exit %d, without the drain's failure or the rollback:\n%s", code, out)
	}
	// Nothing moved: the data directory, the trial's data and fstab are as
	// they were, and only the backup's directory and the stop of lodge ran.
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the failed move changed %s", d)
	}
	for _, c := range changes(t, root) {
		if !strings.HasPrefix(c, "mkdir ") && c != "systemctl stop lodge.service" && c != "systemctl restart frostnotes.service" {
			t.Errorf("the failed move ran %q", c)
		}
	}
	// The rollback puts lodge back, and leaves everything where it was.
	out, code = move(t, root, "--rollback", filepath.Join(root, backup), "--apply")
	if code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
	for unit, want := range map[string]string{"lodge.service": "active\n", "notesd.service": "active\n", "frostnotes.service": "active\n", "frostnotes-lodge.socket": "active\n"} {
		if got := readFile(t, root, "stub/units/"+unit+"/active"); got != want {
			t.Errorf("%s after the rollback is %q, want %q", unit, got, want)
		}
	}
	if c := changes(t, root); !slices.Contains(c, "systemctl start lodge.service") {
		t.Errorf("the rollback does not start lodge again: %q", c)
	}
}

func TestACopysURLNamesTheAddressTheHubListensOn(t *testing.T) {
	for name, tc := range map[string]struct {
		listen string
		url    string
		ok     bool
	}{
		"IPv4":                       {"listening on 127.0.0.1:7880", "http://127.0.0.1:7880", true},
		"IPv4 with a trailing slash": {"listening on 127.0.0.1:7880", "http://127.0.0.1:7880/", true},
		"IPv4 wildcard":              {"listening on 0.0.0.0:7880", "http://127.0.0.1:7880", true},
		"localhost":                  {"listening on 127.0.0.1:7880", "http://localhost:7880", false},
		"IPv6 on an IPv4 listener":   {"listening on 127.0.0.1:7880", "http://[::1]:7880", false},
		"IPv6":                       {"listening on [::1]:7880", "http://[::1]:7880", true},
		"IPv6 wildcard":              {"listening on [::]:7880", "http://[::1]:7880", true},
		"IPv4 on an IPv6 listener":   {"listening on [::]:7880", "http://127.0.0.1:7880", false},
	} {
		t.Run(name, func(t *testing.T) {
			root := stage(t)
			writeFile(t, root, "stub/listen", tc.listen+"\n")
			stageHubCopies(t, root, "2\tMy Notes\t"+tc.url+"\tidle\t8702/8702\t0 pending\t0 conflicted\n")
			out, code := move(t, root)
			if tc.ok {
				if code != 0 || !strings.Contains(out, "is a copy of the hub's vault My Notes ("+tc.url+")") {
					t.Fatalf("exit %d:\n%s", code, out)
				}
				return
			}
			if want := "syncs with " + tc.url + ", not with this machine's trial hub (port 7880)"; code == 0 || !strings.Contains(out, want) {
				t.Fatalf("exit %d, output lacks %q:\n%s", code, want, out)
			}
		})
	}
}
