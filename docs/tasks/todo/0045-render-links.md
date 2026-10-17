---
id: "0045"
title: Render links, embeds and vault files
milestone: M3
size: M
touch:
  - internal/render/links.go
given:
  - internal/render/links_test.go
acceptance: go test ./internal/render -run 'TestLinkFixtures' -count=1
---
# T-0045: Render links, embeds and vault files

## Goal

The reading view shows a note as notesd renders it (`note.render`). Draw
its links and embeds as `docs/specs/render.md` pins them: wikilinks and
markdown links to vault files as internal links, other links as external
ones, embedded notes (whole, or a heading's section) as blocks nested at
most three deep, and images, audio, video and other files by their hash,
never by URL.

## Read first

- `docs/specs/render.md`: "Interface", "Links and embeds", the fixture
  vault and the fixtures tagged `[base]` and `[links]` (R01 to R11).
- `internal/render/render.go`: `Options`, `HTML`, the `obsidianHTML`
  extension, its `embed` (depth and cycles), `lineOf` and the fallback
  renderer your renderers replace.
- `internal/render/links.go`: the stub `extendLinks`.
- `internal/markdown/render.go`: `Goldmark`, `WikilinkKind`, `WikilinkOf`
  (a wikilink's `Link`); `internal/markdown/parse.go`: `Parse`, `Doc`,
  `Heading`.
- goldmark's `renderer/html/html.go` (`renderLink`, `renderImage`) and
  `extension/strikethrough.go`, for how a node renderer and an AST
  transformer register.
- The given test `internal/render/links_test.go` and the reader in
  `internal/render/fixtures_test.go`.
- `docs/tasks/EXECUTOR.md`.

## Contract

Keep `extendLinks`'s signature and doc comment; everything else in
`links.go` is yours. Every fixture tagged `[base]` or `[links]` must
render exactly as the spec shows, byte for byte.

- **Register** in `extendLinks`: node renderers (goldmark
  `renderer.WithNodeRenderers`, priority 100, below the fallback's 900 so
  yours win) for `markdown.WikilinkKind()`, `ast.KindLink` and
  `ast.KindImage`, and an AST transformer (`parser.WithASTTransformers`)
  for note embeds alone in a paragraph.
- **Wikilinks, markdown links, external links, embeds, files, markdown
  images and unresolved embeds** render exactly as the spec's "Links and
  embeds" says. The attribute order is the fixtures': `class`, then
  `data-href`, `data-path` (or `data-blob`, then `data-path`), `alt`,
  `width`, `height`; for an external link `class`, `href`, `title`,
  `rel`; for a remote image `src`, `alt`, `title`. Attribute values and text are HTML-escaped (`util.EscapeHTML`).
- **Resolving:** a link target resolves with `opts.Resolve(opts.Path,
  target)` (`""` when nil or unresolved). A markdown destination is a
  vault link when it has no scheme (`^[A-Za-z][A-Za-z0-9+.-]*:`) and does
  not start with `//`; its target is the destination percent-decoded
  (`url.PathUnescape`, kept as written when that fails) up to the first
  `#`, and the rest, with its `#`, is the subpath.
- **A file's type** is its lower-cased extension: `md` a note;
  `bmp png jpg jpeg gif svg webp avif` an image; `mp3 wav m4a 3gp flac
  ogg oga opus` audio; `mp4 webm ogv mov mkv` video; else another file.
  Its `data-blob` is `opts.File(path)`'s hash, `.`, and that extension.
- **Note embeds:** the transformer replaces each paragraph whose inline
  children are only embeds that resolve to notes (`md`) and text nodes of
  white space and line breaks with one block node per embed, in order.
  Such a block renders `<div class="embed" data-href data-path>`, a
  newline, the note rendered by `e.embed(path, note)`, then `</div>` and
  a newline. With a heading subpath (`#Goals`, not `#^b1`) only its
  section is rendered: the lines from the first heading whose text
  equals it, ignoring case (`markdown.Parse(note).Headings`), to the line
  before the next heading of its level or above, or to the end. When
  `e.embed` refuses (depth or a cycle), the block is `<p>`, an internal
  link to the note with its file name without `.md` (and the subpath) as
  text, `</p>` and a newline. A note `opts.Note` does not give renders as
  an unresolved embed inside `<p>…</p>`.
- **Everything else in a paragraph** stays inline: a note embed among
  other text is an internal link.
- No mutable package-level variables: compiled regexps are fine; write
  the extension table as a function with a `switch`.

## Tests (given, do not edit)

`internal/render/links_test.go`: `TestLinkFixtures`, which runs the
spec's `[base]` and `[links]` fixtures through `HTML`.

## Gotchas

- goldmark panics on a node kind nothing renders; the fallback in
  `render.go` covers the Obsidian extension's kinds, but a node kind you
  define must be registered by your renderer.
- An embed node holds the embedded note's blocks, and HTML cannot put a
  `div` inside a `p`: never write a note embed inside a paragraph.
- `e.embed` already counts depth and catches cycles; call it, don't
  re-implement it. An embedded note's links resolve from that note
  (`render.go` passes its path as `Options.Path`).
- An image's alt is its text (`![A map](…)`): walk the image's children
  for `ast.Text` and `ast.String` values.

## Out of scope

Tags, highlights, math, callouts, task checkboxes, headings and block IDs
(T-0046), sanitizing (Phase 4), the fixtures themselves, and every file
except `internal/render/links.go`.

## Done when

`make accept T=0045` and `make check` pass, and only
`internal/render/links.go` changed.
