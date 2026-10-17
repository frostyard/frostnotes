package secrets

// CONTRACT TEST for task card T-0004 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestFile(t *testing.T) (*File, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	return NewFile(path), path
}

func TestFileRoundTrip(t *testing.T) {
	f, path := newTestFile(t)
	ctx := t.Context()
	if err := f.Set(ctx, HubToken("one"), "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set(ctx, HubToken("two"), "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set(ctx, HubToken("one"), "changed"); err != nil {
		t.Fatal(err)
	}
	reopened := NewFile(path)
	for key, want := range map[string]string{HubToken("one"): "changed", HubToken("two"): "correct horse"} {
		got, err := reopened.Get(ctx, key)
		if err != nil || got != want {
			t.Fatalf("Get(%s) = %q, %v; want %q", key, got, err, want)
		}
	}
}

func TestFileMissing(t *testing.T) {
	f, _ := newTestFile(t)
	ctx := t.Context()
	if _, err := f.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on a missing file = %v, want ErrNotFound", err)
	}
	if err := f.Delete(ctx, "nope"); err != nil {
		t.Fatalf("Delete on a missing file = %v", err)
	}
	if err := f.Set(ctx, "a", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(ctx, "b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get of a missing key = %v, want ErrNotFound", err)
	}
}

func TestFileModeAndNoLeftovers(t *testing.T) {
	f, path := newTestFile(t)
	ctx := t.Context()
	for i := range 3 {
		if err := f.Set(ctx, fmt.Sprintf("k%d", i), "v"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Delete(ctx, "k0"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "secrets.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("directory holds %v; temporary files must not remain", names)
	}
}

func TestFileRefusesReadableFile(t *testing.T) {
	f, path := newTestFile(t)
	if err := os.WriteFile(path, []byte(`{"a":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, err := f.Get(ctx, "a")
	if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), path) {
		t.Fatalf("Get on a 0644 file = %v; want an error naming %s", err, path)
	}
	if err := f.Set(ctx, "b", "2"); err == nil {
		t.Fatal("Set on a 0644 file succeeded; want an error")
	}
}

func TestFileCorrupt(t *testing.T) {
	f, path := newTestFile(t)
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(t.Context(), "a"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on a corrupt file = %v; want a decode error", err)
	}
}

func TestFileDelete(t *testing.T) {
	f, _ := newTestFile(t)
	ctx := t.Context()
	for _, k := range []string{"a", "b"} {
		if err := f.Set(ctx, k, k+"-value"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Get(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
	if v, err := f.Get(ctx, "b"); err != nil || v != "b-value" {
		t.Fatalf("Get(b) = %q, %v", v, err)
	}
}

func TestFileConcurrentSets(t *testing.T) {
	f, _ := newTestFile(t)
	ctx := t.Context()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if err := f.Set(ctx, fmt.Sprintf("k%d", i), fmt.Sprint(i)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for i := range 20 {
		if v, err := f.Get(ctx, fmt.Sprintf("k%d", i)); err != nil || v != fmt.Sprint(i) {
			t.Fatalf("k%d = %q, %v", i, v, err)
		}
	}
}
