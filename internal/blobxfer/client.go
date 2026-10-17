package blobxfer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	// MaxSize is the largest blob a download takes before it gives up, so
	// a hub cannot fill the device's disk; 0 means DefaultMaxSize.
	MaxSize int64
}

// ErrTooBig is a download longer than the client's MaxSize.
var ErrTooBig = errors.New("blobxfer: the blob is larger than this device takes")

// Upload sends blob hash from Store to the hub with a PUT that carries its
// Content-Length. 201 and 204 are success. Task card T-0029.
func (c *Client) Upload(ctx context.Context, hash string) error {
	f, err := c.Store.Open(hash)
	if err != nil {
		return fmt.Errorf("open upload: %w", err)
	}
	defer func() { _ = f.Close() }()
	size, err := c.Store.Size(hash)
	if err != nil {
		return fmt.Errorf("size upload: %w", err)
	}
	req, err := c.request(ctx, http.MethodPut, hash, f)
	if err != nil {
		return err
	}
	req.ContentLength = size
	if size == 0 {
		req.Body = http.NoBody
	}
	resp, err := c.send(req)
	if err != nil {
		return fmt.Errorf("upload blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return responseError(resp)
	}
	return nil
}

// Download fetches blob hash from the hub into Store, keeping it only if
// its content hashes to hash (an error matching blobs.ErrCorrupt
// otherwise). The bytes arrive in Dir/<hash>.part; when a download breaks,
// the next attempt asks for the rest with Range, up to 3 attempts in one
// call, and a later call resumes too. A blob Store already has is not
// fetched. Task card T-0029.
func (c *Client) Download(ctx context.Context, hash string) error {
	has, err := c.Store.Has(hash)
	if err != nil {
		return fmt.Errorf("check downloaded blob: %w", err)
	}
	if has {
		return nil
	}
	part, err := os.OpenFile(filepath.Join(c.Dir, hash+".part"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open partial download: %w", err)
	}
	defer func() { _ = part.Close() }()
	for range 3 {
		var retry bool
		retry, err = c.downloadAttempt(ctx, hash, part)
		if err == nil {
			return c.finishDownload(ctx, hash, part)
		}
		if errors.Is(err, ErrTooBig) {
			_ = os.Remove(part.Name()) // what it took is not kept
			return err
		}
		if !retry || ctx.Err() != nil {
			return err
		}
	}
	return err
}

// UploadAll uploads each hash, at most 4 at a time, and returns every
// failure joined (errors.Join), after all have run. Task card T-0029.
func (c *Client) UploadAll(ctx context.Context, hashes []string) error {
	return transferAll(ctx, hashes, c.Upload)
}

// DownloadAll downloads each hash, at most 4 at a time, and returns every
// failure joined (errors.Join), after all have run. Task card T-0029.
func (c *Client) DownloadAll(ctx context.Context, hashes []string) error {
	return transferAll(ctx, hashes, c.Download)
}

func (c *Client) request(ctx context.Context, method, hash string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base.JoinPath("sync", "blobs", hash).String(), body)
	if err != nil {
		return nil, fmt.Errorf("create blob request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return req, nil
}

func (c *Client) send(req *http.Request) (*http.Response, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

func responseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	default:
		line, _ := bufio.NewReader(io.LimitReader(resp.Body, 4096)).ReadString('\n')
		return fmt.Errorf("hub returned %s: %s", resp.Status, strings.TrimSpace(line))
	}
}

// downloadAttempt reports whether a failed transfer can resume on another try.
func (c *Client) downloadAttempt(ctx context.Context, hash string, part *os.File) (bool, error) {
	offset, err := part.Seek(0, io.SeekEnd)
	if err != nil {
		return false, fmt.Errorf("seek partial download: %w", err)
	}
	req, err := c.request(ctx, http.MethodGet, hash, nil)
	if err != nil {
		return false, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := c.send(req)
	if err != nil {
		return true, fmt.Errorf("download blob: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := part.Truncate(0); err != nil {
			return false, fmt.Errorf("truncate partial download: %w", err)
		}
		if _, err := part.Seek(0, io.SeekStart); err != nil {
			return false, fmt.Errorf("restart partial download: %w", err)
		}
	case http.StatusPartialContent:
		// The file is positioned after the bytes from earlier attempts.
	case http.StatusRequestedRangeNotSatisfiable:
		if offset > 0 {
			return false, nil // Verify the complete part just like a finished body.
		}
		return false, responseError(resp)
	default:
		return false, responseError(resp)
	}
	limit := c.MaxSize
	if limit <= 0 {
		limit = DefaultMaxSize
	}
	at, err := part.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, fmt.Errorf("seek partial download: %w", err)
	}
	n, err := io.Copy(part, io.LimitReader(resp.Body, limit-at+1))
	if at+n > limit {
		return false, fmt.Errorf("download %s: %w", hash, ErrTooBig)
	}
	if err != nil {
		return true, fmt.Errorf("receive blob: %w", err)
	}
	return false, nil
}

func (c *Client) finishDownload(ctx context.Context, hash string, part *os.File) error {
	if _, err := part.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind partial download: %w", err)
	}
	err := c.Store.PutWant(ctx, part, hash)
	closeErr := part.Close()
	removeErr := os.Remove(part.Name())
	if err := errors.Join(err, closeErr, removeErr); err != nil {
		return fmt.Errorf("finish download: %w", err)
	}
	return nil
}

func transferAll(ctx context.Context, hashes []string, transfer func(context.Context, string) error) error {
	sem := make(chan struct{}, 4)
	errs := make([]error, len(hashes))
	var wg sync.WaitGroup
	for i, hash := range hashes {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			errs[i] = transfer(ctx, hash)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}
