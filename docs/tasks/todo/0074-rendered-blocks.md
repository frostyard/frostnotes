---
id: "0074"
title: Draw callouts, tables and note embeds as notesd renders them
milestone: M3
size: M
touch:
  - app/src/editor/preview/rendered.ts
given:
  - app/src/editor/preview/rendered.test.ts
acceptance: make ui-vitest F=src/editor/preview/rendered.test.ts
---
# T-0074: Draw callouts, tables and note embeds as notesd renders them

## Goal

Callouts, tables and embeds of notes are blocks in live preview, drawn
exactly as the reading view draws them while the selection is elsewhere
(`docs/specs/ui.md`, Editor and reading view;
`docs/design/app.md#live-preview`): the preview's environment renders a
block's markdown through notesd, sanitized, into an element. Links in
it follow, tags search, a foldable callout folds from its title, and a
click anywhere else shows the source.

## Read first

- `app/src/editor/preview/rendered.ts`: the stub, already in
  `PROVIDERS` (`preview/index.ts`).
- `app/src/editor/preview/framework.ts`: block providers, `PreviewEnv`
  (`render`, `resolve`), `PreviewAction` (`handled` for a click the
  provider dealt with), `PreviewCommand`.
- `app/src/editor/syntax/callouts.ts` (`readCallout`) and
  `syntax/embeds.ts` (an `Embed` holds a `WikiLink`).
- `app/src/reading/ReadingView.tsx`, `click`: how the reading view
  answers clicks on the same HTML.
- `docs/specs/render.md`: the HTML (`a.internal-link`, `a.file-embed`,
  `a.external-link`, `a.tag`, `.callout[data-fold]`).
- The given test, whose environment stands in for notesd.

## Contract

Keep the export `RENDERED: readonly Provider[]`; replace the file's
"Task T-0074 …" sentence with a description. Every provider here is
`kind: "block"` and returns false when it draws.

- **A rendered widget** (export it as `RenderedWidget`, built from the
  provider's id, the markdown and a class): a `div` of classes
  `cm-pv-rendered rendered <class>`, `data-pv` the provider's id,
  filled by `view.state.facet(previewEnv).render(el, markdown)` in
  `toDOM(view)`; equal by provider, markdown and class; `ignoreEvent`
  false. A block replaces whole lines: from the start of its first line
  to the end of its last, with `Decoration.replace({ widget, block: true })`,
  and its markdown is the text of those lines.
- **Callouts** (`id: "callout"`, `nodes: ["Blockquote"]`, class
  `cm-pv-callout`): a quote that starts its line (only white space
  before it), that `readCallout` reads as a callout, and that the
  selection does not touch (`!cx.revealed(node.from, node.to)`).
- **Tables** (`id: "table"`, `nodes: ["Table"]`, class `cm-pv-table`): a
  table that starts its line and the selection does not touch.
- **Note embeds** (`id: "embed"`, `nodes: ["Embed"]`, class
  `cm-pv-embed`): an embed that is its line's whole text (trimmed), on a
  line the selection does not touch, whose target
  `cx.env.resolve(target)` gives a path ending `.md` (not null, not
  undefined while it is asked).
- **A click** (export `renderedClick(el, view, event)`, every provider's
  `click`), reading the event's target:
  - in `a.tag[data-tag]`: search `tag:#<name>`;
  - in `a.internal-link` or `a.file-embed`: follow its `data-href` up
    to any `#`, its `data-path` as the path (null without one), where
    `whereFor(event)` says;
  - in `a.external-link`: follow its `href`, path null;
  - in a `.callout[data-fold] > .callout-title`: switch the callout's
    `data-fold` between `-` and `+` and answer `{ kind: "handled" }`;
  - elsewhere: a transaction that puts the cursor at the block's start
    (`view.posAtDOM(el)`), which shows its source.

## Tests (given, do not edit)

`app/src/editor/preview/rendered.test.ts`: callouts, tables and note
embeds as blocks, their source with the selection in them, what is not
drawn, rendering again only for a changed source, and each kind of
click.

## Gotchas

- The rendered HTML is the environment's to make and sanitize: never
  build HTML from the note's text here.
- CodeMirror keeps a widget's DOM while a new one compares equal, so a
  block renders again only when its markdown changes.

## Out of scope

Rendering, sanitizing and caching (`editor/rendered.ts`, the planner's),
images, audio and video (T-0075), the styles, and every file except
`app/src/editor/preview/rendered.ts`.

## Done when

`make accept T=0074`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/rendered.ts` changed.
