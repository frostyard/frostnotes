---
id: "0017"
title: List notes, and read their links and backlinks
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/queries.go
given:
  - internal/store/queries_test.go # copied from 0017-note-queries/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestListNotes|TestNoteInfo|TestLinksAndBacklinks' -count=1
---
# T-0017: List notes, and read their links and backlinks

## Goal

The queries behind `note.list`, `note.read`, `note.links` and
`note.backlinks`: notes by folder, tag or property, what the index holds
for one note, a note's links with the files they resolve to, and the
links to a file. Implement the stubs in `internal/store/queries.go`. Runs
after T-0015.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/rpc-api.md, section "note": `note.list`, `note.read`,
  `note.links`, `note.backlinks`, `NoteEntry`, `Link`, `Backlink`.
- internal/store/migrations/0001_init.sql: `files`, `notes`, `props`,
  `tags`, `links`.
- internal/store/queries.go: the stubs, `NoteFilter` and `NoteInfo`, with
  their doc comments (the contract).
- internal/store/index.go (T-0015): what each column holds.
- internal/store/queries_test.go (given).

## Contract

Keep the types, signatures and doc comments in `queries.go`. Every
method returns a non-nil empty slice when nothing matches.

**`ListNotes(vaultID, f)`**: the vault's notes (files with a `notes`
row), ordered by path, as `api.NoteEntry{Path, Title (notes.title), Hash,
Size, ModifiedAt: time.Unix(0, mtime).UTC()}`. Conditions, each only when
its field is set:

- `Folder`: trim `/` from both ends; then `path >= folder + "/"` and
  `path < folder + "0"` (`0` follows `/`), so `Orchard` matches
  `Orchard/x.md` and `Orchard/North Block/y.md` but not
  `Orchard Notes/z.md`. Case matters, as in paths.
- `Tag`: drop a leading `#`, lower-case it; a `tags` row of the note whose
  `folded` equals it, or lies in `[tag + "/", tag + "0")` (nested tags).
- `Property` alone: the key is one of the frontmatter's, even with a
  mapping or empty list as its value: `EXISTS (SELECT 1 FROM
  json_each(notes.props) WHERE key = ?)`.
- `Property` and `Value`: a `props` row with that `key` whose `folded`
  equals `strings.ToLower(Value)`.

**`NoteInfo(fileID)`**: from `notes`: `Title`, `Props` (the column's JSON
as `jsontext.Value`), `PropError`, `Aliases` (decoded), `Words`; and
`Tags`: the `tags` names by `ord`. `ErrNotFound` (wrapped) when the file
has no `notes` row.

**`Links(fileID)`**: the note's `links` by `ord` as `api.Link`, with
`Resolved` = the path of `dest_id`'s file, or nil.

**`Backlinks(fileID)`**: the links whose `dest_id` is `fileID`, except a
note's links to itself, ordered by the source's path, then `line`, then
`start`, as `api.Backlink{Source, Link}` with `Link.Resolved` set.

## Tests (given, do not edit)

`internal/store/queries_test.go`: `TestListNotes` (12 filters),
`TestNoteInfo`, `TestLinksAndBacklinks`.

## Gotchas

- Attachments have `files` rows but no `notes` row: join `notes` to list
  only notes.
- Query parameters, never string building with user values, for every
  filter.
- `LEFT JOIN` the destination file for `Links`: unresolved links have no
  `dest_id`.
- `props` has no rows for a mapping value or an empty list, which is why
  "has this key" reads `notes.props`.

## Out of scope

Search (T-0016), versions (T-0018), the engine's paths and errors, and
every file other than `queries.go`.

## Done when

`make accept T=0017` passes, `make check` passes, and only the files under
touch changed.
