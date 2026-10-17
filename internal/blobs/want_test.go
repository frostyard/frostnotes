package blobs

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestPutWantStoresOnlyTheNamedContent(t *testing.T) {
	s := New(t.TempDir())
	ctx := t.Context()
	good := ID([]byte("hello"))
	if err := s.PutWant(ctx, strings.NewReader("hello"), good); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Size(good); err != nil || n != 5 {
		t.Fatalf("Size = %d, %v; want 5", n, err)
	}
	other := ID([]byte("other"))
	err := s.PutWant(ctx, strings.NewReader("forged"), other)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("PutWant of the wrong content = %v, want ErrCorrupt", err)
	}
	for _, id := range []string{other, ID([]byte("forged"))} {
		if ok, _ := s.Has(id); ok {
			t.Fatalf("a refused PutWant stored %s", id)
		}
	}
	if _, err := s.Size(other); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Size of a missing blob = %v, want fs.ErrNotExist", err)
	}
	if err := s.PutWant(ctx, strings.NewReader("x"), "nope"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("PutWant with a malformed ID = %v, want ErrInvalidID", err)
	}
	if ids, _ := s.IDs(ctx); len(ids) != 1 {
		t.Fatalf("store holds %v, want only the good blob", ids)
	}
}
