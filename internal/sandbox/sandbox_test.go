package sandbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// info is a /.flatpak-info whose [Context] holds filesystems.
func info(filesystems string) string {
	return "[Application]\nname=org.frostyard.Frostnotes\nruntime=runtime/org.gnome.Platform/x86_64/48\n\n" +
		"[Context]\nshared=network;ipc;\nsockets=wayland;fallback-x11;\nfilesystems=" + filesystems + "\n\n" +
		"[Instance]\ninstance-id=1\n"
}

func grants(t *testing.T, filesystems string, env Env) *Grants {
	t.Helper()
	g, err := Parse(strings.NewReader(info(filesystems)), env)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

var home = Env{Home: "/home/ana", RuntimeDir: "/run/user/1000"}

// defaultDirs is a user-dirs.dirs that names the XDG default folders.
const defaultDirs = `# a comment
XDG_DESKTOP_DIR="$HOME/Desktop"
XDG_DOWNLOAD_DIR="$HOME/Downloads"
XDG_TEMPLATES_DIR="$HOME/Templates"
XDG_PUBLICSHARE_DIR="$HOME/Public"
XDG_DOCUMENTS_DIR="$HOME/Documents"
XDG_MUSIC_DIR="$HOME/Music"
XDG_PICTURES_DIR="$HOME/Pictures"
XDG_VIDEOS_DIR="$HOME/Videos"
`

// writeUserDirs writes a user-dirs.dirs in a new directory and returns the directory.
func writeUserDirs(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "user-dirs.dirs"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// withDirs is home with the user's folders named by defaultDirs.
func withDirs(t *testing.T) Env {
	e := home
	e.ConfigHome = writeUserDirs(t, defaultDirs)
	return e
}

func TestAccess(t *testing.T) {
	for _, tc := range []struct {
		name        string
		filesystems string
		path        string
		want        Mode
	}{
		{"no grants", "", "/home/ana/Notes", None},
		{"host", "host;", "/srv/notes", ReadWrite},
		{"host and the root", "host;", "/", ReadWrite},
		{"host, read only", "host:ro;", "/srv/notes", ReadOnly},
		{"home covers the home", "home;", "/home/ana/Notes", ReadWrite},
		{"home covers the home itself", "home;", "/home/ana", ReadWrite},
		{"home does not cover a sibling", "home;", "/home/anabel/Notes", None},
		{"home does not cover elsewhere", "home;", "/srv/notes", None},
		{"home, read only", "home:ro;", "/home/ana/Notes", ReadOnly},
		{"home with create", "home:create;", "/home/ana/Notes", ReadWrite},
		{"explicit rw", "home:rw;", "/home/ana/Notes", ReadWrite},
		{"a home subpath", "~/Notes;", "/home/ana/Notes/Work", ReadWrite},
		{"a home subpath, the vault above it", "~/Notes/Work;", "/home/ana/Notes", None},
		{"a home subpath, a sibling", "~/Notes;", "/home/ana/Notes2", None},
		{"an absolute path", "/srv/notes;", "/srv/notes/a", ReadWrite},
		{"an absolute path, read only", "/srv/notes:ro;", "/srv/notes/a", ReadOnly},
		{"a lookalike of an absolute path", "/srv/notes;", "/srv/notes-old", None},
		{"a negation under home", "home;!~/Secret;", "/home/ana/Secret/x", None},
		{"a negation leaves the rest", "home;!~/Secret;", "/home/ana/Notes", ReadWrite},
		{"a more specific grant beats a negation above it", "home;!~/Secret;~/Secret/Open;", "/home/ana/Secret/Open/a", ReadWrite},
		{"a more specific read-only grant", "home;~/Notes:ro;", "/home/ana/Notes/a", ReadOnly},
		{"a negation of home", "home;!home;~/Notes;", "/home/ana/Notes", ReadWrite},
		{"a negation of home, elsewhere", "home;!home;~/Notes;", "/home/ana/Other", None},
		{"a later entry of the same root wins", "~/Notes:ro;~/Notes;", "/home/ana/Notes", ReadWrite},
		{"a negated host", "host;!host;", "/srv/notes", None},
		{"xdg-documents, default", "xdg-documents;", "/home/ana/Documents/Notes", ReadWrite},
		{"xdg-documents, not Downloads", "xdg-documents;", "/home/ana/Downloads/Notes", None},
		{"xdg-documents with a subpath", "xdg-documents/Notes:ro;", "/home/ana/Documents/Notes/a", ReadOnly},
		{"xdg-documents with a subpath, beside it", "xdg-documents/Notes;", "/home/ana/Documents/Other", None},
		{"xdg-download", "xdg-download;", "/home/ana/Downloads/x", ReadWrite},
		{"xdg-public-share", "xdg-public-share;", "/home/ana/Public/x", ReadWrite},
		{"xdg-data", "xdg-data/notes;", "/home/ana/.local/share/notes/a", ReadWrite},
		{"xdg-config", "xdg-config;", "/home/ana/.config/x", ReadWrite},
		{"xdg-cache", "xdg-cache;", "/home/ana/.cache/x", ReadWrite},
		{"xdg-run", "xdg-run/app;", "/run/user/1000/app/x", ReadWrite},
		{"host-os covers no user path", "host-os;host-etc;", "/home/ana/Notes", None},
		{"an unknown token", "xdg-nothing;", "/home/ana/Notes", None},
		{"an unknown modifier", "home:bogus;", "/home/ana/Notes", None},
		{"a relative path", "home;", "Notes", None},
		{"an unclean path", "home;", "/home/ana/x/../Notes/", ReadWrite},
		{"an escaped semicolon in a path", `/srv/a\;b;`, "/srv/a;b/x", ReadWrite},
		{"an escaped space in a path", `/srv/a\sb;`, "/srv/a b/x", ReadWrite},
		{"a doubled semicolon", "home;;~/Notes:ro;", "/home/ana/Notes", ReadOnly},
		{"host dirs, defaults", "xdg-state;", "/home/ana/.local/state/x", ReadWrite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := grants(t, tc.filesystems, withDirs(t)).Access(tc.path); got != tc.want {
				t.Errorf("filesystems=%s: Access(%s) = %d, want %d", tc.filesystems, tc.path, got, tc.want)
			}
		})
	}
}

func TestAccessOfNoGrants(t *testing.T) {
	var g *Grants
	if got := g.Access("/home/ana"); got != None {
		t.Errorf("nil Grants: Access = %d, want None", got)
	}
}

func TestParseTakesTheContextSection(t *testing.T) {
	const text = "[Application]\nfilesystems=host\n\n[Session Bus Policy]\nfilesystems=host\n\n[Context]\n# a comment\nfilesystems=~/Notes;\n\n[Environment]\nfilesystems=host\n"
	g, err := Parse(strings.NewReader(text), home)
	if err != nil {
		t.Fatal(err)
	}
	if g.Access("/home/ana/Notes") != ReadWrite || g.Access("/srv") != None {
		t.Errorf("grants = %+v, want only ~/Notes", g.grants)
	}
	if g.App() != AppID {
		t.Errorf("App = %q, want %q", g.App(), AppID)
	}
}

func TestParseReadsTheAppName(t *testing.T) {
	g, err := Parse(strings.NewReader("[Application]\nname=org.example.Other\n"), home)
	if err != nil || g.App() != "org.example.Other" {
		t.Errorf("App = %q, %v", g.App(), err)
	}
	var none *Grants
	if none.App() != AppID {
		t.Errorf("nil Grants App = %q", none.App())
	}
}

func TestUserDirsFile(t *testing.T) {
	text := "# comment\nXDG_DOCUMENTS_DIR=\"$HOME/Papers\"\nXDG_DOWNLOAD_DIR=\"/data/dl\"\nXDG_MUSIC_DIR=\"relative\"\n" +
		"XDG_VIDEOS_DIR=\"$HOMEBREW/v\"\n"
	env := home
	env.ConfigHome = writeUserDirs(t, text)
	for _, tc := range []struct {
		fs   string
		path string
		want Mode
		sure bool
	}{
		{"xdg-documents;", "/home/ana/Papers/Notes", ReadWrite, true}, // renamed
		{"xdg-documents;", "/home/ana/Documents/Notes", None, true},   // no longer there
		{"xdg-download;", "/data/dl/x", ReadWrite, true},              // absolute
		{"xdg-music;", "/home/ana/Music/x", None, false},              // a relative value: not resolved
		{"xdg-videos;", "/home/ana/Videos/x", None, false},            // not $HOME exactly: not resolved
		{"xdg-pictures;", "/home/ana/Pictures/x", None, false},        // not named: not guessed
	} {
		g := grants(t, tc.fs, env)
		if got := g.Access(tc.path); got != tc.want || g.Certain() != tc.sure {
			t.Errorf("%s: Access(%s) = %d, Certain = %t; want %d, %t", tc.fs, tc.path, got, g.Certain(), tc.want, tc.sure)
		}
	}
}

// The file in $XDG_CONFIG_HOME (the sandbox's copy of the host's) comes
// first, then ~/.config's; with neither, a grant of a user folder is uncertain.
func TestUserDirsFileOrder(t *testing.T) {
	h := t.TempDir()
	if err := os.MkdirAll(filepath.Join(h, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h, ".config", "user-dirs.dirs"), []byte("XDG_PICTURES_DIR=\"$HOME/Fotos\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := grants(t, "xdg-pictures;", Env{Home: h})
	if !g.Certain() || g.Access(filepath.Join(h, "Fotos", "a")) != ReadWrite || g.Access(filepath.Join(h, "Pictures", "a")) != None {
		t.Errorf("~/.config only: grants = %+v, want %s/Fotos", g.grants, h)
	}

	cfg := writeUserDirs(t, "XDG_PICTURES_DIR=\"$HOME/Bilder\"\n")
	g = grants(t, "xdg-pictures;", Env{Home: h, ConfigHome: cfg})
	if !g.Certain() || g.Access(filepath.Join(h, "Bilder", "a")) != ReadWrite || g.Access(filepath.Join(h, "Fotos", "a")) != None {
		t.Errorf("both: grants = %+v, want %s/Bilder", g.grants, h)
	}

	// An unreadable $XDG_CONFIG_HOME file falls through to ~/.config's.
	g = grants(t, "xdg-pictures;", Env{Home: h, ConfigHome: t.TempDir()})
	if !g.Certain() || g.Access(filepath.Join(h, "Fotos", "a")) != ReadWrite {
		t.Errorf("config home without the file: grants = %+v, want %s/Fotos", g.grants, h)
	}

	g = grants(t, "xdg-pictures;", Env{Home: t.TempDir()})
	if g.Certain() {
		t.Errorf("no user-dirs.dirs: grants = %+v, want uncertain", g.grants)
	}
}

func TestHostXDGDirectories(t *testing.T) {
	env := home
	env.HostDataHome, env.HostConfigHome, env.HostCacheHome, env.HostStateHome = "/data/d", "/data/c", "/data/k", "/data/s"
	g := grants(t, "xdg-data/a;xdg-config/b;xdg-cache/c;xdg-state/d;", env)
	for _, p := range []string{"/data/d/a/x", "/data/c/b/x", "/data/k/c/x", "/data/s/d/x"} {
		if g.Access(p) != ReadWrite {
			t.Errorf("Access(%s) = none, want the host's directory", p)
		}
	}
	if !g.Certain() || g.Access("/home/ana/.local/share/a") != None || g.Access("/home/ana/.config/b") != None {
		t.Errorf("grants = %+v: the defaults under home must not stand in when the host's are set", g.grants)
	}
}

func TestLoad(t *testing.T) {
	file := filepath.Join(t.TempDir(), "flatpak-info")
	if err := os.WriteFile(file, []byte(info("~/Notes:ro;")), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(file, home)
	if err != nil || g.Access("/home/ana/Notes") != ReadOnly {
		t.Fatalf("Load = %+v, %v", g, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "none"), home); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load of a missing file = %v, want not exist", err)
	}
}

func pathError(errno syscall.Errno) error {
	return &fs.PathError{Op: "mkdir", Path: "/x", Err: errno}
}

func TestExplain(t *testing.T) {
	const plainRO = "Frostnotes cannot write to"
	for _, tc := range []struct {
		name        string
		filesystems string
		path        string
		err         error
		want        string // the message's start; "" for the error as it was
	}{
		{"no such file, not covered", "~/Notes;", "/home/ana/Other", pathError(syscall.ENOENT), "Frostnotes cannot reach /home/ana/Other because"},
		{"permission denied, not covered", "~/Notes;", "/srv/notes", pathError(syscall.EACCES), "Frostnotes cannot reach /srv/notes because"},
		{"not permitted, not covered", "", "/srv/notes", pathError(syscall.EPERM), "Frostnotes cannot reach /srv/notes because"},
		{"read-only, not covered", "", "/srv/notes", pathError(syscall.EROFS), "Frostnotes cannot reach /srv/notes because"},
		{"read-only, covered read only", "home:ro;", "/home/ana/Notes", pathError(syscall.EROFS), plainRO},
		{"no such file, covered read only", "home:ro;", "/home/ana/Notes", pathError(syscall.ENOENT), ""},
		{"read-only, covered read and write", "home;", "/home/ana/Notes", pathError(syscall.EROFS), ""},
		{"no such file, covered", "home;", "/home/ana/Notes", pathError(syscall.ENOENT), ""},
		{"permission denied, covered", "home;", "/home/ana/Notes", pathError(syscall.EACCES), ""},
		{"not permitted, covered", "home;", "/home/ana/Notes", pathError(syscall.EPERM), ""},
		{"another error, not covered", "", "/srv/notes", pathError(syscall.EIO), ""},
		{"a text error, not covered", "", "/srv/notes", errors.New("not a directory"), ""},
		{"a wrapped error", "", "/srv/notes", errors.Join(errors.New("walk"), pathError(syscall.ENOENT)), "Frostnotes cannot reach"},
		{"an fs.ErrNotExist error", "", "/srv/notes", fs.ErrNotExist, "Frostnotes cannot reach"},
		{"a relative path", "", "notes", pathError(syscall.ENOENT), ""},
		{"no error", "", "/srv/notes", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := grants(t, tc.filesystems, home).Explain(tc.path, tc.err)
			if tc.want == "" {
				if got != tc.err {
					t.Errorf("Explain = %v, want the error as it was (%v)", got, tc.err)
				}
				return
			}
			var e *Error
			if !errors.As(got, &e) || !strings.HasPrefix(got.Error(), tc.want) {
				t.Fatalf("Explain = %v, want %q...", got, tc.want)
			}
			if tc.err != nil && !errors.Is(got, tc.err) {
				t.Errorf("Explain = %v does not wrap %v", got, tc.err)
			}
		})
	}
	var none *Grants
	if err := pathError(syscall.ENOENT); none.Explain("/srv/notes", err) != err {
		t.Error("a nil Grants must keep the error")
	}
}

func TestErrorNamesTheFix(t *testing.T) {
	g := grants(t, "~/Notes;", home)
	got := g.Explain("/srv/my notes", pathError(syscall.ENOENT)).Error()
	want := "Frostnotes cannot reach /srv/my notes because the Flatpak's file access does not include it; " +
		"grant it with: flatpak override --user --filesystem='/srv/my notes' org.frostyard.Frostnotes, " +
		"then log out and in (notesd keeps its sandbox until then)."
	if got != want {
		t.Errorf("message =\n%s\nwant\n%s", got, want)
	}
	ro := grants(t, "~/Notes:ro;", home).Explain("/home/ana/Notes", pathError(syscall.EROFS)).Error()
	wantRO := "Frostnotes cannot write to /home/ana/Notes because the Flatpak's file access to it is read-only; " +
		"grant it with: flatpak override --user --filesystem=/home/ana/Notes org.frostyard.Frostnotes, " +
		"then log out and in (notesd keeps its sandbox until then)."
	if ro != wantRO {
		t.Errorf("message =\n%s\nwant\n%s", ro, wantRO)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/srv/notes":     "/srv/notes",
		"/srv/my notes":  "'/srv/my notes'",
		"/srv/it's":      `'/srv/it'\''s'`,
		"/srv/$HOME":     "'/srv/$HOME'",
		"":               "''",
		"/srv/a;b":       "'/srv/a;b'",
		"~/Notes-1_2.md": "~/Notes-1_2.md",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitList(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
		ok   bool
	}{
		{"", nil, true},
		{"home;", []string{"home"}, true},
		{"home;~/Notes:ro;", []string{"home", "~/Notes:ro"}, true},
		{"home;~/Notes", []string{"home", "~/Notes"}, true},
		{`/srv/a\;b;`, []string{"/srv/a;b"}, true},
		{`/srv/a\sb;`, []string{"/srv/a b"}, true},
		{`\s/srv/x;`, []string{" /srv/x"}, true},
		{`/srv/a\\b;`, []string{`/srv/a\b`}, true},
		{`/srv/a\\;b;`, []string{`/srv/a\`, "b"}, true},
		{`/srv/a\nb\tc\rd;`, []string{"/srv/a\nb\tc\rd"}, true},
		{"a;;b;", []string{"a", "", "b"}, true},
		{`/srv/a\xb;`, nil, false},
		{`/srv/a\;b\`, nil, false},
		{`\`, nil, false},
		{`/srv/a\ b;`, nil, false},
		{`/srv/a\1;`, nil, false},
	} {
		got, ok := splitList(tc.in)
		if ok != tc.ok || strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("splitList(%q) = %q, %t; want %q, %t", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// A value or an entry that cannot be decoded or resolved makes every
// refusal uncertain: Explain keeps the plain error.
func TestExplainSaysNothingWhenUncertain(t *testing.T) {
	for _, tc := range []struct {
		name        string
		filesystems string
		env         Env
	}{
		{"a bad escape", `~/Notes;/srv/a\xb;`, home},
		{"a backslash at the end", `~/Notes;/srv/a\`, home},
		{"an unknown token", "~/Notes;xdg-nothing;", home},
		{"an unknown modifier", "~/Notes;/srv/a:bogus;", home},
		{"a user folder with no user-dirs.dirs", "~/Notes;xdg-documents;", home},
		{"a user folder that the file does not name", "~/Notes;xdg-music;", Env{Home: "/home/ana", ConfigHome: writeUserDirs(t, "XDG_DOCUMENTS_DIR=\"$HOME/D\"\n")}},
		{"a relative home", "~/Notes;", Env{Home: "ana"}},
		{"no home for home", "home;", Env{}},
		{"no runtime directory for xdg-run", "xdg-run/app;", Env{Home: "/home/ana"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := grants(t, tc.filesystems, tc.env)
			if g.Certain() {
				t.Fatalf("filesystems=%s: Certain = true, want false", tc.filesystems)
			}
			err := pathError(syscall.ENOENT)
			if got := g.Explain("/srv/elsewhere/Vault", err); got != err {
				t.Errorf("Explain = %v, want the plain error", got)
			}
		})
	}
	// The same value, understood, explains.
	g := grants(t, "~/Notes;", home)
	if !g.Certain() || g.Explain("/srv/elsewhere/Vault", pathError(syscall.ENOENT)) == nil {
		t.Error("a decoded value must explain")
	}
	var none *Grants
	if none.Certain() {
		t.Error("a nil Grants is not certain")
	}
}

// Flatpak always exposes some paths, so a missing grant does not prove a
// missing access there.
func TestExposed(t *testing.T) {
	g := grants(t, "", home)
	for p, want := range map[string]bool{
		"/app/bin":                     true,
		"/usr/lib/x":                   true,
		"/etc/hosts":                   true,
		"/tmp/vault":                   true,
		"/tmp":                         true,
		"/var/tmp/x":                   true,
		"/run/foo/x":                   true,
		"/proc/1/status":               true,
		"/sys/class":                   true,
		"/dev/null":                    true,
		"/run/user/1000":               true,
		"/run/user/1000/app/x":         true,
		"/run/user/1000/doc/abc/Notes": true,
		"/home/ana/.var/app/org.frostyard.Frostnotes":   true,
		"/home/ana/.var/app/org.frostyard.Frostnotes/x": true,
		"/run/media/ana/disk":                           false,
		"/run/media":                                    false,
		"/home/ana/Notes":                               false,
		"/home/ana/.var/app/org.example.Other/x":        false,
		"/home/ana/.var":                                false,
		"/srv/notes":                                    false,
		"/var/home/ana":                                 false,
		"/var/lib/x":                                    false,
		"/usrlocal":                                     false,
		"/tmpx":                                         false,
		"relative":                                      false,
	} {
		if got := g.Exposed(p); got != want {
			t.Errorf("Exposed(%s) = %t, want %t", p, got, want)
		}
	}
	var none *Grants
	if none.Exposed("/tmp") {
		t.Error("a nil Grants exposes nothing")
	}
}

func TestExplainKeepsTheErrorOfAnExposedPath(t *testing.T) {
	g := grants(t, "~/Notes;", home)
	for _, p := range []string{"/tmp/Vault", "/usr/x", "/run/user/1000/doc/a/Notes", "/home/ana/.var/app/org.frostyard.Frostnotes/Vault"} {
		for _, errno := range []syscall.Errno{syscall.ENOENT, syscall.EACCES, syscall.EPERM, syscall.EROFS} {
			err := pathError(errno)
			if got := g.Explain(p, err); got != err {
				t.Errorf("Explain(%s, %v) = %v, want the plain error", p, errno, got)
			}
		}
	}
	// /run/media is not exposed, so a refusal there is explained.
	if got := g.Explain("/run/media/ana/disk/Vault", pathError(syscall.ENOENT)); got == nil || !strings.Contains(got.Error(), "Frostnotes cannot reach") {
		t.Errorf("Explain(/run/media/...) = %v, want an explanation", got)
	}
}
