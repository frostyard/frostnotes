---
id: "0008"
title: Generate test vaults with a manifest of their links
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/vaultgen/vaultgen.go
given:
  - internal/vaultgen/vaultgen_test.go # copied from 0008-vaultgen/_given/ when the task starts
acceptance: go test ./internal/vaultgen -count=1
---
# T-0008: Generate test vaults with a manifest of their links

## Goal

Add `internal/vaultgen`, which writes a deterministic vault of any size
that reproduces the user's vault's patterns with made-up names, and
returns a manifest of every link in it and the file each link must resolve
to. M1's tests use it for the cold-index timing, the `kill -9` tests, and
an end-to-end check that the index finds exactly the manifest's links and
resolves each one as the spec says. Real notes never enter the repository,
so this is how the tests see the user's patterns at scale.

**Runs after T-0004:** the given test calls `markdown.Resolve`.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/markdown.md: "Link resolution", the fixture vault (the
  orchard co-op names are the model for yours), and "Markdown links to
  vault files".
- internal/markdown/resolve.go (T-0004): `Resolve`, which the given test
  checks every link against.
- internal/vaultgen/vaultgen_test.go (given): every invariant below is
  checked there.

## Contract

New package `internal/vaultgen`, one file `vaultgen.go`, with a package
doc comment.

```go
type Options struct {
	Seed  uint64
	Notes int // how many .md files; at least 20
}

type Manifest struct {
	Seed        uint64   `json:"seed"`
	Notes       []string `json:"notes"`       // every note's vault path, sorted
	Attachments []string `json:"attachments"` // every other file, sorted
	Links       []Link   `json:"links"`       // every link, by Source then Start
	Decoys      []Link   `json:"decoys"`      // link-like text inside code, which is not a link
}

type Link struct {
	Source string `json:"source"`         // the note it is in
	Text   string `json:"text"`           // the exact markup, e.g. "[[Orchard/Plan#Goals|the plan]]"
	Start  int    `json:"start"`          // byte offset of Text in the note
	Kind   string `json:"kind"`           // "wiki" or "markdown"
	Target string `json:"target"`         // what a parser reports as the link's Target (percent-decoded)
	Want   string `json:"want,omitzero"`  // the vault path it resolves to; "" for a note that does not exist
}

// Generate writes a vault of o.Notes notes into dir and returns its manifest.
func Generate(dir string, o Options) (Manifest, error)
```

**Safety:** `dir` must be missing or an empty directory; otherwise return
an error and write nothing. `o.Notes < 20` is an error. Generate writes
only under `dir` (folders 0755, files 0644).

**Determinism:** all randomness comes from one
`rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))` (`math/rand/v2`),
used in a fixed order: never range over a map while generating. The same
`Options` give the same bytes and the same manifest.

**What the vault holds** (names made up; reuse the spec's orchard co-op
style; nothing like a real person's or company's name):

- **Daily notes** in the root, `YYYY-MM-DD.md`, one per day for the
  `clamp(Notes/10, 2, 90)` days ending 2026-10-16, all from one template:
  identical up to and including a `## Log` line except for the date. Some
  get a line after it.
- **Member folders** `Members/<Name>/` (at least 3), each with date-named
  meeting notes on consecutive Mondays from 2026-01-05, so the same date
  name is in every member folder; each meeting links the previous one by
  its bare name (relative, step 4 of the spec).
- **Topic folders**: top-level folders, some with child folders and at
  least one with a grandchild folder. Every top-level folder has an
  `Overview.md`; one has `Plan.md` in itself and in its child folders.
  Some top-level folders have a `media/` folder of `.png` attachments with
  unique names, linked from notes in the folder above as
  `![alt](./media/img-N.png)`.
- `Inbox.md` in the root holds at least one link of every form below, a
  `> [!important]` callout, and one decoy of each kind, so every vault has
  them all by construction.
- The rest are topic notes with unique names (words and a counter; at
  least one name with a space and a non-ASCII letter such as `Café`). The
  first ones go one to each folder, so no folder is empty, then at random.
- **Names:** folder names are unique, ignoring case, except the `media`
  folders, which hold only attachments and never sit in the root. Root note
  names are unique in the whole vault (member dates and daily dates never
  overlap).
- **Frontmatter:** daily, member and Inbox notes always; about 85% of topic
  notes. Valid YAML mappings: quoted `title`, `created`, a flow-list
  `tags`, sometimes a flow-list `aliases`.
- **Bodies:** a `# Title` heading, paragraphs of words, tasks (`- [ ]`,
  `- [x]`), inline tags, sometimes a callout, sometimes a decoy. About half
  of the notes end without a final newline.

**Links.** Choose the text for a wikilink from note `src` to note `dst` so
it resolves to `dst` without a tie: the bare name when `dst` is in `src`'s
folder or its name is unique in the vault; otherwise its full vault path
without `.md`. The folder-name rules above are what make the full path
unambiguous. Forms, each in the manifest with its `Want`:

| Form | Example `Text` | `Target` |
| --- | --- | --- |
| bare, unique or same folder | `[[Plan]]` | `Plan` |
| full path to a shared name | `[[Orchard/North Block/Plan]]` | `Orchard/North Block/Plan` |
| heading | `[[Garden/Overview#Goals]]` | `Garden/Overview` |
| alias | `[[Members/Wren/2026-01-05\|last meeting]]` | `Members/Wren/2026-01-05` |
| unresolved (`Want` "") | `[[Someday quince 17]]` | `Someday quince 17` |
| markdown link to a note | `[the plan](Garden/Caf%C3%A9%20ledger%203.md)` | `Garden/Café ledger 3.md` |
| relative image | `![a map](./media/img-1.png)` | `./media/img-1.png` |
| wiki embed of an attachment | `![[img-1.png]]` | `img-1.png` |

Topic notes get 0 to 3 links each at random, and every Overview links
its folder's `Plan` when there is one, so a vault has more links than
notes. In the table, `\|` is only the table's escape: the text has a
plain `|`. Markdown destinations escape each path segment with
`url.PathEscape`.
**Decoys** are `[[Decoy N]]` inside a fenced block (```` ``` ```` on its own
line before and after) or an inline code span, with `Kind` "wiki", the
`Target` a parser would report outside code, and no `Want`; no file is
named `Decoy N`. No real link is inside code.

## Tests (given, do not edit)

`internal/vaultgen/vaultgen_test.go`: `TestGenerateIsDeterministic`,
`TestGenerateRefuses`, `TestManifestMatchesTheVault` (every link and decoy
at its offset; every link resolves to its `Want` through
`markdown.Resolve`), `TestVaultPatterns` (daily template, shared names,
depth, bytes, frontmatter, every link form).

## Gotchas

- Record `Start` as the byte length of the note so far, just before
  writing the link's text; removing a final newline afterwards does not
  move it.
- A root note may not share its name: `[[2026-01-05]]` from a topic note
  would then depend on the spec's tie rule, which the manifest must not
  rely on.
- `Target` for a wikilink is the text before `#` or `|`; for a markdown
  link, the decoded destination (`url.PathUnescape`).
- Quote YAML strings that could read as other types (`title: "2026-01-12"`).
- 5,000 notes should take well under a second; never search the whole
  note list once per link in a loop over all links.

## Out of scope

The `tools/vaultgen` command (a later card), parsing, the index, timing
tests, and every file except `internal/vaultgen/vaultgen.go`.

## Done when

`make accept T=0008` passes, `make check` passes, and only the files under
touch changed.
