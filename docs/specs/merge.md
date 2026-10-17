# Spec: three-way merge

How `internal/merge` merges three versions of a text file: the base a
writer started from, the head (the version on disk for a local write, the
hub's head for a push), and the incoming version. Every notesd's write path
calls it when a write's base is behind ([vault.md](../design/vault.md#writes)),
and the hub calls it for a device's push ([sync.md](../design/sync.md#hub));
it decides whether edits survive. The planner owns it
([ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md)).
Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md);
where it fits: [design/sync.md](../design/sync.md#merge). Its first 30
cases are M0's spike ([plan 0001](../plans/0001-m0-foundations.md#phase-5--spikes-go-or-no-go));
the rest came with [plan 0004](../plans/0004-m2-sync.md).

## Interface

```go
package merge

// Text merges any UTF-8 text by line, then by word.
func Text(base, head, incoming []byte) Result
// Note merges a markdown note: frontmatter by top-level key, then the body as Text.
func Note(base, head, incoming []byte) Result
// Create merges two files created at one path with no common revision.
func Create(head, incoming []byte) Result

type Result struct {
	Text      []byte     // the merged file
	Conflicts []Conflict // hunks the head won, in order
}

type Conflict struct {
	Line                 int    // 1-based line of Text where the head's side starts
	Base, Head, Incoming string // the hunk's three versions
}
```

Which function a caller uses:

| File | A write or push whose base is behind | Two creates at one path |
| --- | --- | --- |
| a note (`.md`) of at most 10 MB | `Note` | `Create` |
| other UTF-8 text of at most 10 MB | `Text` | `Create` |
| a file that is not UTF-8, or is over 10 MB | no merge: a local write is refused (read again); on the hub, the later push wins and the file is flagged | the same |

A base whose content is gone (pruned on the hub,
[ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md))
merges as two creates. A local write's stray bytes that are not UTF-8 are
put back by `Text` too ([ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)).

## Rules

### Bytes, lines and words

- Nothing is normalized: line endings, a byte order mark, trailing spaces
  and a missing final newline stay as each side wrote them, and text only
  one side touched keeps its bytes.
- A line is the bytes up to and including `\n`; the last line may lack
  one. A `\r` belongs to its line.
- A word is a maximal run of characters that are all space or all not
  space (Go's `unicode.IsSpace`), so joining a line's words gives it back.
- The same three inputs always give the same bytes.

### Text

1. Base, head and incoming are compared by line (diff3, with Myers'
   linear-space diff of the base against each side).
2. A hunk that neither side changed, or that both changed the same way,
   or that only the head changed, takes the head's lines; one only the
   incoming side changed takes the incoming lines.
3. Where both sides only inserted lines at one place (the base has none
   there), both insertions are kept, the head's first.
4. A hunk both sides changed otherwise merges again by word, with rules 2
   and 3, except that insertions at one place conflict: interleaved words
   read worse than a conflict. A word hunk that conflicts keeps the head's
   words, and the line hunk is reported as one `Conflict`.

### Note

1. **Frontmatter** starts with a first line that is exactly `---` (with
   its line ending) and ends at the next line that is `---` or `...` and
   ends in a newline. Without both, the whole note is body.
2. **Units.** A line that starts a top-level key (it does not start with a
   space, a tab, `#`, `-` or a line ending, and has `key: ` or ends in
   `key:`) starts that key's unit; the lines after it (nested values,
   comments, blank lines) belong to it, and lines before the first key are
   one more unit. A key that appears twice makes the whole note merge as
   `Text`.
3. **Order:** the head's keys, then the incoming keys the head lacks.
4. **Each key:**

   | Base | Head | Incoming | Result |
   | --- | --- | --- | --- |
   | any | same as incoming | same as head | that unit |
   | as the head | left alone | changed, added or deleted | the incoming side's |
   | as the incoming | changed, added or deleted | left alone | the head's |
   | present | changed | changed | a flow list merge (rule 5) if it applies, else the units merged as `Text` |
   | absent | added | added differently | the head's, and a conflict |
   | present | changed | deleted | the head's, and a conflict |
   | present | deleted | changed | the incoming side's, and a conflict |

5. **Flow lists.** When the three units of a key are each one line holding
   a flow sequence of scalars (`tags: [work, q4]`, the way Obsidian writes
   tags and aliases), with items that are plain or quoted and contain no
   `[`, `]`, `{` or `}` outside quotes, the key merges as a list: the
   head's items in order, without those the incoming side removed from the
   base, then the incoming side's additions that the head lacks. Items are
   compared as written, trimmed of spaces. The line keeps the head's text
   before `[` and after `]`, and its items are written joined by `, `,
   unless they are the head's items, when the head's line stands as it is.
   A flow list merge never conflicts.
6. The delimiters are the head's, else the incoming side's, else the
   base's; frontmatter is written when a unit remains or both the head and
   the incoming side have it.
7. The body merges as `Text`. Conflict lines count from the note's first
   line.

### Create

1. Two equal files, or an empty incoming file, give the head; an empty
   head gives the incoming file.
2. Otherwise the lines both files share (their longest common subsequence)
   are the base, and the two merge as `Note`. Two daily notes made from
   one template merge cleanly this way.
3. Files that share no line conflict as a whole: the head stands, with
   one conflict at line 1.

### Conflicts

`Result.Text` holds the head's side of every conflict. The caller keeps
the incoming content whole, as a conflict version on a device or a
conflict revision on the hub, so nothing anyone wrote is lost
([ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).

## Fixtures

`internal/merge/fixtures_test.go` runs every `### Mnn` fixture below: its
JSON block's strings are the exact bytes (`\r`, `﻿` and a missing
final newline included); `kind` names the function (`text`, `note` or
`create`, which has no base); `want` is `Result.Text`, and `conflicts`
lists each conflict's `Line`, in order, when there are any.

### M01 — Nothing changed

```json
{
  "kind": "text",
  "base": "a\nb\n",
  "head": "a\nb\n",
  "incoming": "a\nb\n",
  "want": "a\nb\n"
}
```

### M02 — Only the head changed

```json
{
  "kind": "text",
  "base": "a\nb\n",
  "head": "a\nB\n",
  "incoming": "a\nb\n",
  "want": "a\nB\n"
}
```

### M03 — Only the incoming side changed

```json
{
  "kind": "text",
  "base": "a\nb\n",
  "head": "a\nb\n",
  "incoming": "A\nb\n",
  "want": "A\nb\n"
}
```

### M04 — The same change on both sides

```json
{
  "kind": "text",
  "base": "a\nb\n",
  "head": "a\nX\n",
  "incoming": "a\nX\n",
  "want": "a\nX\n"
}
```

### M05 — Different lines

```json
{
  "kind": "text",
  "base": "a\nb\nc\nd\ne\n",
  "head": "A\nb\nc\nd\ne\n",
  "incoming": "a\nb\nc\nd\nE\n",
  "want": "A\nb\nc\nd\nE\n"
}
```

### M06 — Adjacent lines merge by word

```json
{
  "kind": "text",
  "base": "a\nb\nc\nd\n",
  "head": "a\nB\nc\nd\n",
  "incoming": "a\nb\nC\nd\n",
  "want": "a\nB\nC\nd\n"
}
```

### M07 — Different words of one line

```json
{
  "kind": "text",
  "base": "the quick brown fox\n",
  "head": "the slow brown fox\n",
  "incoming": "the quick brown dog\n",
  "want": "the slow brown dog\n"
}
```

### M08 — The same word changed differently

```json
{
  "kind": "text",
  "base": "meet on monday\n",
  "head": "meet on tuesday\n",
  "incoming": "meet on friday\n",
  "want": "meet on tuesday\n",
  "conflicts": [1]
}
```

### M09 — Both appended lines

```json
{
  "kind": "text",
  "base": "# log\n",
  "head": "# log\nfrom the laptop\n",
  "incoming": "# log\nfrom the phone\n",
  "want": "# log\nfrom the laptop\nfrom the phone\n"
}
```

### M10 — Both inserted lines at one place mid-file

```json
{
  "kind": "text",
  "base": "a\nz\n",
  "head": "a\nh1\nh2\nz\n",
  "incoming": "a\ni1\nz\n",
  "want": "a\nh1\nh2\ni1\nz\n"
}
```

### M11 — A deletion and an edit elsewhere

```json
{
  "kind": "text",
  "base": "a\nb\nc\nd\ne\n",
  "head": "a\nc\nd\ne\n",
  "incoming": "a\nb\nc\nd\nE\n",
  "want": "a\nc\nd\nE\n"
}
```

### M12 — A deletion against an edit of the same line

```json
{
  "kind": "text",
  "base": "a\nb\nc\n",
  "head": "a\nc\n",
  "incoming": "a\nB\nc\n",
  "want": "a\nc\n",
  "conflicts": [2]
}
```

### M13 — A missing final newline is kept

```json
{
  "kind": "text",
  "base": "a\nb",
  "head": "top\na\nb",
  "incoming": "a\nB",
  "want": "top\na\nB"
}
```

### M14 — CRLF lines keep their endings

```json
{
  "kind": "text",
  "base": "a\r\nb\r\nc\r\n",
  "head": "A\r\nb\r\nc\r\n",
  "incoming": "a\r\nb\r\nC\r\n",
  "want": "A\r\nb\r\nC\r\n"
}
```

### M15 — Accented and emoji words

```json
{
  "kind": "text",
  "base": "café 🎉 naïve\n",
  "head": "café 🎊 naïve\n",
  "incoming": "café 🎉 naive\n",
  "want": "café 🎊 naive\n"
}
```

### M16 — Both inserted different words at one place

```json
{
  "kind": "text",
  "base": "the cat\n",
  "head": "the black cat\n",
  "incoming": "the fat cat\n",
  "want": "the black cat\n",
  "conflicts": [1]
}
```

### M17 — Indentation and a word change on one line

```json
{
  "kind": "text",
  "base": "- item one\n",
  "head": "  - item one\n",
  "incoming": "- item two\n",
  "want": "  - item two\n"
}
```

### M18 — Different keys

```json
{
  "kind": "note",
  "base": "---\ntitle: Plan\nstatus: draft\n---\nbody\n",
  "head": "---\ntitle: Plan v2\nstatus: draft\n---\nbody\n",
  "incoming": "---\ntitle: Plan\nstatus: done\n---\nbody\n",
  "want": "---\ntitle: Plan v2\nstatus: done\n---\nbody\n"
}
```

### M19 — One key changed differently

```json
{
  "kind": "note",
  "base": "---\nstatus: draft\n---\nbody\n",
  "head": "---\nstatus: review\n---\nbody\n",
  "incoming": "---\nstatus: done\n---\nbody\n",
  "want": "---\nstatus: review\n---\nbody\n",
  "conflicts": [2]
}
```

### M20 — Each side added a key

```json
{
  "kind": "note",
  "base": "---\ntitle: T\n---\n",
  "head": "---\ntitle: T\nowner: me\n---\n",
  "incoming": "---\ndue: 2026-10-20\ntitle: T\n---\n",
  "want": "---\ntitle: T\nowner: me\ndue: 2026-10-20\n---\n"
}
```

### M21 — A deleted key against a changed one

```json
{
  "kind": "note",
  "base": "---\ntitle: T\nstatus: draft\n---\n",
  "head": "---\ntitle: T\n---\n",
  "incoming": "---\ntitle: T\nstatus: done\n---\n",
  "want": "---\ntitle: T\nstatus: done\n---\n",
  "conflicts": [3]
}
```

### M22 — Both added tags to a list

```json
{
  "kind": "note",
  "base": "---\ntags:\n  - work\n---\n",
  "head": "---\ntags:\n  - work\n  - urgent\n---\n",
  "incoming": "---\ntags:\n  - work\n  - q4\n---\n",
  "want": "---\ntags:\n  - work\n  - urgent\n  - q4\n---\n"
}
```

### M23 — Frontmatter and body on both sides

```json
{
  "kind": "note",
  "base": "---\na: 1\nb: 2\n---\none\ntwo\nthree\n",
  "head": "---\na: 10\nb: 2\n---\nONE\ntwo\nthree\n",
  "incoming": "---\na: 1\nb: 20\n---\none\ntwo\nTHREE\n",
  "want": "---\na: 10\nb: 20\n---\nONE\ntwo\nTHREE\n"
}
```

### M24 — Frontmatter added on one side

```json
{
  "kind": "note",
  "base": "one\ntwo\n",
  "head": "one\nTWO\n",
  "incoming": "---\naliases: [first]\n---\none\ntwo\n",
  "want": "---\naliases: [first]\n---\none\nTWO\n"
}
```

### M25 — Untouched keys keep their bytes

```json
{
  "kind": "note",
  "base": "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 1\n---\n",
  "head": "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 2\n---\n",
  "incoming": "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 1\nnew: yes\n---\n",
  "want": "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 2\nnew: yes\n---\n"
}
```

### M26 — A rule in the body is not frontmatter

```json
{
  "kind": "note",
  "base": "intro\n---\nend\n",
  "head": "INTRO\n---\nend\n",
  "incoming": "intro\n---\nEND\n",
  "want": "INTRO\n---\nEND\n"
}
```

### M27 — A repeated key merges as text

```json
{
  "kind": "note",
  "base": "---\na: 1\na: 2\n---\nx\n",
  "head": "---\na: 1\na: 2\n---\nX\n",
  "incoming": "---\na: 1\na: 3\n---\nx\n",
  "want": "---\na: 1\na: 3\n---\nX\n"
}
```

### M28 — Two daily notes from one template

```json
{
  "kind": "create",
  "head": "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\n\n",
  "incoming": "# 2026-10-16\n## Tasks\n## Notes\nmet Ana at lunch\n\n",
  "want": "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\nmet Ana at lunch\n\n"
}
```

### M29 — Two daily notes with tasks in one section

```json
{
  "kind": "create",
  "head": "---\ntags: [daily]\n---\n## Tasks\n- [ ] laptop task\n## Notes\n",
  "incoming": "---\ntags: [daily]\n---\n## Tasks\n- [ ] phone task\n## Notes\n",
  "want": "---\ntags: [daily]\n---\n## Tasks\n- [ ] laptop task\n- [ ] phone task\n## Notes\n"
}
```

### M30 — Two creates with nothing in common

```json
{
  "kind": "create",
  "head": "alpha\n",
  "incoming": "beta\n",
  "want": "alpha\n",
  "conflicts": [1]
}
```

### M31 — An older copy merged as a create repeats changed lines

```json
{
  "kind": "create",
  "head": "a\nB\nc\n",
  "incoming": "a\nb\nc\n",
  "want": "a\nB\nb\nc\n"
}
```

### M32 — Both deleted the same line

```json
{
  "kind": "text",
  "base": "a\nb\nc\n",
  "head": "a\nc\n",
  "incoming": "a\nc\n",
  "want": "a\nc\n"
}
```

### M33 — One side emptied the file

```json
{
  "kind": "text",
  "base": "a\nb\n",
  "head": "",
  "incoming": "a\nb\nc\n",
  "want": "",
  "conflicts": [1]
}
```

### M34 — A byte order mark is kept

```json
{
  "kind": "text",
  "base": "\ufeffa\nb\n",
  "head": "\ufeffA\nb\n",
  "incoming": "\ufeffa\nb\nc\n",
  "want": "\ufeffA\nb\nc\n"
}
```

### M35 — A moved paragraph against an edit of it

```json
{
  "kind": "text",
  "base": "p1\n\np2\n",
  "head": "p2\n\np1\n",
  "incoming": "P1\n\np2\n",
  "want": "p2\n\np1\n",
  "conflicts": [1]
}
```

### M36 — Two identical creates

```json
{
  "kind": "create",
  "head": "x\n",
  "incoming": "x\n",
  "want": "x\n"
}
```

### M37 — A create on an empty file

```json
{
  "kind": "create",
  "head": "",
  "incoming": "# Plan\n",
  "want": "# Plan\n"
}
```

### M38 — An empty create

```json
{
  "kind": "create",
  "head": "# Plan\n",
  "incoming": "",
  "want": "# Plan\n"
}
```

### M39 — Both added tags to a flow list

```json
{
  "kind": "note",
  "base": "---\ntags: [work]\n---\n",
  "head": "---\ntags: [work, urgent]\n---\n",
  "incoming": "---\ntags: [work, q4]\n---\n",
  "want": "---\ntags: [work, urgent, q4]\n---\n"
}
```

### M40 — A flow list with a removal on one side and an addition on the other

```json
{
  "kind": "note",
  "base": "---\ntags: [a, b]\ntitle: x\n---\nbody\n",
  "head": "---\ntags: [a, b, c]\ntitle: x\n---\nbody\n",
  "incoming": "---\ntags: [b]\ntitle: x\n---\nbody\n",
  "want": "---\ntags: [b, c]\ntitle: x\n---\nbody\n"
}
```

### M41 — Quoted items in a flow list

```json
{
  "kind": "note",
  "base": "---\naliases: [\"One, two\"]\n---\n",
  "head": "---\naliases: [\"One, two\", Three]\n---\n",
  "incoming": "---\naliases: [\"One, two\", 'Four']\n---\n",
  "want": "---\naliases: [\"One, two\", Three, 'Four']\n---\n"
}
```

### M42 — A flow list that grew from empty on both sides

```json
{
  "kind": "note",
  "base": "---\ntags: []\n---\n",
  "head": "---\ntags: [x]\n---\n",
  "incoming": "---\ntags: [y]\n---\n",
  "want": "---\ntags: [x, y]\n---\n"
}
```

### M43 — A nested flow list merges as text

```json
{
  "kind": "note",
  "base": "---\nk: [a]\n---\n",
  "head": "---\nk: [a, [b]]\n---\n",
  "incoming": "---\nk: [a, c]\n---\n",
  "want": "---\nk: [a, [b]]\n---\n",
  "conflicts": [2]
}
```

### M44 — A flow list with CRLF lines

```json
{
  "kind": "note",
  "base": "---\r\ntags: [a]\r\n---\r\n",
  "head": "---\r\ntags: [a, b]\r\n---\r\n",
  "incoming": "---\r\ntags: [a, c]\r\n---\r\n",
  "want": "---\r\ntags: [a, b, c]\r\n---\r\n"
}
```

## References

- Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md), [ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md)
- Context: [design/sync.md](../design/sync.md#merge), [design/vault.md](../design/vault.md#writes)
- Built in: [plan 0001, Phase 5](../plans/0001-m0-foundations.md#phase-5--spikes-go-or-no-go) (the first 30 cases), [plan 0004, Phase 2](../plans/0004-m2-sync.md#phase-2--contracts-planner)
