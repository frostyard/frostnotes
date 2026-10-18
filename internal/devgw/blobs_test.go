package devgw

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/blobs"
)

func TestBlobHandler(t *testing.T) {
	store := blobs.New(t.TempDir())
	content := []byte("not really a PNG, but bytes all the same\n")
	id, err := store.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	const token = "0123456789abcdef"
	srv := httptest.NewServer(BlobHandler(token, store))
	defer srv.Close()

	get := func(method, path string, header map[string]string) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range header {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}

	res := get(http.MethodGet, "/blobs/"+id+".png?token="+token, nil)
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, content) {
		t.Fatalf("GET = %d %q, want 200 and the blob", res.StatusCode, body)
	}
	for k, want := range map[string]string{
		"Content-Type":            "image/png",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; sandbox",
		"Cache-Control":           "private, max-age=31536000, immutable",
		"ETag":                    `"` + id + `"`,
	} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	res = get(http.MethodGet, "/blobs/"+id+".mp3?token="+token, map[string]string{"Range": "bytes=4-9"})
	body, _ = io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent || string(body) != string(content[4:10]) || res.Header.Get("Content-Type") != "audio/mpeg" {
		t.Errorf("Range = %d %q %q, want 206 %q audio/mpeg", res.StatusCode, body, res.Header.Get("Content-Type"), content[4:10])
	}
	if res := get(http.MethodHead, "/blobs/"+id+".bin?token="+token, nil); res.StatusCode != http.StatusOK ||
		res.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("HEAD = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/blobs/" + id + ".png", http.StatusForbidden},
		{http.MethodGet, "/blobs/" + id + ".png?token=wrong", http.StatusForbidden},
		{http.MethodPost, "/blobs/" + id + ".png?token=" + token, http.StatusMethodNotAllowed},
		{http.MethodGet, "/blobs/" + id + "?token=" + token, http.StatusNotFound},
		{http.MethodGet, "/blobs/" + strings.ToUpper(id) + ".png?token=" + token, http.StatusNotFound},
		{http.MethodGet, "/blobs/" + strings.Repeat("0", 64) + ".png?token=" + token, http.StatusNotFound},
		{http.MethodGet, "/blobs/../notesd.db?token=" + token, http.StatusNotFound},
		{http.MethodGet, "/blobs/" + id + ".png/x?token=" + token, http.StatusNotFound},
	} {
		if res := get(tc.method, tc.path, nil); res.StatusCode != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, res.StatusCode, tc.want)
		}
	}
	if res := get(http.MethodGet, "/blobs/"+id+".png?token=", nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("an empty token = %d, want 403", res.StatusCode)
	}
}

func TestBlobHandlerNeedsAToken(t *testing.T) {
	srv := httptest.NewServer(BlobHandler("", blobs.New(t.TempDir())))
	defer srv.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/blobs/"+strings.Repeat("a", 64)+".png?token=", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("no token configured: %d, want 403", res.StatusCode)
	}
}

func TestContentType(t *testing.T) {
	for ext, want := range map[string]string{
		"png": "image/png", "jpeg": "image/jpeg", "svg": "image/svg+xml", "opus": "audio/opus",
		"oga": "audio/ogg", "mkv": "video/x-matroska", "pdf": "application/pdf", "md": "text/markdown; charset=utf-8",
		"exe": "application/octet-stream", "": "application/octet-stream",
	} {
		if got := ContentType(ext); got != want {
			t.Errorf("ContentType(%q) = %q, want %q", ext, got, want)
		}
	}
}
