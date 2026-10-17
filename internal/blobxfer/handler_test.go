package blobxfer

// CONTRACT TEST for task card T-0029 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// hub is a test hub's blob endpoint: a store, the hashes it may hand out,
// and an httptest server with the handler mounted as the hub mounts it.
type hub struct {
	t        *testing.T
	store    *blobs.Store
	mu       sync.Mutex
	readable map[string]bool
	srv      *httptest.Server
}

func newHub(t *testing.T, maxSize int64, wrap func(http.Handler) http.Handler) *hub {
	t.Helper()
	h := &hub{t: t, store: blobs.New(t.TempDir()), readable: map[string]bool{}}
	handler := Handler(h.store, maxSize, func(_ context.Context, hash string) (bool, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.readable[hash], nil
	}, nil)
	if wrap != nil {
		handler = wrap(handler)
	}
	mux := http.NewServeMux()
	mux.Handle("/sync/blobs/{hash}", handler)
	h.srv = httptest.NewServer(mux)
	t.Cleanup(h.srv.Close)
	return h
}

// put stores content on the hub and lets it be read; it returns the hash.
func (h *hub) put(content string) string {
	h.t.Helper()
	id, err := h.store.Put(h.t.Context(), strings.NewReader(content))
	if err != nil {
		h.t.Fatal(err)
	}
	h.mu.Lock()
	h.readable[id] = true
	h.mu.Unlock()
	return id
}

func (h *hub) do(method, hash string, body io.Reader, header http.Header) *http.Response {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.srv.URL+"/sync/blobs/"+hash, body)
	if err != nil {
		h.t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHandlerPut(t *testing.T) {
	h := newHub(t, 1024, nil)
	content := "a note's bytes\n"
	id := blobs.ID([]byte(content))
	if resp := h.do(http.MethodPut, id, strings.NewReader(content), nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT = %s, want 201", resp.Status)
	}
	if got, err := h.store.Get(id); err != nil || string(got) != content {
		t.Fatalf("stored %q, %v", got, err)
	}
	// A blob the hub has already is read and checked like any other, and
	// answered the same: an upload tells nothing of what the hub holds.
	if resp := h.do(http.MethodPut, id, strings.NewReader(content), nil); resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT of a blob the hub has = %s, want 201", resp.Status)
	}
	if resp := h.do(http.MethodPut, id, strings.NewReader("not it"), nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT of a wrong body for a blob the hub has = %s, want 400", resp.Status)
	}

	other := blobs.ID([]byte("what the URL names"))
	if resp := h.do(http.MethodPut, other, strings.NewReader("forged"), nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT whose body does not hash to the URL's hash = %s, want 400", resp.Status)
	}
	for _, unwanted := range []string{other, blobs.ID([]byte("forged"))} {
		if ok, _ := h.store.Has(unwanted); ok {
			t.Fatalf("a refused PUT stored %s", unwanted)
		}
	}

	big := strings.Repeat("x", 2048)
	if resp := h.do(http.MethodPut, blobs.ID([]byte(big)), strings.NewReader(big), nil); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("PUT over the limit = %s, want 413", resp.Status)
	}
	if ok, _ := h.store.Has(blobs.ID([]byte(big))); ok {
		t.Fatal("a blob over the limit was stored")
	}

	// A body without a Content-Length (chunked).
	chunked := io.MultiReader(strings.NewReader("no "), strings.NewReader("length"))
	if resp := h.do(http.MethodPut, blobs.ID([]byte("no length")), chunked, nil); resp.StatusCode != http.StatusLengthRequired {
		t.Fatalf("PUT without Content-Length = %s, want 411", resp.Status)
	}
	for _, bad := range []string{"abc", strings.ToUpper(id), id + "0"} {
		if resp := h.do(http.MethodPut, bad, strings.NewReader(content), nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("PUT of hash %q = %s, want 400", bad, resp.Status)
		}
	}
	if resp := h.do(http.MethodPost, id, strings.NewReader(content), nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %s, want 405", resp.Status)
	}
}

func TestHandlerGet(t *testing.T) {
	h := newHub(t, DefaultMaxSize, nil)
	content := "0123456789abcdef"
	id := h.put(content)

	resp := h.do(http.MethodGet, id, nil, nil)
	if resp.StatusCode != http.StatusOK || readAll(t, resp.Body) != content {
		t.Fatalf("GET = %s", resp.Status)
	}
	if resp.Header.Get("Content-Type") != "application/octet-stream" || resp.Header.Get("ETag") != `"`+id+`"` ||
		resp.ContentLength != int64(len(content)) {
		t.Fatalf("GET headers = %v, length %d", resp.Header, resp.ContentLength)
	}
	ranged := h.do(http.MethodGet, id, nil, http.Header{"Range": {"bytes=10-"}})
	if ranged.StatusCode != http.StatusPartialContent || readAll(t, ranged.Body) != "abcdef" {
		t.Fatalf("GET with Range = %s, want 206 and the rest", ranged.Status)
	}
	head := h.do(http.MethodHead, id, nil, nil)
	if head.StatusCode != http.StatusOK || head.ContentLength != int64(len(content)) || readAll(t, head.Body) != "" {
		t.Fatalf("HEAD = %s, length %d", head.Status, head.ContentLength)
	}

	// Content the store has but no revision names is not handed out.
	hidden, err := h.store.Put(t.Context(), bytes.NewReader([]byte("another vault's note")))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if resp := h.do(method, hidden, nil, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s of unreadable content = %s, want 404", method, resp.Status)
		}
	}
	// Readable, but gone from the store.
	gone := blobs.ID([]byte("pruned"))
	h.mu.Lock()
	h.readable[gone] = true
	h.mu.Unlock()
	if resp := h.do(http.MethodGet, gone, nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET of a missing blob = %s, want 404", resp.Status)
	}
	if resp := h.do(http.MethodGet, "zz", nil, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET of a malformed hash = %s, want 400", resp.Status)
	}
}
