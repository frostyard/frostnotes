package main

// CONTRACT TEST for task card T-0009 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// files reads every file under dir, by slash-separated relative path.
func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRunWritesTheVaultAndManifest(t *testing.T) {
	dir := t.TempDir()
	out, manifest := filepath.Join(dir, "vault"), filepath.Join(dir, "links.json")
	var stdout bytes.Buffer
	if err := run([]string{"-out", out, "-notes", "40", "-seed", "3", "-manifest", manifest}, &stdout); err != nil {
		t.Fatal(err)
	}
	want, err := vaultgen.Generate(filepath.Join(dir, "same"), vaultgen.Options{Seed: 3, Notes: 40})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(files(t, out), files(t, filepath.Join(dir, "same"))) {
		t.Fatal("the vault differs from vaultgen.Generate's with the same options")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var got vaultgen.Manifest
	if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %+v, %v; want Generate's", got, err)
	}
	if !strings.HasPrefix(string(data), "{\n  \"seed\": 3,") || !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("manifest is not indented JSON ending in a newline: %.40q…", data)
	}
	line := fmt.Sprintf("wrote 40 notes, %d attachments and %d links to %s\n", len(want.Attachments), len(want.Links), out)
	if stdout.String() != line {
		t.Errorf("stdout = %q, want %q", stdout.String(), line)
	}
}

func TestRunDefaults(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "vault")
	var stdout bytes.Buffer
	if err := run([]string{"-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	want, err := vaultgen.Generate(filepath.Join(dir, "same"), vaultgen.Options{Seed: 1, Notes: 666})
	if err != nil {
		t.Fatal(err)
	}
	if got := files(t, out); len(got) != len(want.Notes)+len(want.Attachments) || !maps.Equal(got, files(t, filepath.Join(dir, "same"))) {
		t.Fatal("the default vault is not Generate's with seed 1 and 666 notes")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("without -manifest, run wrote %d entries next to the vault, want none", len(entries)-2)
	}
}

func TestRunErrors(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "full")
	if err := os.MkdirAll(filepath.Join(full, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no -out", []string{"-notes", "30"}, "-out"},
		{"too few notes", []string{"-out", filepath.Join(dir, "a"), "-notes", "19"}, "20"},
		{"not empty", []string{"-out", full}, "not empty"},
		{"unknown flag", []string{"-out", filepath.Join(dir, "b"), "-size", "3"}, "-size"},
		{"arguments", []string{"-out", filepath.Join(dir, "c"), "extra"}, "extra"},
		{"manifest folder missing", []string{"-out", filepath.Join(dir, "d"), "-manifest", filepath.Join(dir, "no", "m.json")}, "manifest"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run(tc.args, &stdout)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run(%q) = %v, want an error mentioning %q", tc.args, err, tc.want)
			}
		})
	}
	if entries, _ := os.ReadDir(filepath.Join(full)); len(entries) != 1 {
		t.Errorf("run wrote into a folder that was not empty")
	}
}
