package packagecheck

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// debCheck is the package check that make deb runs on every .deb it builds
// (frostyard/core ADR-0011, ADR-0058). These tests feed it hand-made .deb
// files, so each rule is shown to fail a bad package.
const debCheck = "../../packaging/deb-check.py"

// The package, a version, and the payload arguments make deb-check passes.
const debPkg, debVersion = "frostyard-notesd", "1.2.3"

var debArgs = []string{
	"--exec", "/usr/bin/notesd",
	"--exec", "/usr/bin/notesctl",
	"--file", "/usr/lib/systemd/system/frostnotes.service",
	"--file", "/usr/lib/systemd/system/frostnotes-lodge.socket",
	"--file", "/usr/lib/sysusers.d/frostnotes.conf",
	"--file", "/usr/share/lodge/apps/notes/web/index.html",
}

// debManifest is the --manifest path, or "" for none.
const debManifest = "/usr/share/lodge/apps/notes/app.json"

// A debEntry is one archive member of a hand-made .deb.
type debEntry struct {
	name     string
	typ      byte
	mode     int64
	uid, gid int
	body     string
	link     string
}

func file(name, body string) debEntry {
	return debEntry{name: name, typ: tar.TypeReg, mode: 0o644, body: body}
}

func exe(name string) debEntry {
	return debEntry{name: name, typ: tar.TypeReg, mode: 0o755, body: "\x7fELF"}
}

// goodDeb is the members of a package that passes: the directories, each
// --exec as a binary, each --file, and the manifest at debVersion.
func goodDeb() []debEntry {
	out := []debEntry{{name: "./usr/", typ: tar.TypeDir, mode: 0o755}, {name: "./usr/bin/", typ: tar.TypeDir, mode: 0o755}}
	for i := 0; i+1 < len(debArgs); i += 2 {
		if debArgs[i] == "--exec" {
			out = append(out, exe("."+debArgs[i+1]))
		} else {
			out = append(out, file("."+debArgs[i+1], "x"))
		}
	}
	if debManifest != "" {
		out = append(out, file("."+debManifest, `{"contract": 1, "version": "`+debVersion+`"}`))
	}
	return out
}

// firstArg is the path of the first debArgs flag named flag.
func firstArg(flag string) string {
	for i := 0; i+1 < len(debArgs); i += 2 {
		if debArgs[i] == flag {
			return debArgs[i+1]
		}
	}
	panic("no " + flag + " in debArgs")
}

func tgz(t *testing.T, entries []debEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: e.mode, Uid: e.uid, Gid: e.gid, Linkname: e.link, Size: int64(len(e.body))}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeDeb writes an ar archive in the .deb layout: debian-binary, then the
// control and data tarballs.
func writeDeb(t *testing.T, control map[string]string, data []debEntry) string {
	t.Helper()
	var ctrl []debEntry
	for name, body := range control {
		ctrl = append(ctrl, debEntry{name: "./" + name, typ: tar.TypeReg, mode: 0o644, body: body})
	}
	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	for _, m := range []struct {
		name string
		body []byte
	}{
		{"debian-binary", []byte("2.0\n")},
		{"control.tar.gz", tgz(t, ctrl)},
		{"data.tar.gz", tgz(t, data)},
	} {
		fmt.Fprintf(&ar, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", m.name, 0, 0, 0, "100644", len(m.body))
		ar.Write(m.body)
		if len(m.body)%2 == 1 {
			ar.WriteByte('\n')
		}
	}
	path := filepath.Join(t.TempDir(), "test.deb")
	if err := os.WriteFile(path, ar.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func controlFile(version string) string {
	return fmt.Sprintf("Package: %s\nVersion: %s\nArchitecture: amd64\nMaintainer: x <x@example.com>\nDepends: systemd\nDescription: test\n", debPkg, version)
}

func runDebCheck(t *testing.T, deb string) (string, error) {
	t.Helper()
	for _, tool := range []string{"python3", "dpkg-deb"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	args := append([]string{debCheck, deb, debPkg, debVersion}, debArgs...)
	if debManifest != "" {
		args = append(args, "--manifest", debManifest)
	}
	out, err := exec.Command("python3", args...).CombinedOutput()
	return string(out), err
}

func TestDebCheckPassesAGoodPackage(t *testing.T) {
	deb := writeDeb(t, map[string]string{"control": controlFile(debVersion)}, goodDeb())
	if out, err := runDebCheck(t, deb); err != nil {
		t.Fatalf("deb-check failed a good package: %v\n%s", err, out)
	}
}

// A debCase is a bad package and the error deb-check must give for it.
type debCase struct {
	name    string
	control map[string]string
	data    []debEntry
	want    string
}

func TestDebCheckFailsABadPackage(t *testing.T) {
	without := func(name string) []debEntry {
		var out []debEntry
		for _, e := range goodDeb() {
			if e.name != name {
				out = append(out, e)
			}
		}
		return out
	}
	with := func(extra ...debEntry) []debEntry { return append(goodDeb(), extra...) }
	good := map[string]string{"control": controlFile(debVersion)}
	bin, req := firstArg("--exec"), firstArg("--file")
	cases := []debCase{
		{"wrong version", map[string]string{"control": controlFile("9.9.9")}, goodDeb(), "Version is"},
		{"a maintainer script", map[string]string{"control": controlFile(debVersion), "postinst": "#!/bin/sh\n"}, goodDeb(), "maintainer scripts"},
		{"a path under /etc", good, with(file("./etc/lodge/x", "x")), "not under /usr"},
		{"a path that climbs out of /usr", good, with(file("./usr/../etc/x", "x")), "not under /usr"},
		{"an absolute path", good, with(file("/usr/share/x", "x")), "not under /usr"},
		{"a link out of /usr", good, with(debEntry{name: "./usr/share/x", typ: tar.TypeSymlink, mode: 0o777, link: "../../etc/shadow"}), "outside /usr"},
		{"a group-writable file", good, with(debEntry{name: "./usr/share/x", typ: tar.TypeReg, mode: 0o664, body: "x"}), "writable"},
		{"a file not owned by root", good, with(debEntry{name: "./usr/share/x", typ: tar.TypeReg, mode: 0o644, uid: 1000, body: "x"}), "owned by"},
		{"a missing binary named with a space", good, append(without("."+bin), exe("."+bin+" backup")), bin + " is missing"},
		{"a binary that is not executable", good, append(without("."+bin), file("."+bin, "\x7fELF")), "mode 0755"},
		{"a binary that is a link", good, append(without("."+bin), debEntry{name: "." + bin, typ: tar.TypeSymlink, mode: 0o777, link: "missing"}), bin + " is missing"},
		{"an empty required file", good, append(without("."+req), file("."+req, "")), "is empty"},
	}
	if debManifest != "" {
		m := "." + debManifest
		cases = append(cases,
			debCase{"a manifest at another version", good, append(without(m), file(m, `{"version": "0x2x3", "x": {"version": "`+debVersion+`"}}`)), "has version"},
			debCase{"a manifest that is not JSON", good, append(without(m), file(m, `"version": "`+debVersion+`"`)), "not JSON"},
		)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runDebCheck(t, writeDeb(t, c.control, c.data))
			if err == nil {
				t.Fatalf("deb-check passed a package with %s:\n%s", c.name, out)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("deb-check's output lacks %q:\n%s", c.want, out)
			}
		})
	}
}
