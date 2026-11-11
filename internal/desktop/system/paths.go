// Package system is what the desktop app needs of the system around it
// (plan 0009): where its files live, the end of notesd's log, and the
// Flatpak's background setting. It imports no UI.
package system

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/frostyard/frostnotes/internal/config"
)

// dir holds the app's files under the XDG base directories, as the
// launcher's $state does.
const dir = "frostnotes"

// Getenv reads the environment; tests pass their own.
type Getenv func(string) string

// InFlatpak reports whether the app runs in its own Flatpak.
func InFlatpak(getenv Getenv) bool { return config.InFlatpak(getenv) }

func abs(v string) (string, bool) { return v, filepath.IsAbs(v) }

func home(getenv Getenv) (string, error) {
	if h := getenv("HOME"); h != "" {
		return h, nil
	}
	return "", errors.New("HOME is not set")
}

// ConfigDir is $XDG_CONFIG_HOME/frostnotes, else ~/.config/frostnotes: in
// the Flatpak, the app's own, which all of its sandboxes share.
func ConfigDir(getenv Getenv) (string, error) {
	if d, ok := abs(getenv("XDG_CONFIG_HOME")); ok {
		return filepath.Join(d, dir), nil
	}
	h, err := home(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".config", dir), nil
}

// StateDir is $XDG_STATE_HOME/frostnotes, else ~/.local/state/frostnotes.
func StateDir(getenv Getenv) (string, error) {
	if d, ok := abs(getenv("XDG_STATE_HOME")); ok {
		return filepath.Join(d, dir), nil
	}
	h, err := home(getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".local", "state", dir), nil
}

// WriteFile writes data to path atomically: a temporary file in the same
// directory, then a rename, so a crash leaves the old file or the new one.
func WriteFile(path string, data []byte) error {
	d := filepath.Dir(path)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(d, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
