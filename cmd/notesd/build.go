package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/frostyard/frostnotes/internal/config"
)

// defaultFlatpakInfo is the file a Flatpak sandbox describes itself in;
// FROSTNOTES_FLATPAK_INFO replaces it, for tests.
const defaultFlatpakInfo = "/.flatpak-info"

// printBuild is -build: the Flatpak build of this instance.
func printBuild(w io.Writer) error {
	build, err := flatpakBuild(os.Getenv)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, build)
	return err
}

// flatpakBuild is the app-commit of the [Instance] group of the sandbox's
// .flatpak-info: it differs for every build of the Flatpak, even of one
// version. The launcher compares it with the latest installed build's
// (notesd -build run through flatpak-spawn --latest-version) to replace an
// older notesd (docs/adr/0032-an-update-replaces-the-running-notesd.md).
func flatpakBuild(getenv func(string) string) (string, error) {
	path := getenv("FROSTNOTES_FLATPAK_INFO")
	if path == "" {
		path = defaultFlatpakInfo
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	var inInstance bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inInstance = line == "[Instance]"
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && inInstance && strings.TrimSpace(key) == "app-commit" {
			if value = strings.TrimSpace(value); value != "" {
				return value, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return "", fmt.Errorf("%s has no app-commit in [Instance]", path)
}

// serverBuild is what rpc.hello reports as build: the Flatpak build, only
// in this application's Flatpak and only when it is known.
func serverBuild(logger *slog.Logger) string {
	if !config.InFlatpak(os.Getenv) {
		return ""
	}
	build, err := flatpakBuild(os.Getenv)
	if err != nil {
		logger.Warn("the Flatpak build is unknown", "err", err)
		return ""
	}
	return build
}
