package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEscape(t *testing.T) {
	for in, want := range map[string]string{
		"github.com/spf13/cobra":       "github.com/spf13/cobra",
		"github.com/BurntSushi/toml":   "github.com/!burnt!sushi/toml",
		"v1.0.0-RC1":                   "v1.0.0-!r!c1",
		"golang.org/x/text":            "golang.org/x/text",
		"github.com/Azure/go-ansiterm": "github.com/!azure/go-ansiterm",
	} {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSourcesAreSortedAndHashed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mods := []module{
		{Path: "z.example/mod", Version: "v1.2.3", GoMod: write("z.mod", "zmod"), Zip: write("z.zip", "zzip")},
		{Path: "github.com/Foo/bar", Version: "v0.1.0-RC1", GoMod: write("f.mod", "fmod"), Zip: write("f.zip", "fzip")},
	}
	sum := "github.com/Foo/bar v0.1.0-RC1 h1:a=\ngithub.com/Foo/bar v0.1.0-RC1/go.mod h1:b=\n" +
		"z.example/mod v1.2.3 h1:c=\nz.example/mod v1.2.3/go.mod h1:d=\nunused.example/x v1.0.0 h1:e=\n"
	got, err := sources(mods, []byte(sum))
	if err != nil {
		t.Fatal(err)
	}
	sha := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	want := []source{
		{"file", "https://proxy.golang.org/github.com/!foo/bar/@v/v0.1.0-!r!c1.mod", sha("fmod"), "goproxy/github.com/!foo/bar/@v"},
		{"file", "https://proxy.golang.org/github.com/!foo/bar/@v/v0.1.0-!r!c1.zip", sha("fzip"), "goproxy/github.com/!foo/bar/@v"},
		{"file", "https://proxy.golang.org/z.example/mod/@v/v1.2.3.mod", sha("zmod"), "goproxy/z.example/mod/@v"},
		{"file", "https://proxy.golang.org/z.example/mod/@v/v1.2.3.zip", sha("zzip"), "goproxy/z.example/mod/@v"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSourcesNeedBothSumLines(t *testing.T) {
	mods := []module{{Path: "a.example/m", Version: "v1.0.0", GoMod: "x", Zip: "y"}}
	_, err := sources(mods, []byte("a.example/m v1.0.0/go.mod h1:b=\n"))
	if err == nil || !strings.Contains(err.Error(), "go.sum has no line") {
		t.Fatalf("sources error = %v, want a missing go.sum line", err)
	}
}

func TestSourcesReportDownloadErrors(t *testing.T) {
	_, err := sources([]module{{Path: "a.example/m", Version: "v1.0.0", Error: "boom"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("sources error = %v, want boom", err)
	}
}
