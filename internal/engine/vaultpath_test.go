package engine

import (
	"errors"
	"io/fs"
	"testing"
	"time"
)

// fakeInfo is a file information for a path that is only a directory.
type fakeInfo struct{ fs.FileInfo }

func (fakeInfo) IsDir() bool { return true }

func (fakeInfo) Name() string { return "home" }

func (fakeInfo) Size() int64 { return 0 }

func (fakeInfo) Mode() fs.FileMode { return fs.ModeDir | 0o755 }

func (fakeInfo) ModTime() time.Time { return time.Time{} }

func (fakeInfo) Sys() any { return nil }

// stat answers the paths of a file system a test describes: a path in
// missing is ErrNotExist, a path in dirs is a directory.
func stat(missing, dirs []string) func(string) (fs.FileInfo, error) {
	return func(p string) (fs.FileInfo, error) {
		for _, m := range missing {
			if m == p {
				return nil, &fs.PathError{Op: "lstat", Path: p, Err: fs.ErrNotExist}
			}
		}
		for _, d := range dirs {
			if d == p {
				return fakeInfo{}, nil
			}
		}
		return nil, &fs.PathError{Op: "lstat", Path: p, Err: errors.New("not a directory")}
	}
}

// A sandbox with /var/home but no /home, as Snow's Flatpak, finds the home
// directories where the system keeps them; elsewhere the path is unchanged.
func TestSandboxHome(t *testing.T) {
	sandbox := stat([]string{"/home"}, []string{"/var/home"})
	snow := stat(nil, []string{"/home", "/var/home"})
	noVarHome := stat([]string{"/home"}, nil)
	for _, tc := range []struct {
		name  string
		path  string
		lstat func(string) (fs.FileInfo, error)
		want  string
	}{
		{"a path under /home, sandboxed", "/home/ana/Notes", sandbox, "/var/home/ana/Notes"},
		{"/home itself, sandboxed", "/home", sandbox, "/var/home"},
		{"a path under /home, with one", "/home/ana/Notes", snow, "/home/ana/Notes"},
		{"a path elsewhere", "/srv/Notes", sandbox, "/srv/Notes"},
		{"a lookalike path", "/homeward/Notes", sandbox, "/homeward/Notes"},
		{"no /var/home", "/home/ana/Notes", noVarHome, "/home/ana/Notes"},
	} {
		if got := sandboxHome(tc.path, tc.lstat); got != tc.want {
			t.Errorf("%s: sandboxHome(%q) = %q, want %q", tc.name, tc.path, got, tc.want)
		}
	}
}
