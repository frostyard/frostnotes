package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

// ErrPath is the error CheckPath wraps: a path the API may not name.
var ErrPath = errors.New("not a vault path")

// The longest a path's segment and a whole path may be, in bytes: what
// file systems hold (255 bytes a name, and well within PATH_MAX for the
// vault's own folder too).
const (
	MaxSegment = 255
	MaxPath    = 4000
)

// CheckPath reports whether p may name a file of a vault through the API
// (docs/design/vault.md): vault-relative and slash-separated, valid UTF-8,
// with no empty, "." or ".." segment, no NUL, and no segment starting with
// "." except a first segment of SettingsDir. It is lexical; checkFile
// refuses symlinks on disk.
func CheckPath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("%w: the path is empty", ErrPath)
	case !utf8.ValidString(p) || strings.ContainsRune(p, 0):
		return fmt.Errorf("%w: %q is not valid UTF-8 text", ErrPath, p)
	case strings.HasPrefix(p, "/"):
		return fmt.Errorf("%w: %q is absolute; paths are relative to the vault", ErrPath, p)
	case len(p) > MaxPath:
		return fmt.Errorf("%w: a path of %d bytes is longer than %d", ErrPath, len(p), MaxPath)
	}
	for i, seg := range strings.Split(p, "/") {
		switch {
		case seg == "":
			return fmt.Errorf("%w: %q has an empty segment", ErrPath, p)
		case seg == "." || seg == "..":
			return fmt.Errorf("%w: %q has a %q segment", ErrPath, p, seg)
		case strings.HasPrefix(seg, ".") && (i != 0 || seg != SettingsDir):
			return fmt.Errorf("%w: %q has a hidden segment %q", ErrPath, p, seg)
		case len(seg) > MaxSegment:
			return fmt.Errorf("%w: a name of %d bytes is longer than %d", ErrPath, len(seg), MaxSegment)
		}
	}
	return nil
}

// Beside returns p with suffix put before its extension ("name 1.md",
// "name.sync-7.md"), the name's stem cut short, at a character, when the
// name would be longer than MaxSegment.
func Beside(p, suffix string) string {
	dir, base := path.Split(p)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for len(stem)+len(suffix)+len(ext) > MaxSegment && stem != "" {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	return dir + stem + suffix + ext
}

// IsSettings reports whether p lies in the vault's settings directory.
func IsSettings(p string) bool { return strings.HasPrefix(p, SettingsDir+"/") }

// CheckFolder is CheckPath for a folder, where "", "/" and "." name the
// vault's root. It returns the folder without surrounding slashes.
func CheckFolder(folder string) (string, error) {
	folder = strings.Trim(folder, "/")
	if folder == "" || folder == "." {
		return "", nil
	}
	return folder, CheckPath(folder)
}

// checkFile lstats p's parent folders and p itself under r, refusing a
// symlink anywhere: a vault's files are never reached through one. It
// returns p's FileInfo, or an error matching fs.ErrNotExist when p or a
// parent is missing.
func checkFile(r *os.Root, p string) (fs.FileInfo, error) {
	segs := strings.Split(p, "/")
	for i := 1; i < len(segs); i++ {
		prefix := strings.Join(segs[:i], "/")
		fi, err := r.Lstat(prefix)
		if err != nil {
			return nil, err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %q is a symlink", ErrPath, prefix)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("%w: %q is not a folder", ErrPath, prefix)
		}
	}
	fi, err := r.Lstat(p)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %q is a symlink", ErrPath, p)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrPath, p)
	}
	return fi, nil
}

// mkdirs creates p's missing parent folders under r, refusing to pass
// through a symlink or a file.
func mkdirs(r *os.Root, p string) error {
	dir := path.Dir(p)
	if dir == "." {
		return nil
	}
	segs := strings.Split(dir, "/")
	for i := range segs {
		prefix := strings.Join(segs[:i+1], "/")
		fi, err := r.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			if err := r.Mkdir(prefix, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("create folder %s: %w", prefix, err)
			}
			if err := syncDir(r, path.Dir(prefix)); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !fi.IsDir() || fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %q is not a folder", ErrPath, prefix)
		}
	}
	return nil
}
