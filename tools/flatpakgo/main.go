// Command flatpakgo prints the Go modules notesd needs as flatpak-builder
// `file` sources, so a Flatpak builds offline (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
//
// Each module becomes 2 sources, its go.mod and its zip, fetched from
// proxy.golang.org into goproxy/<escaped module>/@v. The build then points
// GOPROXY at file://$PWD/goproxy. The modules are the ones `go mod download`
// fetches for this module, and each must have both of its go.sum lines.
// Run it from the repository root; it needs a populated module cache, which
// `go mod download` fills, so it runs that first and may use the network.
//
//	go run ./tools/flatpakgo [-gosum go.sum] > packaging/flatpak/go-sources.json
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
)

const proxy = "https://proxy.golang.org"

// source is a flatpak-builder `file` source.
type source struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Dest   string `json:"dest"`
}

// module is one entry of `go mod download -json`.
type module struct {
	Path    string
	Version string
	GoMod   string // the cached .mod file
	Zip     string // the cached .zip file
	Error   string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "flatpakgo:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("flatpakgo", flag.ContinueOnError)
	gosum := fs.String("gosum", "go.sum", "the go.sum to check the modules against")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sum, err := os.ReadFile(*gosum)
	if err != nil {
		return err
	}
	mods, err := download(context.Background())
	if err != nil {
		return err
	}
	srcs, err := sources(mods, sum)
	if err != nil {
		return err
	}
	out, err := json.Marshal(srcs, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(out, '\n'))
	return err
}

// download runs `go mod download -json` and decodes its stream of objects.
func download(ctx context.Context) ([]module, error) {
	cmd := exec.CommandContext(ctx, "go", "mod", "download", "-json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	// A failed module still prints its object, with Error; report that below.
	runErr := cmd.Run()
	dec := jsontext.NewDecoder(&stdout)
	var mods []module
	for {
		var m module
		if err := json.UnmarshalDecode(dec, &m); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go mod download: %w", err)
		}
		mods = append(mods, m)
	}
	if runErr != nil {
		return nil, fmt.Errorf("go mod download: %w", runErr)
	}
	return mods, nil
}

// sources turns downloaded modules into file sources, sorted by destination
// and name. sum is the go.sum text; a module without both of its lines is an
// error, since the offline build would fail on it.
func sources(mods []module, sum []byte) ([]source, error) {
	have, err := sumLines(sum)
	if err != nil {
		return nil, err
	}
	var out []source
	for _, m := range mods {
		if m.Error != "" {
			return nil, fmt.Errorf("%s@%s: %s", m.Path, m.Version, m.Error)
		}
		for _, line := range []string{m.Path + " " + m.Version, m.Path + " " + m.Version + "/go.mod"} {
			if !have[line] {
				return nil, fmt.Errorf("go.sum has no line for %q (run go mod tidy)", line)
			}
		}
		for _, f := range []struct{ file, ext string }{{m.GoMod, "mod"}, {m.Zip, "zip"}} {
			s, err := fileSource(m, f.file, f.ext)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b source) int { return strings.Compare(a.Dest+"/"+a.URL, b.Dest+"/"+b.URL) })
	return out, nil
}

func fileSource(m module, file, ext string) (source, error) {
	if file == "" {
		return source{}, fmt.Errorf("%s@%s: go mod download gave no .%s file", m.Path, m.Version, ext)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return source{}, err
	}
	sum := sha256.Sum256(data)
	dir := path.Join(escape(m.Path), "@v")
	return source{
		Type:   "file",
		URL:    proxy + "/" + dir + "/" + escape(m.Version) + "." + ext,
		SHA256: hex.EncodeToString(sum[:]),
		Dest:   "goproxy/" + dir,
	}, nil
}

// sumLines returns the "module version[/go.mod]" part of each go.sum line.
func sumLines(sum []byte) (map[string]bool, error) {
	have := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(sum))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		if len(f) != 3 {
			return nil, fmt.Errorf("go.sum: malformed line %q", sc.Text())
		}
		have[f[0]+" "+f[1]] = true
	}
	return have, sc.Err()
}

// escape is the module proxy's case encoding: each capital letter becomes
// "!" and its lower-case letter.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}
