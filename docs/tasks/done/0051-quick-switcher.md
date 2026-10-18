---
id: "0051"
title: Build the quick switcher over the fuzzy search
milestone: M3
size: M
touch:
  - app/src/lib/switcher.ts
  - app/src/features/prompt/QuickSwitcher.tsx
given:
  - app/src/lib/switcher.test.ts
  - app/src/features/prompt/QuickSwitcher.test.tsx
acceptance: make ui-vitest F="src/lib/switcher.test.ts src/features/prompt/QuickSwitcher.test.tsx"
---
# T-0051: Build the quick switcher over the fuzzy search

## Goal

Ctrl+O finds any note by a few letters of its name, its path or an
alias, as Obsidian's quick switcher does, and creates one when none
matches (`docs/specs/ui.md`, Prompts). Build its results as a pure
function over T-0039's fuzzy search, and its component over T-0049's
prompt frame. The container gives it the notes and the recent files,
and opens or creates what it asks for.

## Read first

- `app/src/lib/switcher.ts` and `app/src/features/prompt/QuickSwitcher.tsx`:
  the types and the stubs.
- `docs/specs/ui.md`, "Prompts: quick switcher and command palette".
- `app/src/lib/fuzzy.ts`: `prepareFuzzySearch`, `matchPath` (a path's
  last segment first, else the whole path for one point less) and
  `sortByScore` (stable).
- `app/src/features/prompt/Prompt.tsx`: `Prompt`, `PromptMods` and
  `highlight`, which T-0049 implements.
- The given tests `app/src/lib/switcher.test.ts` and
  `app/src/features/prompt/QuickSwitcher.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0051 …" sentence with a description.

**`stripMd(path)`** drops a final `.md` in any case (`Old.MD` → `Old`)
and changes nothing else.

**`switcherResults(notes, query, recent)`**, with `q` the query
trimmed:

- `q` empty: one `{ kind: "note", path, match: null }` per path of
  `recent`, in its order, skipping paths not among `notes` and paths
  already listed, at most `SWITCHER_LIMIT`.
- Else, with one `prepareFuzzySearch(q)`, for each note in `notes`'
  order: a `{ kind: "note", path, match }` when `matchPath(search,
  stripMd(path))` matches (so the spans are in the path without
  `.md`), then one `{ kind: "alias", path, alias, match }` for each
  alias, in order, that `search(alias)` matches (spans in the alias).
  Return them `sortByScore`d (equal scores keep this order), at most
  `SWITCHER_LIMIT`.

**`whereFor(mods)`**: `"right"` with Ctrl and Alt, `"tab"` with Ctrl
alone, else `"current"` (Shift does not count).

**`QuickSwitcher`** keeps the query in state (empty at first) and draws
`Prompt` with `label="Quick switcher"`, `placeholder="Find or create a
note…"`, the items `switcherResults(notes, query, recent)` (memoized),
`itemKey` unique per row (`note:<path>`, `alias:<path>:<alias>`), and:

- **A note row:** a `span` holding the note's name (the last segment of
  its path without `.md`) in `text-sidebar-row`, truncated, then, when
  it has one, its folder (the path without `.md` up to its last `/`) in
  a 12/16 `text-secondary` span 8px after. Both draw their part of the
  match with `highlight`: the name with `from` the name's start in the
  path without `.md`, the folder with `from` 0. A row's text is thus
  name then folder ("Garden planProjects").
- **An alias row:** the alias (`text-sidebar-row`, its matches
  highlighted), then a 12px Lucide `Forward` icon in `text-secondary`,
  then the note's name in a 12/16 `text-secondary` span, 8px apart.
- **Enter or a click** (`onEnter(item, mods)`), with `where =
  whereFor(mods)` and `name` the query trimmed: with Shift, or with no
  item, create: if `name` is empty do nothing at all; else call
  `onClose()`, then `onCreate(name, where)`. Otherwise call
  `onClose()`, then `onOpen(item.path, where)` (an alias row opens its
  note).
- **`onClose`** is passed to the frame as is (Escape, the backdrop).
- **The footer:** `↑↓` "to navigate", `↵` "to open", `ctrl ↵` "to open
  in new tab", `ctrl alt ↵` "to open to the right", `shift ↵` "to
  create", `esc` "to dismiss".
- **Empty:** `emptyText` "No recent files." while the query is blank,
  else "No notes found.".

## Tests (given, do not edit)

`app/src/lib/switcher.test.ts`: `stripMd`; recent notes for a blank
query; ranking, spans, aliases, stability and the limit.
`app/src/features/prompt/QuickSwitcher.test.tsx`: `whereFor`; the
dialog, recent rows, empty texts; matching with highlights in the name
and folder; alias rows; opening, its order with closing, and where;
creating; the footer.

## Gotchas

- `matchPath`'s spans index the whole path without `.md`, so a name's
  `from` is the index after its last `/` (0 at the vault's root).
- `highlight` comes from `./Prompt`; do not write another.
- The given component test renders T-0049's real `Prompt`: it runs only
  once T-0049 has merged.
- Presentational: no stores, no client, no workspace imports
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Recent files' bookkeeping, opening and creating notes, Ctrl+O (the
planner's), the command palette (T-0052), and every file except the two
under `touch`.

## Done when

`make accept T=0051`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
