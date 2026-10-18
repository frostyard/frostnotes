package devgw

import (
	"crypto/subtle"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"time"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// blobPath is a blob's URL path: /blobs/<sha256>.<extension>.
var blobPath = regexp.MustCompile(`^/blobs/([0-9a-f]{64})\.([a-z0-9]{1,8})$`)

// BlobHandler serves vault files' contents by their hash, so the UI in a
// browser shows images, audio and video as the desktop app does through
// notesblob:// (ADR-0023). A request is GET /blobs/<sha256>.<ext> with the
// gateway's token as ?token=, since an <img> sends no header the page
// sets; the extension only names the content type. It answers Range, as
// audio and video ask.
func BlobHandler(token string, store *blobs.Store) http.Handler {
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
		m := blobPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
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
		h := w.Header()
		h.Set("Content-Type", ContentType(m[2]))
		h.Set("X-Content-Type-Options", "nosniff")
		// A file opened by its URL is passive content: an SVG must not run
		// script or load anything.
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		h.Set("Cache-Control", "private, max-age=31536000, immutable")
		h.Set("ETag", `"`+m[1]+`"`)
		http.ServeContent(w, r, "", time.Time{}, content)
	})
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
