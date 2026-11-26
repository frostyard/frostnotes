package flatpakapp

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// The Flatpak's setting for background activity
// (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md). Off
// is a file, $XDG_CONFIG_HOME/frostnotes/background-off, which the launcher
// and this package both read; On is its absence. The portal's last answer
// is the helper's: frostnotes-request-background writes background.json,
// or background-failure-<generation>.json when it cannot take its lock.
const (
	offFile    = "background-off"
	answerFile = "background.json"
	logFile    = "background.log"
	genFile    = "background-gen"
	helper     = "frostnotes-request-background"
	libexec    = "/app/libexec"
)

// offText is what the off file says to a person who finds it.
const offText = "Off: notesd does not start at login; `notesctl background on` turns it on (ADR-0033).\n"

var failureRe = regexp.MustCompile(`^background-failure-([0-9]+)\.json$`)

// PortalAnswer is the portal's last message, as the helper wrote it.
type PortalAnswer struct {
	At                 string  `json:"at"`
	AutostartRequested bool    `json:"autostartRequested"`
	Response           *uint8  `json:"response"`
	Background         *bool   `json:"background"`
	Autostart          *bool   `json:"autostart"`
	Error              *string `json:"error"`
	Generation         *uint64 `json:"generation"`
}

// Background is the setting and the portal's last answer.
type Background struct {
	// KeepRunning: the portal may start notesd at login, and notesd keeps
	// syncing with no client.
	KeepRunning bool          `json:"keepRunning"`
	Portal      *PortalAnswer `json:"portal"`
	// Generation is this change's number, 0 from GetBackground and when
	// the helper could not start.
	Generation uint64 `json:"generation,omitzero"`
}

// GetBackground reports the setting in the Flatpak; ok is false anywhere
// else, where notesd is a service the desktop and systemd run.
func GetBackground(getenv Getenv) (b Background, ok bool) {
	if !InFlatpak(getenv) {
		return Background{}, false
	}
	return backgroundState(getenv), true
}

// errNotFlatpak answers a change of the setting outside the Flatpak.
var errNotFlatpak = errors.New("the background setting belongs to the Flatpak org.frostyard.Frostnotes; elsewhere notesd runs as its service manager starts it")

// SetBackground writes the setting, starts the Background portal's request
// without waiting for it, and answers the new state. A process that stays
// in its sandbox until the portal answers, as a window does, can use it;
// one that exits sooner uses Change and Ask.
func SetBackground(getenv Getenv, keepRunning bool) (Background, error) {
	gen, err := Change(getenv, keepRunning)
	if err != nil {
		return Background{}, err
	}
	started := askPortal(getenv)
	b := backgroundState(getenv)
	if started {
		b.Generation = gen
	}
	return b, nil
}

// Change writes the setting and counts the change, in the Flatpak only. It
// answers the change's number: an answer of the portal whose generation is
// at least that read the new setting. Ask then asks the portal.
func Change(getenv Getenv, keepRunning bool) (uint64, error) {
	if !InFlatpak(getenv) {
		return 0, errNotFlatpak
	}
	off, err := offPath(getenv)
	if err != nil {
		return 0, err
	}
	if keepRunning {
		if err := os.Remove(off); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, fmt.Errorf("remove %s: %w", off, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(off), 0o755); err != nil {
			return 0, fmt.Errorf("create %s: %w", filepath.Dir(off), err)
		}
		if err := os.WriteFile(off, []byte(offText), 0o644); err != nil {
			return 0, fmt.Errorf("write %s: %w", off, err)
		}
	}
	// The file first, and only then the counter: a helper that reads the
	// counter after this moment answers about this setting.
	return bumpGeneration(getenv)
}

// Ask runs the Background portal's request helper and waits until it
// exits: it stays on the bus until the portal answers, at most 10 minutes,
// and the portal drops a request whose caller left. The helper reads the
// setting itself, writes its outcome for GetBackground and always exits 0;
// Ask fails only when it cannot run it, or when ctx ends first. Its output
// is appended to background.log in the state directory.
func Ask(ctx context.Context, getenv Getenv) error {
	if !InFlatpak(getenv) {
		return errNotFlatpak
	}
	d, err := StateDir(getenv)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(d, logFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, helperPath(getenv))
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ask the Background portal (%s): %w", cmd.Path, err)
	}
	return nil
}

func offPath(getenv Getenv) (string, error) {
	d, err := ConfigDir(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, offFile), nil
}

func backgroundState(getenv Getenv) Background {
	b := Background{KeepRunning: true}
	if off, err := offPath(getenv); err == nil {
		if _, err := os.Stat(off); err == nil {
			b.KeepRunning = false
		}
	}
	if d, err := StateDir(getenv); err == nil {
		b.Portal = newest(readAnswer(filepath.Join(d, answerFile)), newestFailure(d))
	}
	return b
}

func readAnswer(path string) *PortalAnswer {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var a PortalAnswer
	if err := json.Unmarshal(data, &a); err != nil {
		return nil
	}
	return &a
}

func newestFailure(d string) *PortalAnswer {
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil
	}
	var best uint64
	name := ""
	for _, e := range entries {
		m := failureRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if n, err := strconv.ParseUint(m[1], 10, 64); err == nil && (name == "" || n > best) {
			best, name = n, e.Name()
		}
	}
	if name == "" {
		return nil
	}
	return readAnswer(filepath.Join(d, name))
}

// newest is the message the window shows: the newer of the helper's answer
// and a failure, an answer winning a tie. A failure without a generation
// is no answer at all.
func newest(answer, failure *PortalAnswer) *PortalAnswer {
	if failure == nil || failure.Generation == nil {
		return answer
	}
	if answer == nil || answer.Generation == nil || *failure.Generation > *answer.Generation {
		return failure
	}
	return answer
}

func helperPath(getenv Getenv) string {
	if d, ok := abs(getenv("FROSTNOTES_LIBEXEC_DIR")); ok {
		return filepath.Join(d, helper)
	}
	return filepath.Join(libexec, helper)
}

// askPortal starts the helper in the background, its output appended to
// background.log, in a process group of its own, and reports whether it
// started. Nothing waits for it: it stays on the bus until the portal
// answers.
func askPortal(getenv Getenv) bool {
	d, err := StateDir(getenv)
	if err != nil || os.MkdirAll(d, 0o755) != nil {
		return false
	}
	log, err := os.OpenFile(filepath.Join(d, logFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	defer log.Close()
	cmd := exec.Command(helperPath(getenv)) //nolint:noctx // it outlives this call on purpose
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cmd.Start() != nil {
		return false
	}
	go func() { _ = cmd.Wait() }() // reap it
	return true
}

func generation(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func bumpGeneration(getenv Getenv) (uint64, error) {
	d, err := StateDir(getenv)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(d, genFile)
	next := generation(path) + 1
	if err := WriteFile(path, []byte(strconv.FormatUint(next, 10)+"\n")); err != nil {
		return 0, err
	}
	return next, nil
}
