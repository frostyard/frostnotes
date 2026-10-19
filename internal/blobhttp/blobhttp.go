// Package blobhttp serves vault files' contents by their hash over HTTP
// (ADR-0023): GET and HEAD <prefix><sha256>.<ext>, with the content type
// the extension names, a policy that keeps the bytes passive, immutable
// caching and one Range per request (docs/specs/rpc-protocol.md,
// Transports). The development gateway serves it at /blobs/, and the lodge
// app socket at /content/blobs/ (docs/specs/lodge-app.md#contentblobs).
package blobhttp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// name is a blob's last path segment: <sha256>.<extension>.
var name = regexp.MustCompile(`^([0-9a-f]{64})\.([a-z0-9]{1,8})$`)

// Options configure a Handler. Zero limits are none.
type Options struct {
	// Prefix is the path before a blob's name, such as "/blobs/".
	Prefix string
	// MaxRange is the most bytes a 206 carries: a longer range, or an
	// open one, is cut to it.
	MaxRange int64
	// MaxWhole is the largest blob answered whole; a larger one without a
	// Range is 413.
	MaxWhole int64
	// Download lets ?download=1 add Content-Disposition: attachment, for
	// files a browser will not show in a sandboxed page.
	Download bool
}

// Handler serves store's blobs under opts.Prefix.
func Handler(store *blobs.Store, opts Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest, ok := strings.CutPrefix(r.URL.Path, opts.Prefix)
		m := name.FindStringSubmatch(rest)
		if !ok || m == nil {
			http.NotFound(w, r)
			return
		}
		f, err := store.Open(m[1])
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
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
		size, err := content.Seek(0, io.SeekEnd)
		if err == nil {
			_, err = content.Seek(0, io.SeekStart)
		}
		if err != nil {
			http.Error(w, "cannot read blob", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Type", ContentType(m[2]))
		h.Set("X-Content-Type-Options", "nosniff")
		// A file opened by its URL is passive content: an SVG must not run
		// script or load anything.
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
		h.Set("ETag", `"`+m[1]+`"`)
		if opts.Download && r.URL.Query().Get("download") == "1" {
			h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, m[1], m[2]))
		}
		// A blob never changes under its hash, so a real If-Range always
		// matches; one that does not would make ServeContent answer whole,
		// past the limits.
		r.Header.Del("If-Range")
		if rg := r.Header.Get("Range"); rg != "" {
			cut, ok := oneRange(rg, size, opts.MaxRange)
			if !ok {
				h.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
				http.Error(w, "one byte range per request", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			r.Header.Set("Range", cut)
		} else if opts.MaxWhole > 0 && size > opts.MaxWhole {
			http.Error(w, fmt.Sprintf("a file over %d bytes is served by range", opts.MaxWhole), http.StatusRequestEntityTooLarge)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, content)
	})
}

// oneRange checks a Range header for one satisfiable byte range of a blob
// of size bytes, and returns it cut to at most limit bytes (0: no limit).
func oneRange(header string, size, limit int64) (string, bool) {
	spec, ok := strings.CutPrefix(header, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return "", false
	}
	first, last, ok := strings.Cut(strings.TrimSpace(spec), "-")
	if !ok {
		return "", false
	}
	var start, end int64
	if first == "" { // the last n bytes
		n, err := strconv.ParseInt(last, 10, 64)
		if err != nil || n <= 0 {
			return "", false
		}
		start, end = max(size-n, 0), size-1
	} else {
		s, err := strconv.ParseInt(first, 10, 64)
		if err != nil || s < 0 || s >= size {
			return "", false
		}
		start, end = s, size-1
		if last != "" {
			e, err := strconv.ParseInt(last, 10, 64)
			if err != nil || e < s {
				return "", false
			}
			end = min(e, size-1)
		}
	}
	if limit > 0 && end-start+1 > limit {
		end = start + limit - 1
	}
	return fmt.Sprintf("bytes=%d-%d", start, end), true
}

// ContentType returns the media type the app serves a vault file with, by
// its lower-cased extension (docs/design/app.md#attachments); the Tauri
// side's table in app/src-tauri/src/protocol.rs is the same.
func ContentType(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	case "bmp":
		return "image/bmp"
	case "svg":
		return "image/svg+xml"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "m4a":
		return "audio/mp4"
	case "3gp":
		return "audio/3gpp"
	case "flac":
		return "audio/flac"
	case "ogg", "oga":
		return "audio/ogg"
	case "opus":
		return "audio/opus"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "ogv":
		return "video/ogg"
	case "mov":
		return "video/quicktime"
	case "mkv":
		return "video/x-matroska"
	case "pdf":
		return "application/pdf"
	case "md":
		return "text/markdown; charset=utf-8"
	}
	return "application/octet-stream"
}
