package blobs

// CONTRACT TEST for task card T-0005 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func sha(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// files lists every regular file under dir, relative to it.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return out
}

func put(t *testing.T, s *Store, content string) string {
	t.Helper()
	id, err := s.Put(t.Context(), strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPutGetRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "blobs")
	s := New(dir)
	note := []byte("---\ntags: [a]\n---\r\nno final newline")
	id, err := s.Put(t.Context(), bytes.NewReader(note))
	if err != nil {
		t.Fatal(err)
	}
	if id != sha(note) || ID(note) != id {
		t.Fatalf("id = %s, ID = %s; want the SHA-256 %s", id, ID(note), sha(note))
	}
	got, err := s.Get(id)
	if err != nil || !bytes.Equal(got, note) {
		t.Fatalf("Get = %q, %v", got, err)
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || !bytes.Equal(streamed, note) {
		t.Fatalf("Open read %q, %v", streamed, err)
	}
	want := filepath.Join(id[:2], id)
	if got := files(t, dir); len(got) != 1 || got[0] != want {
		t.Fatalf("files = %v, want [%s]", got, want)
	}
	fi, err := os.Stat(filepath.Join(dir, want))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("blob mode = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
	for _, d := range []string{dir, filepath.Join(dir, id[:2])} {
		di, err := os.Stat(d)
		if err != nil || di.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %v, %v; want 0700", d, di.Mode().Perm(), err)
		}
	}
}

func TestEmptyBlob(t *testing.T) {
	s := New(t.TempDir())
	id := put(t, s, "")
	if id != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("id of empty content = %s", id)
	}
	if got, err := s.Get(id); err != nil || len(got) != 0 {
		t.Fatalf("Get = %q, %v", got, err)
	}
}

func TestPutIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	big := bytes.Repeat([]byte("0123456789abcdef"), 1<<16) // 1 MiB
	a, err := s.Put(t.Context(), bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Put(t.Context(), bytes.NewReader(big))
	if err != nil || a != b {
		t.Fatalf("second Put = %s, %v; want %s", b, err, a)
	}
	if got := files(t, dir); len(got) != 1 {
		t.Fatalf("files after two identical Puts = %v", got)
	}
}

func TestConcurrentPuts(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	ids := make([]string, 16)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			id, err := s.Put(context.Background(), strings.NewReader("same bytes"))
			if err != nil {
				t.Error(err)
			}
			ids[i] = id
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != sha([]byte("same bytes")) {
			t.Fatalf("ids = %v", ids)
		}
	}
	if got := files(t, dir); len(got) != 1 {
		t.Fatalf("files after concurrent Puts = %v", got)
	}
}

func TestHasAndRemove(t *testing.T) {
	s := New(t.TempDir())
	id := put(t, s, "x")
	if ok, err := s.Has(id); !ok || err != nil {
		t.Fatalf("Has = %v, %v", ok, err)
	}
	if err := s.Remove(id); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Has(id); ok || err != nil {
		t.Fatalf("Has after Remove = %v, %v", ok, err)
	}
	if err := s.Remove(id); err != nil {
		t.Fatalf("second Remove = %v, want nil", err)
	}
	if _, err := s.Open(id); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open after Remove = %v, want fs.ErrNotExist", err)
	}
	if _, err := s.Get(id); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Get after Remove = %v, want fs.ErrNotExist", err)
	}
}

func TestGetDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	id := put(t, s, "the original")
	if err := os.WriteFile(filepath.Join(dir, id[:2], id), []byte("bit rot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(id); !errors.Is(err, ErrCorrupt) || got != nil {
		t.Fatalf("Get of a changed blob = %q, %v; want nil, ErrCorrupt", got, err)
	}
}

func TestRejectsInvalidIDs(t *testing.T) {
	s := New(t.TempDir())
	good := sha([]byte("x"))
	for _, id := range []string{
		"", "../../etc/passwd", strings.ToUpper(good), good[:63], good + "0",
		strings.Repeat("g", 64), good[:2] + "/" + good[3:],
	} {
		if _, err := s.Open(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Open(%q) = %v, want ErrInvalidID", id, err)
		}
		if _, err := s.Get(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Get(%q) = %v, want ErrInvalidID", id, err)
		}
		if _, err := s.Has(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Has(%q) = %v, want ErrInvalidID", id, err)
		}
		if err := s.Remove(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Remove(%q) = %v, want ErrInvalidID", id, err)
		}
	}
}

// failingReader returns data, then an error.
type failingReader struct{ n int }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.n > 0 {
		f.n--
		return copy(p, "partial"), nil
	}
	return 0, errors.New("disk went away")
}

func TestPutFailuresLeaveNothing(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if _, err := s.Put(t.Context(), &failingReader{n: 3}); err == nil || !strings.Contains(err.Error(), "disk went away") {
		t.Fatalf("Put with a failing reader = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Put(ctx, strings.NewReader("never stored")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put with a canceled context = %v, want context.Canceled", err)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Fatalf("failed Puts left %v", got)
	}
}

func TestListIDs(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if ids, err := s.IDs(t.Context()); err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("IDs of an empty store = %#v, %v; want an empty, non-nil slice", ids, err)
	}
	if ids, err := New(filepath.Join(dir, "missing")).IDs(t.Context()); err != nil || len(ids) != 0 {
		t.Fatalf("IDs of a missing dir = %v, %v", ids, err)
	}
	want := []string{put(t, s, "one"), put(t, s, "two"), put(t, s, "three")}
	slices.Sort(want)
	// Strays IDs must skip: a temporary file, a file that is not an ID, an
	// ID in the wrong shard, and a directory where a blob would be.
	other := sha([]byte("stray"))
	for _, p := range []string{".put-123", filepath.Join(want[0][:2], "notes.txt"), filepath.Join("00", other)} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, other[:2], other), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := s.IDs(t.Context())
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("IDs = %v, %v; want %v", got, err, want)
	}
}

func TestRemoveTemp(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	id := put(t, s, "kept")
	now := time.Now()
	for name, age := range map[string]time.Duration{".put-old": 2 * time.Hour, ".put-new": 0, "README": 2 * time.Hour} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.RemoveTemp(now.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("RemoveTemp = %d, %v; want 1, nil", n, err)
	}
	got := files(t, dir)
	slices.Sort(got)
	want := []string{".put-new", "README", filepath.Join(id[:2], id)}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("files after RemoveTemp = %v, want %v", got, want)
	}
}
