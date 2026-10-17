---
id: "0029"
title: Move blobs between a device and its hub
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/blobxfer/handler.go
  - internal/blobxfer/client.go
given:
  - internal/blobxfer/handler_test.go # copied from 0029-blob-transfer/_given/ when the task starts
  - internal/blobxfer/client_test.go
acceptance: go test ./internal/blobxfer -count=1 -timeout 120s
---
# T-0029: Move blobs between a device and its hub

## Goal

File contents travel outside JSON-RPC, over `/sync/blobs/{hash}`,
streamed and checked against their hash at both ends, so a 100 MB
attachment syncs in bounded memory (ADR-0018). Implement the hub's HTTP
handler and the device's client in `internal/blobxfer`.

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0018-move-blobs-over-http-named-by-their-hash.md
- docs/specs/sync-protocol.md: "Endpoints" and "Blobs".
- internal/blobxfer/handler.go and client.go: the stubs, `Client` and the
  doc comments (the contract).
- internal/blobs/blobs.go: `Store.Open`, `Has`, `Size`, `PutWant` (stores
  content only if it hashes to the ID wanted), `ErrCorrupt`, `ID`.
- internal/blobxfer/handler_test.go and client_test.go (given).

## Contract

Keep the exported names, signatures and doc comments; add unexported
helpers as you need.

**`Handler(store, maxSize, readable)`**, for `r.PathValue("hash")`:

- A hash that is not exactly 64 lowercase hex digits: 400.
- `GET` and `HEAD`: 404 when `readable` says false or the store lacks the
  blob; 500 when `readable` fails. Otherwise set `Content-Type:
  application/octet-stream` and `ETag: "<hash>"`, and serve the opened file
  with `http.ServeContent` (which handles `Range`, 206 and `HEAD`).
- `PUT`: 411 when the request has no `Content-Length` (`r.ContentLength
  < 0`); 413 when it is over `maxSize`, before reading the body; 204 when
  the store has the blob already; otherwise `store.PutWant` the body read
  through `http.MaxBytesReader(w, r.Body, r.ContentLength)`: 201 when
  stored, 400 when the content does not hash to the URL's hash (nothing is
  stored) or the body is short.
- Any other method: 405 with `Allow: GET, HEAD, PUT`.
- Errors are `http.Error` with one line saying why.

**`Client`**:

- Each request goes to `Base.JoinPath("sync", "blobs", hash)` with
  `Authorization: Bearer <Token>`, through `HTTP` or
  `http.DefaultClient`.
- A 401 answer is `ErrUnauthorized`; a 404 is `ErrNotFound`; any other
  failure an error naming the status and the hub's line.
- **`Upload(hash)`:** open the blob from `Store` and `PUT` it with
  `req.ContentLength` = its `Store.Size`; 201 or 204 is success. A blob
  the store lacks is an error.
- **`Download(hash)`:** return nil at once if `Store` has the blob.
  Otherwise append the response body to `Dir/<hash>.part`, asking with
  `Range: bytes=<n>-` when the part already holds `n > 0` bytes (from an
  earlier attempt or an earlier call). On 206 append; on 200 start the
  part again from empty. When a body breaks, try again, at most 3 attempts
  in all; `ErrUnauthorized` and `ErrNotFound` end at once. Then
  `Store.PutWant` the whole part with `hash` and remove the part, whether
  or not it matched; a mismatch returns the `blobs.ErrCorrupt` error.
- **`UploadAll`, `DownloadAll`:** run `Upload` or `Download` for every
  hash, at most 4 at once (a semaphore of 4), wait for all, and return
  `errors.Join` of the failures (nil when none).

## Tests (given, do not edit)

`internal/blobxfer/handler_test.go`: `TestHandlerPut`, `TestHandlerGet`.
`internal/blobxfer/client_test.go`: `TestUploadAndDownload`,
`TestDownloadChecksTheHash`, `TestDownloadResumes`,
`TestDownloadResumesAPartFromEarlier`, `TestTransfersRunFourAtATime`.

## Gotchas

- Never read a blob into memory: `io.Copy` between files and bodies.
- Check `Content-Length` and the size limit before reading the body, so a
  refused upload costs nothing.
- `TestTransfersRunFourAtATime` holds every request until four are in
  flight: a client that runs fewer than 4 at once makes it wait forever
  (the acceptance command's timeout ends it), and one that runs more fails
  it.
- `Store.Open` returns an `io.ReadCloser` that is an `*os.File`, which
  `http.ServeContent` needs as an `io.ReadSeeker`.
- A 416 for a `Range` means the part already holds the whole blob: check
  it with `PutWant` as usual.

## Out of scope

Authenticating requests (the hub's listener does, Phase 4), deciding what
is readable (`store.DB.HasContent`), choosing which blobs to move (the
sync loop), and every file other than the two under touch.

## Done when

`make accept T=0029` passes, `make check` passes, and only the files under
touch changed.
