// Package sandbox reads a Flatpak sandbox's file grants and explains a file
// error that they cause (docs/design/app.md#the-flatpak). A vault outside
// the grants fails with a plain "no such file or directory" or "permission
// denied", which does not tell a person that a narrowed --filesystem is the
// cause. Grants reads the [Context] filesystems of /.flatpak-info and says
// how far a path is covered; Explain turns a file error on an uncovered
// path into one that names the fix, and only when it is sure: a grant it
// cannot decode or resolve, or a path that Flatpak always exposes, leaves
// the plain error.
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
	// ConfigHome is the sandbox's $XDG_CONFIG_HOME (in a Flatpak,
	// ~/.var/app/<ID>/config), where Flatpak puts a copy of the host's
	// user-dirs.dirs; it is read before Home's .config/user-dirs.dirs.
	ConfigHome string
	// HostDataHome, HostConfigHome, HostCacheHome and HostStateHome are the
	// host's $XDG_*_HOME, which Flatpak passes on as HOST_XDG_*_HOME when
	// the host sets them; "xdg-data" and its kin name them, and the XDG
	// defaults under Home when they are empty.
	HostDataHome, HostConfigHome, HostCacheHome, HostStateHome string
}

// EnvFromOS returns the Env of this process.
func EnvFromOS() Env {
	return Env{
		Home:           os.Getenv("HOME"),
		RuntimeDir:     os.Getenv("XDG_RUNTIME_DIR"),
		ConfigHome:     os.Getenv("XDG_CONFIG_HOME"),
		HostDataHome:   os.Getenv("HOST_XDG_DATA_HOME"),
		HostConfigHome: os.Getenv("HOST_XDG_CONFIG_HOME"),
		HostCacheHome:  os.Getenv("HOST_XDG_CACHE_HOME"),
		HostStateHome:  os.Getenv("HOST_XDG_STATE_HOME"),
	}
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
	// always are the paths Flatpak exposes whatever the grants say.
	always []string
	// uncertain is set when a grant could not be decoded or resolved: what
	// the grants cover is then unknown, so nothing can be said to be refused.
	uncertain bool
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

// Parse reads the [Context] filesystems of a /.flatpak-info from r. A
// value it cannot decode, or an entry it cannot understand or resolve,
// makes the grants uncertain (Certain).
func Parse(r io.Reader, env Env) (*Grants, error) {
	g := &Grants{app: AppID}
	var section, raw string
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
				raw = strings.TrimLeft(value, " \t") // the last one wins, as in a key file
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read the sandbox's file grants: %w", err)
	}
	g.always = alwaysExposed(g.app, env)
	entries, ok := splitList(raw)
	if !ok {
		g.uncertain = true
		return g, nil
	}
	dirs := loadUserDirs(env)
	for _, e := range entries {
		if e == "" {
			continue // "a;;b", and the end of "a;b;"
		}
		root, mode, res := entry(e, env, dirs)
		switch res {
		case resolved:
			g.grants = append(g.grants, grant{root, mode})
		case unknown:
			g.uncertain = true
		}
	}
	return g, nil
}

// splitList decodes a GLib key file's string list: entries end at a ";"
// that no backslash escapes, and an entry decodes \; to ";", \\ to a
// backslash and \s, \n, \t and \r to a space, a line feed, a tab and a
// carriage return. It is not ok for any other escape, or a
// backslash at the end: the value cannot then be trusted.
func splitList(raw string) ([]string, bool) {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' {
			if c == ';' {
				out = append(out, cur.String())
				cur.Reset()
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		i++
		if i == len(raw) {
			return nil, false
		}
		switch raw[i] {
		case ';':
			cur.WriteByte(';')
		case '\\':
			cur.WriteByte('\\')
		case 's':
			cur.WriteByte(' ')
		case 'n':
			cur.WriteByte('\n')
		case 't':
			cur.WriteByte('\t')
		case 'r':
			cur.WriteByte('\r')
		default:
			return nil, false
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String()) // an unterminated last entry
	}
	return out, true
}

// Certain says whether every grant was decoded and resolved. Explain says
// nothing when it is not: coverage cannot be disproved.
func (g *Grants) Certain() bool { return g != nil && !g.uncertain }

// alwaysExposed lists the paths that a Flatpak sandbox has whatever its
// filesystems say: the app's own directory under the home, its runtime,
// the temporary and system directories, and the runtime directory (with
// the document portal's mount).
func alwaysExposed(app string, env Env) []string {
	exposed := []string{"/app", "/usr", "/etc", "/tmp", "/var/tmp", "/run", "/proc", "/sys", "/dev"}
	if filepath.IsAbs(env.Home) {
		exposed = append(exposed, path.Join(env.Home, ".var", "app", app))
	}
	if filepath.IsAbs(env.RuntimeDir) {
		exposed = append(exposed, path.Clean(env.RuntimeDir), path.Join(env.RuntimeDir, "doc"))
	}
	return exposed
}

// Exposed says whether Flatpak gives the sandbox p whatever the grants
// say. Under /run only /run/media is left out.
func (g *Grants) Exposed(p string) bool {
	if g == nil || !filepath.IsAbs(p) {
		return false
	}
	p = filepath.Clean(p)
	if under(p, "/run/media") {
		return false
	}
	for _, root := range g.always {
		if under(p, root) {
			return true
		}
	}
	return false
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

// result says what an entry of the filesystems list came to.
type result int

const (
	resolved result = iota // it names a path
	ignored                // it names no user path (host-os, host-etc)
	unknown                // it cannot be understood or resolved
)

// entry parses one filesystems entry into the path it names and the mode
// it grants; a negation has mode None.
func entry(s string, env Env, dirs map[string]string) (string, Mode, result) {
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
			return "", None, unknown
		}
		s = s[:i]
	}
	root, res := resolve(s, env, dirs)
	if res != resolved {
		return "", None, res
	}
	if deny {
		mode = None
	}
	return root, mode, resolved
}

// resolve returns the absolute path that a filesystems token names.
func resolve(s string, env Env, dirs map[string]string) (string, result) {
	token, sub, _ := strings.Cut(s, "/")
	switch {
	case s == "host":
		return "/", resolved
	case s == "host-os" || s == "host-etc" || s == "host-reset":
		return "", ignored
	case s == "home":
		return abs(env.Home, "")
	case token == "~":
		return abs(env.Home, sub)
	case strings.HasPrefix(s, "/"):
		return abs(s, "")
	case token == "xdg-run":
		return abs(env.RuntimeDir, sub)
	case token == "xdg-data":
		return abs(hostDir(env.HostDataHome, env.Home, ".local/share"), sub)
	case token == "xdg-config":
		return abs(hostDir(env.HostConfigHome, env.Home, ".config"), sub)
	case token == "xdg-cache":
		return abs(hostDir(env.HostCacheHome, env.Home, ".cache"), sub)
	case token == "xdg-state":
		return abs(hostDir(env.HostStateHome, env.Home, ".local/state"), sub)
	}
	if name, ok := strings.CutPrefix(token, "xdg-"); ok {
		if dir, ok := dirs[name]; ok {
			return abs(dir, sub)
		}
	}
	return "", unknown
}

// hostDir is the host's XDG base directory: host when set, else def under home.
func hostDir(host, home, def string) string {
	if host != "" {
		return host
	}
	if !filepath.IsAbs(home) {
		return ""
	}
	return path.Join(home, def)
}

func abs(base, sub string) (string, result) {
	if !filepath.IsAbs(base) {
		return "", unknown
	}
	return path.Join(base, sub), resolved
}

// userDirKeys maps the XDG token to the user-dirs.dirs key.
var userDirKeys = []struct{ name, key string }{
	{"desktop", "XDG_DESKTOP_DIR"},
	{"documents", "XDG_DOCUMENTS_DIR"},
	{"download", "XDG_DOWNLOAD_DIR"},
	{"music", "XDG_MUSIC_DIR"},
	{"pictures", "XDG_PICTURES_DIR"},
	{"public-share", "XDG_PUBLICSHARE_DIR"},
	{"templates", "XDG_TEMPLATES_DIR"},
	{"videos", "XDG_VIDEOS_DIR"},
}

// loadUserDirs reads the user's folders from the first user-dirs.dirs that
// can be read: $XDG_CONFIG_HOME's (in a Flatpak, the copy of the host's
// that Flatpak places there), then ~/.config's. A folder it does not name
// is missing from the map, and so is every folder when no file can be
// read: nothing is guessed.
func loadUserDirs(env Env) map[string]string {
	dirs := map[string]string{}
	var files []string
	if filepath.IsAbs(env.ConfigHome) {
		files = append(files, filepath.Join(env.ConfigHome, "user-dirs.dirs"))
	}
	if filepath.IsAbs(env.Home) {
		files = append(files, filepath.Join(env.Home, ".config", "user-dirs.dirs"))
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		named := map[string]string{}
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
		for _, k := range userDirKeys {
			if v, ok := named[k.key]; ok {
				dirs[k.name] = v
			}
		}
		break
	}
	return dirs
}

// Explain returns err as it is, unless it is a file error that the grants
// explain for p: no such file, permission denied or not permitted on a path
// that the grants do not cover, or a read-only file system on one that
// they cover at most read-only. It then returns an *Error that says what
// to grant. It says nothing, and returns err, when it cannot be sure: a
// nil Grants (not in a Flatpak), grants that are not Certain, and a path
// that Flatpak always exposes (Exposed). A path that the grants cover keeps
// its plain error too.
func (g *Grants) Explain(p string, err error) error {
	if !g.Certain() || err == nil || !filepath.IsAbs(p) || g.Exposed(p) {
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
