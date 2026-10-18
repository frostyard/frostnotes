# 0007 — Edit in CodeMirror 6 with live preview

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

The user chose Obsidian's editing model: markdown that renders inline,
showing its syntax only where the cursor is. Obsidian builds this on
CodeMirror 6. Notes are files other tools edit, and merges work on their
text ([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)),
so the editor must keep the bytes exactly as typed. The reading view and
embeds need rendered HTML. Notes can hold raw HTML and pasted content, and
the hub serves them to browsers over the network.

## Decision

- **The editor is CodeMirror 6 over the note's markdown source.** It uses
  `@codemirror/lang-markdown` (Lezer, with GFM) plus our extensions for
  wikilinks, embeds, tags, callouts, highlights, comments, block IDs and
  math.
- **Live preview is decoration, never transformation.** It hides syntax
  away from the cursor and draws widgets for checkboxes, images, embeds,
  callouts and math. Source mode is the same editor without the
  decorations. The document CodeMirror holds is exactly the file, with no
  serialization step.
- **notesd renders the reading view and embeds.** It renders with goldmark
  and our extensions, then sanitizes: no script, event handlers, forms,
  frames or `javascript:` URLs. The app sanitizes again (DOMPurify) before
  inserting the HTML, under a CSP that allows no inline script. Remote
  images load by default, and a setting turns them off.
- **Math** renders with KaTeX in the client. Mermaid and other code-block
  renderers wait until after v1.

## Consequences

- Round trips are byte-exact. Outside tools and merges see exactly what was
  typed.
- Live preview has a long tail (tables, nested lists, embeds inside
  callouts). It grows widget by widget, and source mode is always correct.
- Two parsers read the same text: Lezer here and goldmark in notesd. Shared
  fixtures pin where they must agree.
- Rendered notes never run code, so Obsidian plugins and `dataviewjs`
  blocks do not work.
- One renderer serves desktop, web, embeds and later exports.

## Alternatives considered

- **TipTap or ProseMirror (WYSIWYG):** they serialize their document back
  to markdown, which rewrites list markers, spacing and unknown syntax, and
  so fights other tools and merges.
- **Separate source and reading panes:** simpler, but not the editing feel
  the user chose.
- **Milkdown or Lexical:** the same serialization problem as TipTap.
- **Monaco:** a code editor; weak at prose decorations and heavy in
  WebKitGTK.
- **Rendering in the client:** a second renderer to keep in step with the
  index, and the web app would trust browser-side sanitizing alone.

## References

- Shapes: [design/overview.md](../design/overview.md), [design/app.md](../design/app.md), [specs/markdown.md](../specs/markdown.md) (the fixtures both parsers run), [specs/ui.md](../specs/ui.md), [specs/render.md](../specs/render.md) (the reading view's HTML and sanitizer)
- Extended by: [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md) (saves and merges), [ADR-0023](0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md) (attachments)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md)
- Built in: [roadmap — M0 spike, M3](../plans/0002-roadmap.md), [plan 0005](../plans/0005-m3-desktop-app.md)
