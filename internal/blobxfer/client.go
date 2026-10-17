package blobxfer

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// ErrUnauthorized means the hub refused the device token (401): it was
// revoked, or never valid.
var ErrUnauthorized = errors.New("blobxfer: the hub refused the device token")

// ErrNotFound means the hub has no content for the hash (404).
var ErrNotFound = errors.New("blobxfer: the hub has no such blob")

// Client moves blobs between a device's store and its hub.
type Client struct {
	// Base is the hub's URL; a blob's URL is Base with "/sync/blobs/<hash>"
	// appended to its path.
	Base *url.URL
	// Token is the device token, sent as "Authorization: Bearer <token>".
	Token string
	// HTTP sends the requests; nil means http.DefaultClient.
	HTTP *http.Client
	// Store is the device's blob store.
	Store *blobs.Store
	// Dir holds partial downloads as <hash>.part, so a broken download
	// resumes; it must exist.
	Dir string
}

// Upload sends blob hash from Store to the hub with a PUT that carries its
// Content-Length. 201 and 204 are success. Task card T-0029.
func (c *Client) Upload(ctx context.Context, hash string) error {
	return errors.New("blobxfer: Upload is not implemented yet (T-0029)")
}

// Download fetches blob hash from the hub into Store, keeping it only if
// its content hashes to hash (an error matching blobs.ErrCorrupt
// otherwise). The bytes arrive in Dir/<hash>.part; when a download breaks,
// the next attempt asks for the rest with Range, up to 3 attempts in one
// call, and a later call resumes too. A blob Store already has is not
// fetched. Task card T-0029.
func (c *Client) Download(ctx context.Context, hash string) error {
	return errors.New("blobxfer: Download is not implemented yet (T-0029)")
}

// UploadAll uploads each hash, at most 4 at a time, and returns every
// failure joined (errors.Join), after all have run. Task card T-0029.
func (c *Client) UploadAll(ctx context.Context, hashes []string) error {
	return errors.New("blobxfer: UploadAll is not implemented yet (T-0029)")
}

// DownloadAll downloads each hash, at most 4 at a time, and returns every
// failure joined (errors.Join), after all have run. Task card T-0029.
func (c *Client) DownloadAll(ctx context.Context, hashes []string) error {
	return errors.New("blobxfer: DownloadAll is not implemented yet (T-0029)")
}
