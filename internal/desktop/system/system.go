// Package system is what the desktop app needs of the system around it
// (plan 0009): where its files live, the end of notesd's log, the
// Flatpak's background setting, and the open rules. The files and the
// setting are internal/flatpakapp's, which notesctl shares; this package
// names them for the clients. It imports no UI.
package system

import (
	"context"
	"os/exec"
	"time"

	"github.com/frostyard/frostnotes/internal/flatpakapp"
)

// shutdownMax bounds the stop of notesd when the last window closes.
const shutdownMax = 35 * time.Second

// Getenv reads the environment; tests pass their own.
type Getenv = flatpakapp.Getenv

// Background is the setting and the portal's last answer, for the window.
type Background = flatpakapp.Background

// PortalAnswer is the portal's last message, as the helper wrote it.
type PortalAnswer = flatpakapp.PortalAnswer

// NotesdLog is what the window learns from notesd.log.
type NotesdLog = flatpakapp.NotesdLog

// PortalFailure starts the launcher's line when the Flatpak portal did not
// start notesd.
const PortalFailure = flatpakapp.PortalFailure

// InFlatpak reports whether the app runs in its own Flatpak.
func InFlatpak(getenv Getenv) bool { return flatpakapp.InFlatpak(getenv) }

// ConfigDir is the app's config directory (flatpakapp.ConfigDir).
func ConfigDir(getenv Getenv) (string, error) { return flatpakapp.ConfigDir(getenv) }

// StateDir is the app's state directory (flatpakapp.StateDir).
func StateDir(getenv Getenv) (string, error) { return flatpakapp.StateDir(getenv) }

// WriteFile writes data to path atomically (flatpakapp.WriteFile).
func WriteFile(path string, data []byte) error { return flatpakapp.WriteFile(path, data) }

// GetBackground reports the setting in the Flatpak (flatpakapp.GetBackground).
func GetBackground(getenv Getenv) (Background, bool) { return flatpakapp.GetBackground(getenv) }

// SetBackground writes the setting and asks the portal again, without
// waiting: the window's sandbox outlives the request
// (flatpakapp.SetBackground).
func SetBackground(getenv Getenv, keepRunning bool) (Background, error) {
	return flatpakapp.SetBackground(getenv, keepRunning)
}

// ReadNotesdLog reports the end of notesd.log in the Flatpak
// (flatpakapp.ReadNotesdLog).
func ReadNotesdLog(getenv Getenv) (NotesdLog, bool) { return flatpakapp.ReadNotesdLog(getenv) }

// StopOnExit stops notesd when the app exits with the setting Off, so
// that Off holds also where the desktop's own background permission is on.
// Only the process that owns the window calls it, and only the Flatpak
// stops notesd: elsewhere notesd is a service of its own.
func StopOnExit(ctx context.Context, getenv Getenv) {
	b, ok := GetBackground(getenv)
	if !ok || b.KeepRunning {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, shutdownMax)
	defer cancel()
	_ = exec.CommandContext(ctx, "notesctl", "shutdown").Run()
}
