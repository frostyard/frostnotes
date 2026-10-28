package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFlatpakBuildReadsAppCommitOfInstance(t *testing.T) {
	const info = `[Application]
name=org.frostyard.Frostnotes
app-commit=not-this-one

[Instance]
instance-id=123
app-path=/x
app-commit = 0a1b2c3d
runtime-commit=ffff

[Other]
app-commit=nor-this
`
	path := filepath.Join(t.TempDir(), "info")
	writeFile(t, path, info)
	got, err := flatpakBuild(func(k string) string {
		if k == "FROSTNOTES_FLATPAK_INFO" {
			return path
		}
		return ""
	})
	if err != nil || got != "0a1b2c3d" {
		t.Fatalf("flatpakBuild = %q, %v; want 0a1b2c3d", got, err)
	}
}

func TestFlatpakBuildErrors(t *testing.T) {
	dir := t.TempDir()
	noKey := filepath.Join(dir, "nokey")
	writeFile(t, noKey, "[Instance]\ninstance-id=1\n[Application]\napp-commit=x\n")
	empty := filepath.Join(dir, "empty")
	writeFile(t, empty, "[Instance]\napp-commit=\n")
	for name, path := range map[string]string{"missing file": filepath.Join(dir, "none"), "no key": noKey, "empty value": empty} {
		t.Run(name, func(t *testing.T) {
			got, err := flatpakBuild(func(string) string { return path })
			if err == nil || got != "" {
				t.Fatalf("flatpakBuild = %q, %v; want an error", got, err)
			}
		})
	}
}

func TestPrintBuildWritesNothingOnError(t *testing.T) {
	t.Setenv("FROSTNOTES_FLATPAK_INFO", filepath.Join(t.TempDir(), "none"))
	var out bytes.Buffer
	if err := printBuild(&out); err == nil || out.Len() != 0 {
		t.Fatalf("printBuild = %v, %q; want an error and no output", err, out.String())
	}
	path := filepath.Join(t.TempDir(), "info")
	writeFile(t, path, "[Instance]\napp-commit=abc\n")
	t.Setenv("FROSTNOTES_FLATPAK_INFO", path)
	if err := printBuild(&out); err != nil || out.String() != "abc\n" {
		t.Fatalf("printBuild = %v, %q; want abc", err, out.String())
	}
}

// hello reports a build only inside this application's Flatpak.
func TestServerBuildOnlyInTheFlatpak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "info")
	writeFile(t, path, "[Instance]\napp-commit=abc\n")
	t.Setenv("FROSTNOTES_FLATPAK_INFO", path)
	for id, want := range map[string]string{"": "", "org.example.Other": "", "org.frostyard.Frostnotes": "abc"} {
		t.Setenv("FLATPAK_ID", id)
		if got := serverBuild(nil); got != want {
			t.Errorf("serverBuild with FLATPAK_ID=%q = %q; want %q", id, got, want)
		}
	}
}
