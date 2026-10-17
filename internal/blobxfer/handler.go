// Package blobxfer moves file contents between a device and its hub over
// HTTP, named by their SHA-256 (ADR-0018, docs/specs/sync-protocol.md#blobs):
// the hub's handler for /sync/blobs/{hash}, and the device's client.
// Neither end holds a blob in memory, and each checks the hash of what it
// receives before keeping it.
package blobxfer

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// DefaultMaxSize is the hub's limit on one blob unless notesd --hub-max-blob
// sets another: 1 GiB.
const DefaultMaxSize = 1 << 30

// Handler serves HEAD, GET and PUT of one blob from store, for requests the
// hub has already authenticated; it reads the hash with
// r.PathValue("hash"), so it is mounted on a pattern ending in {hash}.
// readable reports whether GET and HEAD may hand out a hash: the hub passes
// store.DB.HasContent. PUT refuses blobs over maxSize. Task card T-0029.
func Handler(store *blobs.Store, maxSize int64, readable func(ctx context.Context, hash string) (bool, error),
	uploaded func(ctx context.Context, hash string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash := r.PathValue("hash")
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			http.Error(w, "invalid blob hash", http.StatusBadRequest)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			ok, err := readable(r.Context(), hash)
			if err != nil {
				http.Error(w, "cannot check blob readability", http.StatusInternalServerError)
			} else if !ok {
				http.Error(w, "blob not found", http.StatusNotFound)
			} else {
				serveBlob(w, r, store, hash)
			}
		case http.MethodPut:
			if putBlob(w, r, store, maxSize, hash) && uploaded != nil {
				uploaded(r.Context(), hash)
			}
		default:
			w.Header().Set("Allow", "GET, HEAD, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func serveBlob(w http.ResponseWriter, r *http.Request, store *blobs.Store, hash string) {
	f, err := store.Open(hash)
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "blob not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "cannot open blob", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	content, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "blob is not seekable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", `"`+hash+`"`)
	http.ServeContent(w, r, hash, time.Time{}, content)
}

// putBlob stores an upload and reports whether it did. The body is read
// and checked even when the store has the blob already, and the answer is
// the same either way, so an upload never tells a device what else the
// hub holds.
func putBlob(w http.ResponseWriter, r *http.Request, store *blobs.Store, maxSize int64, hash string) bool {
	if r.ContentLength < 0 {
		http.Error(w, "content-length required", http.StatusLengthRequired)
		return false
	}
	if r.ContentLength > maxSize {
		http.Error(w, "blob exceeds size limit", http.StatusRequestEntityTooLarge)
		return false
	}
	body := &uploadBody{reader: http.MaxBytesReader(w, r.Body, r.ContentLength), remaining: r.ContentLength}
	if err := store.PutWant(r.Context(), body, hash); err != nil {
		if errors.Is(err, blobs.ErrCorrupt) {
			http.Error(w, "content does not match blob hash", http.StatusBadRequest)
		} else if body.err != nil {
			http.Error(w, "invalid or incomplete request body", http.StatusBadRequest)
		} else {
			http.Error(w, "cannot store blob", http.StatusInternalServerError)
		}
		return false
	}
	w.WriteHeader(http.StatusCreated)
	return true
}

// uploadBody rejects a short body before PutWant can publish its contents.
type uploadBody struct {
	reader    io.Reader
	remaining int64
	err       error
}

func (b *uploadBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.remaining -= int64(n)
	if errors.Is(err, io.EOF) && b.remaining > 0 {
		err = io.ErrUnexpectedEOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.err = err
	}
	return n, err
}
