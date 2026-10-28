// Package sandbox reads a Flatpak sandbox's file grants and explains a file
// error that they cause (docs/design/app.md#the-flatpak). A vault outside
// the grants fails with a plain "no such file or directory" or "permission
// denied", which does not tell a person that a narrowed --filesystem is the
// cause. Grants reads the [Context] filesystems of /.flatpak-info and says
// how far a path is covered; Explain turns a file error on an uncovered
// path into one that names the fix.
package sandbox

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"
)

// InfoPath is the file that a Flatpak sandbox holds at its root.
const InfoPath = "/.flatpak-info"

// AppID is the Frostnotes Flatpak's application ID, as internal/config.AppID.
const AppID = "org.frostyard.Frostnotes"

// Mode is how far the grants let the app reach a path.
type Mode int

// The modes: no access, read only, and read and write.
const (
	None Mode = iota
	ReadOnly
	ReadWrite
)

// Env is what the grants depend on besides /.flatpak-info.
type Env struct {
	// Home is the home directory, what the grant "home" and "~" name.
	Home string
	// RuntimeDir is $XDG_RUNTIME_DIR, what "xdg-run" names.
	RuntimeDir string
	// UserDirs is the file that names the user's folders; Home's
	// .config/user-dirs.dirs when empty. When it cannot be read, the XDG
	// defaults (Documents, Downloads, ...) stand in.
	UserDirs string
}

// EnvFromOS returns the Env of this process.
func EnvFromOS() Env {
	return Env{Home: os.Getenv("HOME"), RuntimeDir: os.Getenv("XDG_RUNTIME_DIR")}
}

// grant is one filesystem entry: root and the mode below it.
type grant struct {
	root string
	mode Mode
}

// Grants are a sandbox's file grants. The zero value, and nil, grant
// nothing; Explain on nil leaves an error as it is, which is how code
// outside a Flatpak asks.
type Grants struct {
	app    string
	grants []grant
}

// Load reads the file grants of the sandbox described by the file at
// infoPath. An error that matches fs.ErrNotExist means this is no Flatpak.
func Load(infoPath string, env Env) (*Grants, error) {
	f, err := os.Open(infoPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return Parse(f, env)
}

// Parse reads the [Context] filesystems of a /.flatpak-info from r.
func Parse(r io.Reader, env Env) (*Grants, error) {
	g := &Grants{app: AppID}
	var section string
	var entries []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = line[1 : len(line)-1]
		default:
			key, value, ok := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			switch {
			case !ok:
			case section == "Application" && key == "name" && strings.TrimSpace(value) != "":
				g.app = strings.TrimSpace(value)
			case section == "Context" && key == "filesystems":
				entries = strings.Split(value, ";") // the last one wins, as in a key file
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read the sandbox's file grants: %w", err)
	}
	dirs := loadUserDirs(env)
	for _, e := range entries {
		if root, mode, ok := entry(strings.TrimSpace(e), env, dirs); ok {
			g.grants = append(g.grants, grant{root, mode})
		}
	}
	return g, nil
}

// App is the sandbox's application ID.
func (g *Grants) App() string {
	if g == nil || g.app == "" {
		return AppID
	}
	return g.app
}

// Access says how far the grants cover p, an absolute path: the entry
// with the deepest root that holds p decides, a later entry beating an
// earlier one of the same root, so "!~/Secret" under "home" leaves
// ~/Secret out and "~/Notes:rw" under "home:ro" opens it.
func (g *Grants) Access(p string) Mode {
	if g == nil || !filepath.IsAbs(p) {
		return None
	}
	p = filepath.Clean(p)
	best, depth := None, -1
	for _, gr := range g.grants {
		if !under(p, gr.root) {
			continue
		}
		d := 0
		if gr.root != "/" {
			d = strings.Count(gr.root, "/")
		}
		if d >= depth {
			best, depth = gr.mode, d
		}
	}
	return best
}

func under(p, root string) bool {
	return p == root || root == "/" || strings.HasPrefix(p, root+"/")
}

// entry parses one filesystems entry into the path it names and the mode
// it grants; a negation has mode None. It is not ok for an entry that
// names no user path (host-os, an unknown token).
func entry(s string, env Env, dirs map[string]string) (string, Mode, bool) {
	deny := strings.HasPrefix(s, "!")
	s = strings.TrimPrefix(s, "!")
	mode := ReadWrite
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		switch s[i+1:] {
		case "ro":
			mode = ReadOnly
		case "rw", "create":
			mode = ReadWrite
		default:
			return "", None, false
		}
		s = s[:i]
	}
	root, ok := resolve(s, env, dirs)
	if !ok {
		return "", None, false
	}
	if deny {
		mode = None
	}
	return root, mode, true
}

// resolve returns the absolute path that a filesystems token names.
func resolve(s string, env Env, dirs map[string]string) (string, bool) {
	token, sub, _ := strings.Cut(s, "/")
	switch {
	case s == "host":
		return "/", true
	case s == "home":
		return abs(env.Home, "")
	case token == "~":
		return abs(env.Home, sub)
	case strings.HasPrefix(s, "/"):
		return abs(s, "")
	case token == "xdg-run":
		return abs(env.RuntimeDir, sub)
	case token == "xdg-data":
		return abs(path.Join(env.Home, ".local/share"), sub)
	case token == "xdg-config":
		return abs(path.Join(env.Home, ".config"), sub)
	case token == "xdg-cache":
		return abs(path.Join(env.Home, ".cache"), sub)
	}
	if name, ok := strings.CutPrefix(token, "xdg-"); ok {
		if dir, ok := dirs[name]; ok {
			return abs(dir, sub)
		}
	}
	return "", false
}

func abs(base, sub string) (string, bool) {
	if !filepath.IsAbs(base) {
		return "", false
	}
	return path.Join(base, sub), true
}

// userDirKeys maps the XDG token to the user-dirs.dirs key and the default.
var userDirKeys = []struct{ name, key, fallback string }{
	{"desktop", "XDG_DESKTOP_DIR", "Desktop"},
	{"documents", "XDG_DOCUMENTS_DIR", "Documents"},
	{"download", "XDG_DOWNLOAD_DIR", "Downloads"},
	{"music", "XDG_MUSIC_DIR", "Music"},
	{"pictures", "XDG_PICTURES_DIR", "Pictures"},
	{"public-share", "XDG_PUBLICSHARE_DIR", "Public"},
	{"templates", "XDG_TEMPLATES_DIR", "Templates"},
	{"videos", "XDG_VIDEOS_DIR", "Videos"},
}

// loadUserDirs resolves the user's folders: what user-dirs.dirs names
// when it can be read, else the XDG defaults under the home directory.
func loadUserDirs(env Env) map[string]string {
	dirs := map[string]string{}
	if env.Home == "" {
		return dirs
	}
	file := env.UserDirs
	if file == "" {
		file = filepath.Join(env.Home, ".config", "user-dirs.dirs")
	}
	named := map[string]string{}
	if b, err := os.ReadFile(file); err == nil {
		for line := range strings.SplitSeq(string(b), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok || strings.HasPrefix(key, "#") {
				continue
			}
			value = strings.Trim(strings.TrimSpace(value), `"`)
			if rest, ok := strings.CutPrefix(value, "$HOME"); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
				value = env.Home + rest
			}
			if filepath.IsAbs(value) {
				named[key] = path.Clean(value)
			}
		}
	}
	for _, k := range userDirKeys {
		if v, ok := named[k.key]; ok {
			dirs[k.name] = v
		} else {
			dirs[k.name] = path.Join(env.Home, k.fallback)
		}
	}
	return dirs
}

// Explain returns err as it is, unless it is a file error that the grants
// explain for p: no such file, permission denied or not permitted on a path
// that the grants do not cover, or a read-only file system on one that
// they cover at most read-only. It then returns an *Error that says what
// to grant. A path that the grants cover keeps its plain error, and so
// does a nil Grants (not in a Flatpak).
func (g *Grants) Explain(p string, err error) error {
	if g == nil || err == nil || !filepath.IsAbs(p) {
		return err
	}
	access := g.Access(p)
	switch {
	case errors.Is(err, syscall.EROFS):
		if access == ReadWrite {
			return err
		}
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		if access != None {
			return err
		}
	default:
		return err
	}
	return &Error{Path: p, ReadOnly: access == ReadOnly, App: g.App(), Cause: err}
}

// Error says that the sandbox's file access keeps the app from a path.
type Error struct {
	Path     string
	ReadOnly bool // the grants cover Path, but read only
	App      string
	Cause    error
}

func (e *Error) Error() string {
	grant := fmt.Sprintf("flatpak override --user --filesystem=%s %s", Quote(e.Path), e.App)
	if e.ReadOnly {
		return fmt.Sprintf("Frostnotes cannot write to %s because the Flatpak's file access to it is read-only; "+
			"grant it with: %s, then log out and in (notesd keeps its sandbox until then).", e.Path, grant)
	}
	return fmt.Sprintf("Frostnotes cannot reach %s because the Flatpak's file access does not include it; "+
		"grant it with: %s, then log out and in (notesd keeps its sandbox until then).", e.Path, grant)
}

// Unwrap returns the file error that this explains.
func (e *Error) Unwrap() error { return e.Cause }

// Quote returns s for a POSIX shell: as it is when it holds only plain
// characters, else in single quotes.
func Quote(s string) string {
	plain := func(r rune) bool {
		return unicode.IsLetter(r) && r < unicode.MaxASCII || unicode.IsDigit(r) && r < unicode.MaxASCII ||
			strings.ContainsRune("/._-+=@%,~", r)
	}
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return !plain(r) }) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
