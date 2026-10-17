package blobxfer

// CONTRACT TEST for task card T-0029 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostnotes/internal/blobs"
)

func (h *hub) client(t *testing.T) *Client {
	t.Helper()
	base, err := url.Parse(h.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	return &Client{Base: base, Token: "fnd_secret", Store: blobs.New(t.TempDir()), Dir: t.TempDir()}
}

// requireToken refuses requests without the client's token, as the hub's
// listener does.
func requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fnd_secret" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestUploadAndDownload(t *testing.T) {
	h := newHub(t, DefaultMaxSize, requireToken)
	c := h.client(t)
	ctx := t.Context()
	content := strings.Repeat("attachment bytes ", 50_000)
	id, err := c.Store.Put(ctx, strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Upload(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got, err := h.store.Get(id); err != nil || string(got) != content {
		t.Fatalf("the hub holds %d bytes, %v", len(got), err)
	}
	if err := c.Upload(ctx, id); err != nil {
		t.Fatalf("uploading a blob the hub has = %v, want nil", err)
	}

	other := h.put("from another device")
	if err := c.Download(ctx, other); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Store.Get(other); err != nil || string(got) != "from another device" {
		t.Fatalf("downloaded %q, %v", got, err)
	}
	if parts, _ := filepath.Glob(filepath.Join(c.Dir, "*")); len(parts) != 0 {
		t.Fatalf("a finished download left %v", parts)
	}
	if err := c.Download(ctx, blobs.ID([]byte("never"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("downloading what the hub lacks = %v, want ErrNotFound", err)
	}
	if err := c.Upload(ctx, blobs.ID([]byte("not here"))); err == nil {
		t.Fatal("uploading a blob the device lacks succeeded")
	}

	c.Token = "fnd_revoked"
	if err := c.Upload(ctx, id); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Upload with a refused token = %v, want ErrUnauthorized", err)
	}
	if err := c.Download(ctx, h.put("more")); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Download with a refused token = %v, want ErrUnauthorized", err)
	}
}

func TestDownloadChecksTheHash(t *testing.T) {
	want := blobs.ID([]byte("the real content"))
	h := newHub(t, DefaultMaxSize, func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "something else") })
	})
	c := h.client(t)
	err := c.Download(t.Context(), want)
	if !errors.Is(err, blobs.ErrCorrupt) {
		t.Fatalf("Download of the wrong content = %v, want blobs.ErrCorrupt", err)
	}
	if ok, _ := c.Store.Has(want); ok {
		t.Fatal("the wrong content was stored")
	}
	if parts, _ := filepath.Glob(filepath.Join(c.Dir, "*")); len(parts) != 0 {
		t.Fatalf("a refused download left %v", parts)
	}
}

func TestDownloadResumes(t *testing.T) {
	content := strings.Repeat("0123456789", 10_000)
	var mu sync.Mutex
	var ranges []string
	h := newHub(t, DefaultMaxSize, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			ranges = append(ranges, r.Header.Get("Range"))
			first := len(ranges) == 1
			mu.Unlock()
			if !first {
				next.ServeHTTP(w, r)
				return
			}
			// The first answer breaks after 30,000 bytes.
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			_, _ = io.WriteString(w, content[:30_000])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		})
	})
	id := h.put(content)
	c := h.client(t)
	if err := c.Download(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Store.Get(id); err != nil || string(got) != content {
		t.Fatalf("a resumed download gave %d bytes, %v", len(got), err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 2 || ranges[0] != "" || ranges[1] != "bytes=30000-" {
		t.Fatalf("requests asked for %q; want the whole blob, then bytes=30000-", ranges)
	}
}

func TestDownloadResumesAPartFromEarlier(t *testing.T) {
	content := strings.Repeat("abcdefghij", 1000)
	var mu sync.Mutex
	var ranges []string
	h := newHub(t, DefaultMaxSize, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			ranges = append(ranges, r.Header.Get("Range"))
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	})
	id := h.put(content)
	c := h.client(t)
	if err := os.WriteFile(filepath.Join(c.Dir, id+".part"), []byte(content[:4000]), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Download(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Store.Get(id); string(got) != content {
		t.Fatal("resuming an earlier part gave the wrong content")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 1 || ranges[0] != "bytes=4000-" {
		t.Fatalf("requests asked for %q, want bytes=4000-", ranges)
	}
}

func TestTransfersRunFourAtATime(t *testing.T) {
	var mu sync.Mutex
	inFlight, most := 0, 0
	four := make(chan struct{})
	reached := sync.OnceFunc(func() { close(four) })
	release := make(chan struct{})
	h := newHub(t, DefaultMaxSize, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			inFlight++
			most = max(most, inFlight)
			if inFlight == 4 {
				reached()
			}
			mu.Unlock()
			<-release
			next.ServeHTTP(w, r)
			mu.Lock()
			inFlight--
			mu.Unlock()
		})
	})
	var hashes []string
	for i := range 10 {
		hashes = append(hashes, h.put(fmt.Sprintf("blob %d", i)))
	}
	hashes = append(hashes, blobs.ID([]byte("missing one")))
	c := h.client(t)
	done := make(chan error, 1)
	go func() { done <- c.DownloadAll(t.Context(), hashes) }()
	<-four
	close(release)
	err := <-done
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DownloadAll = %v; want the missing blob's ErrNotFound, after the others", err)
	}
	for _, id := range hashes[:10] {
		if ok, _ := c.Store.Has(id); !ok {
			t.Fatalf("DownloadAll did not fetch %s", id)
		}
	}
	mu.Lock()
	ran := most
	mu.Unlock()
	if ran != 4 {
		t.Fatalf("at most %d transfers ran at once, want 4", ran)
	}

	if err := c.UploadAll(t.Context(), hashes[:10]); err != nil {
		t.Fatal(err)
	}
}
