//go:build !windows

package config

import (
	"errors"
	"path/filepath"
)

// xdg is an XDG base directory: the variable when it is absolute, else
// fallback under HOME.
func xdg(getenv func(string) string, name, fallback string) (string, error) {
	if v := getenv(name); filepath.IsAbs(v) {
		return v, nil
	}
	home := getenv("HOME")
	if home == "" {
		return "", errors.New("HOME is not set")
	}
	return filepath.Join(home, fallback), nil
}

func defaultDataDir(getenv func(string) string) (string, error) {
	dir, err := xdg(getenv, "XDG_DATA_HOME", ".local/share")
	return filepath.Join(dir, "frostnotes"), err
}

// DefaultSocket is notesd's socket when FROSTNOTES_SOCKET is not set:
// $XDG_RUNTIME_DIR/frostnotes/notesd.sock. In the Flatpak (FLATPAK_ID is
// AppID) it is $XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock:
// Flatpak shares $XDG_RUNTIME_DIR/app/<id> between every sandbox of the
// app, and the rest of $XDG_RUNTIME_DIR is private to each sandbox, so the
// app and notesd (and notesctl) meet only there
// (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
// app/src-tauri/src/paths.rs must stay in step with this function.
func DefaultSocket(getenv func(string) string) (string, error) {
	run := getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(run) {
		return "", errors.New("XDG_RUNTIME_DIR is not set; set FROSTNOTES_SOCKET")
	}
	if getenv("FLATPAK_ID") == AppID {
		run = filepath.Join(run, "app", AppID)
	}
	return filepath.Join(run, "frostnotes", "notesd.sock"), nil
}
