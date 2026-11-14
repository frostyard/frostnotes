package blobs

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// PutWant's failures, as Put's are in TestPutFailuresLeaveNothing: each
// returns its error and leaves no file, a temporary one included.
func TestPutWantFailuresLeaveNothing(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	ctx := t.Context()
	good := ID([]byte("hello"))

	err := s.PutWant(ctx, &failingReader{n: 3}, good)
	if err == nil || !strings.HasPrefix(err.Error(), "blobs: read content: ") || !strings.Contains(err.Error(), "disk went away") {
		t.Fatalf("PutWant with a failing reader = %v", err)
	}

	err = s.PutWant(ctx, strings.NewReader("forged"), good)
	want := "blobs: content of " + good + " hashes to " + ID([]byte("forged")) + ": " + ErrCorrupt.Error()
	if !errors.Is(err, ErrCorrupt) || err.Error() != want {
		t.Fatalf("PutWant of the wrong content = %v, want %q", err, want)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.PutWant(canceled, strings.NewReader("hello"), good); !errors.Is(err, context.Canceled) {
		t.Fatalf("PutWant with a canceled context = %v, want context.Canceled", err)
	}
	// The content is checked before the context: wrong content reports
	// ErrCorrupt even when the context is done.
	if err := s.PutWant(canceled, strings.NewReader("forged"), good); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("PutWant of the wrong content with a canceled context = %v, want ErrCorrupt", err)
	}

	if got := files(t, dir); len(got) != 0 {
		t.Fatalf("failed PutWants left %v", got)
	}
}

// A malformed want is refused before the store's directory is made.
func TestPutWantInvalidIDTouchesNothing(t *testing.T) {
	dir := t.TempDir() + "/store"
	s := New(dir)
	if err := s.PutWant(t.Context(), &failingReader{n: 3}, "nope"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("PutWant with a malformed ID = %v, want ErrInvalidID", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("store dir after a refused PutWant: %v, want it not made", err)
	}
}

// Put and PutWant make the same blob: the same ID, bytes and mode.
func TestPutAndPutWantStoreTheSameBlob(t *testing.T) {
	a, b := New(t.TempDir()), New(t.TempDir())
	ctx := t.Context()
	id, err := a.Put(ctx, strings.NewReader("same bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.PutWant(ctx, strings.NewReader("same bytes"), id); err != nil {
		t.Fatal(err)
	}
	pa, _ := a.path(id)
	pb, _ := b.path(id)
	for _, p := range []string{pa, pb} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %v, want 0600", p, fi.Mode().Perm())
		}
		got, err := os.ReadFile(p)
		if err != nil || string(got) != "same bytes" {
			t.Errorf("%s holds %q, %v", p, got, err)
		}
	}
}
