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

var home = Env{Home: "/home/ana", RuntimeDir: "/run/user/1000", UserDirs: "/nonexistent/user-dirs.dirs"}

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
		{"spaces around entries", " home ; !~/Secret ;", "/home/ana/Secret", None},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := grants(t, tc.filesystems, home).Access(tc.path); got != tc.want {
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
	file := filepath.Join(t.TempDir(), "user-dirs.dirs")
	text := "# comment\nXDG_DOCUMENTS_DIR=\"$HOME/Papers\"\nXDG_DOWNLOAD_DIR=\"/data/dl\"\nXDG_MUSIC_DIR=\"relative\"\nXDG_VIDEOS_DIR=\"$HOMEBREW/v\"\n"
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	env := home
	env.UserDirs = file
	for _, tc := range []struct {
		path string
		want Mode
	}{
		{"/home/ana/Papers/Notes", ReadWrite}, // renamed
		{"/home/ana/Documents/Notes", None},   // no longer there
		{"/data/dl/x", ReadWrite},             // absolute
		{"/home/ana/Music/x", ReadWrite},      // a relative value: the default
		{"/home/ana/Videos/x", ReadWrite},     // not $HOME exactly: the default
	} {
		g := grants(t, "xdg-documents;xdg-download;xdg-music;xdg-videos;", env)
		if got := g.Access(tc.path); got != tc.want {
			t.Errorf("Access(%s) = %d, want %d", tc.path, got, tc.want)
		}
	}
}

func TestUserDirsFromTheHome(t *testing.T) {
	h := t.TempDir()
	if err := os.MkdirAll(filepath.Join(h, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h, ".config", "user-dirs.dirs"), []byte("XDG_PICTURES_DIR=\"$HOME/Fotos\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := grants(t, "xdg-pictures;", Env{Home: h})
	if g.Access(filepath.Join(h, "Fotos", "a")) != ReadWrite || g.Access(filepath.Join(h, "Pictures", "a")) != None {
		t.Errorf("grants = %+v, want %s/Fotos", g.grants, h)
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
