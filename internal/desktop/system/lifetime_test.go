package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostnotes/internal/config"
)

// flatpak is an environment inside the app's Flatpak, in temp directories;
// it answers the app's config directory.
func flatpak(t *testing.T) (Getenv, string) {
	t.Helper()
	cfg := t.TempDir()
	env := map[string]string{"FLATPAK_ID": config.AppID, "XDG_CONFIG_HOME": cfg, "XDG_STATE_HOME": t.TempDir(), "HOME": t.TempDir(), "FROSTNOTES_LIBEXEC_DIR": t.TempDir()}
	return func(k string) string { return env[k] }, filepath.Join(cfg, "frostnotes")
}

func TestOnlyLastFlatpakWindowStopsBackgroundSync(t *testing.T) {
	env, cfg := flatpak(t)
	if err := WriteFile(filepath.Join(cfg, "background-off"), []byte("off")); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "notesctl"), []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$WINDOW_EXIT_LOG\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(bin, "shutdown.log")
	t.Setenv("PATH", bin)
	t.Setenv("WINDOW_EXIT_LOG", log)
	first, err := WindowLifetime(env)
	if err != nil {
		t.Fatal(err)
	}
	second, err := WindowLifetime(env)
	if err != nil {
		t.Fatal(err)
	}
	first()
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("first close called notesctl: %v", err)
	}
	second()
	got, err := os.ReadFile(log)
	if err != nil || string(got) != "shutdown" {
		t.Fatalf("last close: %q, %v", got, err)
	}
}
