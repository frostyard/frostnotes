package devgw

import (
	"crypto/subtle"
	"net/http"

	"github.com/frostyard/frostnotes/internal/blobhttp"
	"github.com/frostyard/frostnotes/internal/blobs"
)

// BlobHandler serves vault files' contents by their hash, so the UI in a
// browser shows images, audio and video as the desktop app does through
// notesblob:// (ADR-0023). A request is GET /blobs/<sha256>.<ext> with the
// gateway's token as ?token=, since an <img> sends no header the page
// sets; the extension only names the content type. It answers Range, as
// audio and video ask (blobhttp).
func BlobHandler(token string, store *blobs.Store) http.Handler {
	serve := blobhttp.Handler(store, blobhttp.Options{Prefix: "/blobs/"})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		got := r.URL.Query().Get("token")
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		serve.ServeHTTP(w, r)
	})
}

// ContentType returns the media type the app serves a vault file with, by
// its lower-cased extension (blobhttp.ContentType).
func ContentType(ext string) string { return blobhttp.ContentType(ext) }
