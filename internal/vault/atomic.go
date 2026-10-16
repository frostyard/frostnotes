package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// Temporary files of atomic writes are dot-files, so walks and the watcher
// skip them; a scan of a vault removes any a crash left behind.
const (
	tempPrefix = ".frostnotes-"
	tempSuffix = ".tmp"
)

// isTemp reports whether name, a file name, is an atomic write's temporary
// file.
func isTemp(name string) bool {
	return strings.HasPrefix(name, tempPrefix) && strings.HasSuffix(name, tempSuffix) &&
		len(name) == len(tempPrefix)+16+len(tempSuffix)
}

func tempName() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails
	return tempPrefix + hex.EncodeToString(b[:]) + tempSuffix
}

// staged is new content written to a temporary file beside its path, not
// yet renamed over it.
type staged struct {
	r    *os.Root
	tmp  string
	path string
	info fs.FileInfo // the temporary file's stat once synced: the file's after the rename
}

// stage writes data to a temporary dot-file in p's folder and fsyncs it.
// perm is the mode a new file gets before the umask; an existing file's
// mode is kept.
func stage(r *os.Root, p string, data []byte, existing fs.FileInfo) (*staged, error) {
	tmp := path.Join(path.Dir(p), tempName())
	perm := fs.FileMode(0o666)
	if existing != nil {
		perm = existing.Mode().Perm()
	}
	f, err := r.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return nil, fmt.Errorf("write %s: %w", p, err)
	}
	s := &staged{r: r, tmp: tmp, path: p}
	err = func() error {
		if existing != nil {
			if err := f.Chmod(perm); err != nil {
				return err
			}
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		info, err := f.Stat()
		s.info = info
		return err
	}()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		s.discard()
		return nil, fmt.Errorf("write %s: %w", p, err)
	}
	return s, nil
}

// commit renames the temporary file over its path and fsyncs the folder,
// so the new content survives a crash once commit returns.
func (s *staged) commit() error {
	if err := s.r.Rename(s.tmp, s.path); err != nil {
		s.discard()
		return fmt.Errorf("write %s: %w", s.path, err)
	}
	return syncDir(s.r, path.Dir(s.path))
}

// discard removes the temporary file.
func (s *staged) discard() { _ = s.r.Remove(s.tmp) }

// remove deletes p and fsyncs its folder.
func remove(r *os.Root, p string) error {
	if err := r.Remove(p); err != nil {
		return fmt.Errorf("delete %s: %w", p, err)
	}
	return syncDir(r, path.Dir(p))
}

// rename moves from to to, which must not exist, and fsyncs both folders.
func rename(r *os.Root, from, to string) error {
	if _, err := r.Lstat(to); err == nil {
		return fmt.Errorf("move %s: %s %w", from, to, fs.ErrExist)
	}
	if err := r.Rename(from, to); err != nil {
		return fmt.Errorf("move %s: %w", from, err)
	}
	if err := syncDir(r, path.Dir(to)); err != nil {
		return err
	}
	if path.Dir(from) != path.Dir(to) {
		return syncDir(r, path.Dir(from))
	}
	return nil
}

func syncDir(r *os.Root, dir string) error {
	d, err := r.Open(dir)
	if err != nil {
		return fmt.Errorf("sync folder %s: %w", dir, err)
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("sync folder %s: %w", dir, err)
	}
	return nil
}

// bigFile is the size above which a file's content is streamed rather than
// held in memory, and a note is indexed by name only
// (docs/design/vault.md#operational-notes).
const bigFile = 10 << 20

// content is a file's bytes as read once: its stat and hash, and the bytes
// themselves unless the file is big.
type content struct {
	info fs.FileInfo
	hash string
	data []byte // nil for a big file, whose bytes are read again to store them
}

func (c content) size() int64        { return c.info.Size() }
func (c content) modTime() time.Time { return c.info.ModTime() }
func (c content) sameStat(fi fs.FileInfo) bool {
	return fi != nil && fi.Size() == c.info.Size() && fi.ModTime().Equal(c.info.ModTime())
}

// errUnstable means a file kept changing while it was read.
var errUnstable = errors.New("the file kept changing while it was read")

// read reads p under r with checkFile's rules. It stats the open file
// before and after reading and retries when they differ, so the bytes and
// stat agree even while another program writes the file in place.
func read(r *os.Root, p string) (content, error) {
	for range 5 {
		if _, err := checkFile(r, p); err != nil {
			return content{}, err
		}
		c, err := readOnce(r, p)
		if !errors.Is(err, errUnstable) {
			return c, err
		}
	}
	return content{}, fmt.Errorf("read %s: %w", p, errUnstable)
}

func readOnce(r *os.Root, p string) (content, error) {
	f, err := r.Open(p)
	if err != nil {
		return content{}, err
	}
	defer func() { _ = f.Close() }()
	before, err := f.Stat()
	if err != nil {
		return content{}, err
	}
	var c content
	if before.Size() > bigFile {
		c.hash, err = blobs.IDReader(f)
	} else {
		var buf bytes.Buffer
		_, err = io.Copy(&buf, f)
		c.data = buf.Bytes()
		c.hash = blobs.ID(c.data)
	}
	if err != nil {
		return content{}, fmt.Errorf("read %s: %w", p, err)
	}
	after, err := f.Stat()
	if err != nil {
		return content{}, err
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) ||
		(c.data != nil && int64(len(c.data)) != after.Size()) {
		return content{}, errUnstable
	}
	c.info = after
	return c, nil
}
