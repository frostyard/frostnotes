---
id: "0079"
title: Build the template chooser for Insert template
milestone: M3
size: S
touch:
  - app/src/lib/templates.ts
  - app/src/features/prompt/TemplateChooser.tsx
given:
  - app/src/features/prompt/TemplateChooser.test.tsx
acceptance: make ui-vitest F=src/features/prompt/TemplateChooser.test.tsx
---
# T-0079: Build the template chooser for Insert template

## Goal

Insert template asks which of the template folder's notes to insert
(`docs/specs/ui.md`, Prompts), as Obsidian's core Templates plugin does.
Write its ranking and its prompt, as T-0063's folder chooser did for
Move file to.

## Read first

- `app/src/lib/templates.ts` and
  `app/src/features/prompt/TemplateChooser.tsx`: the types and stubs.
- `app/src/lib/folders.ts` and `app/src/features/prompt/FolderChooser.tsx`:
  the same shape for folders.
- `app/src/features/prompt/Prompt.tsx` (`Prompt`, `highlight`) and
  `app/src/lib/fuzzy.ts` (`prepareFuzzySearch`, `sortByScore`).
- The given test.

## Contract

Keep every exported name and signature; replace each file's "Task
T-0079 …" sentence with a description.

- **`templateName(path)`**: the file name (after the last `/`) without
  `.md` (any case).
- **`templateResults(paths, query)`**: each path once, as
  `{ path, name, match }`. A blank (or white-space) query lists them all
  by name ignoring case, then by path, with `match` null. Else the
  templates whose name matches the trimmed query
  (`prepareFuzzySearch`), best first (`sortByScore`), `match` the spans
  in the name.
- **`TemplateChooser`**: a `Prompt` labelled "Insert template", the
  placeholder `Choose a template…`, each row the name highlighted
  (`text-sidebar-row`, truncated), keyed `template:<path>`; Enter on a
  row calls `onClose()` then `onChoose(path)`; nothing when no row is
  chosen. Its footer: `↑↓` to navigate, `↵` to insert, `esc` to dismiss.
  With no rows: "No templates found.".

## Tests (given, do not edit)

`app/src/features/prompt/TemplateChooser.test.tsx`: blank and typed
queries, names, the prompt, choosing, nothing matching, the footer.

## Out of scope

Finding the template folder's notes and inserting the template (the
planner's), and every file except the two under `touch`.

## Done when

`make accept T=0079`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
