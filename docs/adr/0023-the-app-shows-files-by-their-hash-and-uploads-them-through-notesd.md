# 0023 — The app shows files by their hash and uploads them through notesd

- **Status:** Proposed
- **Date:** 2026-10-17

## Context

Notes embed images, PDFs, audio and video (`![[photo.png]]`,
`![](scan.pdf)`), and live preview and the reading view must show them.
Pasting or dropping an image into a note adds an attachment: Obsidian
names it `Pasted image YYYYMMDDHHmmss.png`, puts it in the vault's
attachment folder and links it at the cursor.

The API carries text
([ADR-0014](0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)),
its messages stop at 16 MiB, and the Tauri bridge only moves lines. notesd
already keeps every file's content in its blob store, named by SHA-256
([ADR-0006](0006-every-write-names-the-version-it-changes.md)), and M2
moves blobs over HTTP by hash
([ADR-0018](0018-move-blobs-over-http-named-by-their-hash.md)).
Frostmail's app shows decoded mail parts through a Tauri URI scheme,
`mailpart://`, which its Rust side serves from maild's cache. In M4 the
web app loads the same files from the hub over HTTPS. ADR-0014 left
binary attachments to an upload method in M3.

## Decision

- **The app loads a file's bytes by its content hash,** from a URL made
  of the hash and the file's extension alone. It learns hashes from the
  file listing, from `note.changed`, and from rendered HTML, which names
  a vault file by `data-blob` instead of a URL.
  - On the desktop: `notesblob://localhost/<sha256>.<ext>`, a Tauri URI
    scheme. Its Rust handler accepts exactly 64 lowercase hex digits and
    an extension, reads `<data dir>/blobs/<first two digits>/<sha256>`
    (the data directory found as notesd finds it, like the socket in
    `paths.rs`), sets the content type from a fixed table of extensions
    (anything else is `application/octet-stream`), and answers with
    `Cache-Control: immutable`, `X-Content-Type-Options: nosniff`, a
    `Content-Security-Policy` of `default-src 'none'`, and `Range`
    support for audio and video.
  - In a browser during development: `internal/devgw` serves
    `GET /blobs/<sha256>.<ext>` behind its token.
  - In the web app (M4): the hub serves the same path behind the session,
    for content of the session's vaults only, as the M2 review required
    of a device's blob reads.
- **Uploads go through notesd:** `note.upload {vaultId, path, data}`
  takes base64 data of at most 10 MB decoded and creates the file as
  `note.create` does (it fails if the path exists, keeps a version, emits
  `note.changed`). A pasted or dropped image becomes `Pasted image
  YYYYMMDDHHmmss.<ext>` in the vault's attachment folder (Obsidian's
  default: the vault root), and the editor inserts `![[name]]` at the
  cursor. A larger file is refused with the advice to copy it into the
  vault's folder, where the scan finds it.

## Consequences

- Images, audio, video and PDFs of any size show without passing through
  JSON, and a URL names immutable content, so it caches forever and never
  goes stale; when a file changes, its hash and URL change with it.
- The handler takes no path, so there is no traversal to guard; it can
  read any blob notesd keeps, which is the same user's data on the same
  machine. The hub's version (M4) limits it to the session's vaults.
- The Rust side reads notesd's data directory, as frostmail's reads
  maild's cache: `paths.rs` must stay in step with `internal/config`, and
  M6's Flatpak needs read access to `blobs/`.
- The CSP grows `notesblob:` in `img-src` and `media-src`, and `https:`
  in `img-src` for remote images, which load by default
  ([ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)).
- Uploads cost a third more on the wire and a copy in memory, within the
  10 MB cap. This is the API's one base64 field, for bytes that are not
  text by nature; ADR-0014 holds for everything else.
- The mock transport serves blobs as `data:` URLs from its own files, so
  component tests need no scheme.

## Alternatives considered

- **Bytes through JSON-RPC for reads too:** every image a third larger
  and held whole in memory, nothing over about 12 MB, and no streaming or
  seeking for audio and video.
- **Rust reading vault files by path:** a second path guard outside
  notesd, symlinks and `..` to get right again, and it would show the file
  as it is on disk now rather than the version notesd indexed.
- **notesd serving HTTP on a loopback port:** a listener other users of
  the machine can reach, without the socket's peer-UID check.

## References

- Shapes: [design/app.md](../design/app.md#attachments), [specs/ui.md](../specs/ui.md); [specs/rpc-api.md](../specs/rpc-api.md) (`note.upload`, M3 Phase 3)
- Builds on: [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0014](0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md), [ADR-0018](0018-move-blobs-over-http-named-by-their-hash.md)
- Built in: [plan 0005 — M3](../plans/0005-m3-desktop-app.md)
