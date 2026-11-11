package system

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
	"time"
)

// The Flatpak's setting for background activity
// (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md). Off
// is a file, $XDG_CONFIG_HOME/frostnotes/background-off, which the launcher
// and this package both read; On is its absence. The portal's last answer
// is the helper's: frostnotes-request-background writes background.json,
// or background-failure-<generation>.json when it cannot take its lock.
const (
	offFile     = "background-off"
	answerFile  = "background.json"
	logFile     = "background.log"
	genFile     = "background-gen"
	helper      = "frostnotes-request-background"
	libexec     = "/app/libexec"
	shutdownMax = 35 * time.Second
)

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

// Background is the setting and the portal's last answer, for the window.
type Background struct {
	// KeepRunning: notesd keeps syncing after the last window closes, and
	// the portal may start it at login.
	KeepRunning bool
	Portal      *PortalAnswer
	// Generation is this change's number, 0 from GetBackground and when
	// the helper could not start.
	Generation uint64
}

// GetBackground reports the setting in the Flatpak; ok is false anywhere
// else, where notesd is a service the desktop and systemd run.
func GetBackground(getenv Getenv) (b Background, ok bool) {
	if !InFlatpak(getenv) {
		return Background{}, false
	}
	return backgroundState(getenv), true
}

// SetBackground writes the setting, asks the Background portal again, and
// answers the new state.
func SetBackground(getenv Getenv, keepRunning bool) (Background, error) {
	if !InFlatpak(getenv) {
		return Background{}, errors.New("the background setting belongs to the Flatpak")
	}
	return setBackground(getenv, keepRunning)
}

func offPath(getenv Getenv) (string, error) {
	d, err := ConfigDir(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, offFile), nil
}

func setBackground(getenv Getenv, keepRunning bool) (Background, error) {
	off, err := offPath(getenv)
	if err != nil {
		return Background{}, err
	}
	if keepRunning {
		if err := os.Remove(off); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Background{}, fmt.Errorf("remove %s: %w", off, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(off), 0o755); err != nil {
			return Background{}, fmt.Errorf("create %s: %w", filepath.Dir(off), err)
		}
		if err := os.WriteFile(off, []byte("Off: notesd stops when the last window closes (ADR-0033).\n"), 0o644); err != nil {
			return Background{}, fmt.Errorf("write %s: %w", off, err)
		}
	}
	// The file first, and only then the counter: a helper that reads the
	// counter after this moment answers about this setting.
	gen, err := bumpGeneration(getenv)
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

// StopOnExit stops notesd when the app exits with the setting Off, so
// that Off holds also where the desktop's own background permission is on.
// Only the process that owns the window calls it, and only the Flatpak
// stops notesd: elsewhere notesd is a service of its own.
func StopOnExit(ctx context.Context, getenv Getenv) {
	if !InFlatpak(getenv) {
		return
	}
	off, err := offPath(getenv)
	if err != nil {
		return
	}
	if _, err := os.Stat(off); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, shutdownMax)
	defer cancel()
	_ = exec.CommandContext(ctx, "notesctl", "shutdown").Run()
}
