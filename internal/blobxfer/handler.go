// Package blobxfer moves file contents between a device and its hub over
// HTTP, named by their SHA-256 (ADR-0018, docs/specs/sync-protocol.md#blobs):
// the hub's handler for /sync/blobs/{hash}, and the device's client.
// Neither end holds a blob in memory, and each checks the hash of what it
// receives before keeping it.
package blobxfer

import (
	"context"
	"net/http"

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
func Handler(store *blobs.Store, maxSize int64, readable func(ctx context.Context, hash string) (bool, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "blobxfer is not implemented yet (T-0029)", http.StatusNotImplemented)
	})
}
