---
id: "0075"
title: Draw embedded images, audio and video in live preview
milestone: M3
size: M
touch:
  - app/src/editor/preview/media.ts
given:
  - app/src/editor/preview/media.test.ts
acceptance: make ui-vitest F=src/editor/preview/media.test.ts
---
# T-0075: Draw embedded images, audio and video in live preview

## Goal

An embedded image shows in its line, sized as Obsidian sizes it; audio
and video play in place; a markdown image of a vault file shows, and
one from the web while the Load remote images preference is on
(`docs/specs/ui.md`, Editor and reading view). Away from the selection's
lines; a click on an image shows its source.

## Read first

- `app/src/editor/preview/media.ts`: the stub, already in `PROVIDERS`
  (`preview/index.ts`).
- `app/src/editor/preview/framework.ts`: `PreviewEnv` (`resolve`,
  `fileURL`, `remoteImages`).
- `app/src/lib/fileTree.ts`, `fileType`: a path's kind (`image`,
  `audio`, `video`, …).
- `app/src/editor/syntax/embeds.ts` (an `Embed` holds a `WikiLink` with
  its `WikiLinkTarget` and `WikiLinkAlias`) and @lezer/markdown's
  `Image` (`LinkMark`s, `URL`).
- `docs/specs/render.md`, "Links and embeds": the reading view's sizes.
- The given test.

## Contract

Keep the export `MEDIA: readonly Provider[]`; replace the file's
"Task T-0075 …" sentence with a description. Both providers are
`kind: "inline"`, draw only on lines the selection does not touch
(`!cx.revealed(node.from)`), return false when they draw, and answer a
click with a transaction putting the cursor at the widget's position
plus 2 (`view.posAtDOM(el) + 2`).

- **The widget** is an `img`, `audio` or `video` of class `cm-pv-media`
  with `data-pv="media"` and its `src`. An `img` has its `alt` and, when
  sized, `width` (and `height`) attributes; `audio` and `video` have
  `controls`. Equal by kind, URL, alt and size. `ignoreEvent` is false
  for an image and true for audio and video, whose controls take their
  own clicks.
- **Embeds** (`id: "media"`, `nodes: ["Embed"]`): the target resolves
  (`cx.env.resolve`, a string) to a path whose `fileType` is `image`,
  `audio` or `video`, and `cx.env.fileURL(path)` is not null. The alias,
  trimmed, sizes an image when it is digits (`300`: width) or digits,
  `x` and digits (`300x200`: width and height); then, or without an
  alias, the alt is the file's name (after the last `/`); else the alt
  is the alias. Replace the whole `Embed`.
- **Markdown images** (`id: "image"`, `nodes: ["Image"]`): an image with
  a `URL` (without `<` `>`). Its text is between `![` and its `]`; when
  the part after the text's last `|` is digits or digits `x` digits, it
  sizes the image and the alt is the part before, trimmed; else the alt
  is the text. A URL with a scheme (or `//`) shows when it is `http` or
  `https` and `cx.env.remoteImages()` is true, the URL itself as `src`.
  Any other is a vault path: percent-decoded, before any `#`, resolved
  with `cx.env.resolve`; it shows when the path is an image's and
  `fileURL` gives a URL. Replace the whole `Image`.

## Tests (given, do not edit)

`app/src/editor/preview/media.test.ts`: images in their line, sizes and
alt text, audio and video, what is left to links, the source on the
selection's lines, clicks; markdown images of the vault and the web.

## Gotchas

- An embed of a note is T-0074's (a block); a PDF or other file stays a
  link: draw nothing and return nothing for them.
- `decodeURIComponent` throws on a stray `%`: keep the text as it is
  then.

## Out of scope

Image styles (the planner's theme), note embeds (T-0074), and every
file except `app/src/editor/preview/media.ts`.

## Done when

`make accept T=0075`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/media.ts` changed.
