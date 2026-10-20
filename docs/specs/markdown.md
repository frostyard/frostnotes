# Spec: Obsidian-flavored markdown

The dialect notesd reads: what a note is, what its frontmatter means, which
text in its body is a link, tag, heading, block ID or task, and which file a
link names. `internal/markdown` implements it for the index, and the
editor's live preview (M3) must agree with it; both run the fixtures below.
Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md)
and [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md); where
it fits: [design/vault.md](../design/vault.md#parse). Built in
[plan 0003](../plans/0003-m1-headless-vault.md).

**Evidence.** Each rule names its source:

- **dump**: Obsidian's own answers, recorded on 2026-10-16 from the user's
  vault (179 links from 43 notes, [plan 0001, spike 4](../plans/0001-m0-foundations.md#phase-5--spikes-go-or-no-go)).
  The dump stays outside the repository; `TestIObsidianLinkDump` checks it
  locally ([plan 0003](../plans/0003-m1-headless-vault.md)).
- **test vault**: Obsidian's answers, recorded on 2026-10-16 with the same
  read-only snippet, for a vault of made-up notes built to settle what the
  user's vault does not show: links inside comments, and ties beyond the
  source's folder ([ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md)).
- **CommonMark** (0.31.2) and **GFM** (tables, strikethrough, autolinks,
  task list items).
- **Obsidian docs**: Obsidian's help on internal links, embeds, tags,
  properties, callouts, comments and block references.
- **choice**: Frostnotes' rule where Obsidian's behavior is unconfirmed. A
  choice changes, with its code and fixtures, when evidence disagrees.

## Notes and files

- A **note** is a file whose name ends in `.md`; every other file is an
  **attachment**. Files and folders whose names start with `.` are not part
  of the vault, except `.frostnotes/` (settings,
  [vault.md](../design/vault.md#on-disk)). (choice: `.MD` is an attachment.)
- A note's **title** is its file name without `.md`. A `title` property is
  an ordinary property. (Obsidian docs)
- Notes are read as bytes. A BOM, CRLF line endings, a missing final newline
  (384 of the user's 666 notes) and invalid UTF-8 are kept on disk exactly;
  the index and the API see invalid UTF-8 as U+FFFD, and writes keep the
  bytes ([ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)).
- **Positions:** `Start` and `End` are byte offsets in the whole note,
  frontmatter included; `Line` is the 1-based line of `Start`, counting
  `\n` bytes.

## Frontmatter

- The frontmatter is the text between a `---` line at the first byte (after
  an optional BOM) and the next line that is exactly `---` or `...`, CRLF
  allowed. Without a closing line there is no frontmatter
  (`markdown.SplitFrontmatter`, T-0001).
- It is a YAML 1.2 mapping. YAML that does not parse, is not a mapping, or
  repeats a key is a **property error**: the note has no properties,
  aliases, frontmatter tags or property links, and its body is still
  parsed. An empty frontmatter is an empty mapping.
- **`tags`**: a list of strings, or one string split at commas and white
  space (choice). A leading `#` is dropped. Each tag keeps its spelling;
  identity ignores case, as for inline tags. Tags from frontmatter carry the
  line of the `tags` key. (Obsidian docs)
- **`aliases`**: a list of strings, or one string, which is one alias
  (choice). Aliases name a note in search and in link suggestions; they do
  not resolve links ([rule 7](#link-resolution)). (Obsidian docs)
- **Links in properties:** each wikilink inside a string value, or a list
  item, of any key other than `tags` and `aliases` is a link of kind
  `property` carrying its key and the key's line. Obsidian requires such
  values to be quoted. (Obsidian docs)
- Every other key is a property. Its type comes from `.frostnotes/types.yaml`
  (or `.obsidian/types.json` until that exists); the index's typing is
  [vault.md](../design/vault.md#index)'s concern, not this spec's.

## Body

The body is everything after the frontmatter, parsed as CommonMark with
GFM's tables, strikethrough, autolinks and task list items, plus the
Obsidian syntax below.

**Where Obsidian syntax is not recognized** (CommonMark decides each
boundary):

- code: fenced blocks, indented blocks, and code spans. A backslash-escaped
  backtick outside code does not open a code span, and a backslash inside a
  code span is literal. (dump: 103 of the 268 wikilinks in the vault's note
  bodies are inside code; the rest, with the local markdown links, are 177
  links to Obsidian's 179, and the one note that differs has escaped
  backticks, fixture [F03](#f03--escaped-backticks))
- math: `$$…$$` blocks, and `$…$` or `$$…$$` spans within one line, where
  a single `$` opens only before a character that is not a space and
  closes only after one. A math block starts at a line beginning with
  `$$` and ends at the next line holding `$$`. (Obsidian docs)
- HTML blocks and inline HTML; link destinations, autolinks and bare URLs;
  link reference definitions.
- a character escaped with a backslash: `\[[x]]` is not a link, `\#x` is
  not a tag. (choice)

### Comments

- A comment is `%%…%%`, within a line or across lines and blocks; an
  unclosed `%%` runs to the end of the note. A `%%` in code, math or HTML
  opens no comment; inside a comment, the next `%%` closes it wherever it
  is. (Obsidian docs)
- Comments hide text when a note is rendered, not from the index: links
  inside them are indexed like any other (test vault: Obsidian indexed
  the links in an inline comment and in a comment block, 2026-10-16), and
  so are tags, headings, block IDs and tasks (choice, by analogy;
  [ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md)).
  Only `Words` leaves comments out.

### Wikilinks and embeds

- `[[`, then one or more characters other than `[`, `]` and a line break,
  then `]]`. A `!` right before `[[` makes it an embed, and `Start` is the
  `!`'s offset. (Obsidian docs; T-0002)
- The inner text splits at the first `|` (or `\|`, which tables require)
  into the target part and `Alias`; the target part splits at the first `#`
  into `Target` and `Subpath`, which keeps its `#` (`#Heading`,
  `#Heading#Sub`, `#^block-id`). `Target`, `Alias` and the text after `#`
  are trimmed of spaces. (Obsidian docs)
- A link whose `Target`, `Subpath` and `Alias` are all empty (`[[]]`,
  `[[ ]]`) is not a link. An empty `Target` with a subpath links to the
  note itself.

### Markdown links to vault files

- An inline link or image, or a reference link or image with a matching
  definition, is a link of kind `markdown` when its destination is not
  empty, has no URL scheme (`^[A-Za-z][A-Za-z0-9+.-]*:`) and does not
  start with `//`. An image is an embed. (dump: the 7 `./…png` images and
  the local `.md` links Obsidian counted)
- The destination, `<…>` brackets removed and its backslash escapes and
  entities resolved as CommonMark says, splits at the first `#` into
  `Target` and `Subpath`; both are percent-decoded (`%20` is a space). The
  link text, or an image's alt text, as plain text without its markup, is
  the `Alias`. (Obsidian docs)
- External links are not indexed. (choice)

### Tags

- A `#` right after white space or at the start of a block's text (a
  paragraph, heading, list item, quote or table cell: `>#tag` and
  `|#tag|` count), followed by one or more Unicode letters (with their
  combining marks), digits, `_`, `-` or `/`; the tag ends at the first
  other character. All digits (`#123`) is not a tag; `#2026-10` is.
  (Obsidian docs; T-0002; the block's start is CommonMark's)
- Tags count in headings, list items, quotes, table cells and comments,
  never inside a link, code, math or a URL.
- A note lists each tag once, compared ignoring case, with the spelling
  seen first; frontmatter tags come before inline ones.

### Headings

ATX and setext headings, as CommonMark defines them: a paragraph line
followed by `---` is a level-2 heading. `Text` is the heading's content as
written, trimmed, with an ATX closing sequence removed; a setext heading's
lines are joined with one space. `Line` is its first line. (CommonMark)

### Block IDs

`^` followed by one or more ASCII letters, digits or `-`, either at the end
of a paragraph's or list item's text after white space (so a `^id` line
that continues the paragraph counts), or as a paragraph of its own right
after a list, quote or table (blank lines between allowed). The index
records the ID and its line. (Obsidian docs)

### Tasks

A list item whose first line of content starts with `[`, one character, `]`
and a space. `Status` is the character: a space is open, `x` or `X` done;
other characters (`/`, `-`, `>`) are recorded as written. `Text` is the
rest of that line, trimmed. Tasks nest like list items. (GFM, extended as Obsidian
does; the user's vault uses only ` ` and `x`.)

### Callouts, highlights and footnotes

- A **callout** is a blockquote whose first line starts with `[!type]`,
  optionally `+` or `-` (folded), then an optional title; `type` is matched
  ignoring case. Its content is parsed as usual. (Obsidian docs; the user's
  vault has 46, all `important`, `note` or `warning`.)
- `==text==` is a highlight, and footnotes (`[^1]`) are GFM's; both are
  parsed as usual inside.

### Words

`Words` counts, in the body outside comments, each maximal run of non-space
characters that contains a letter or digit. Markup counts as written:
`[[Apple Butter]]` is two words, a lone list marker none. (choice)

## Parse output

`markdown.Parse(note []byte)` returns, for one note:

| Field | Contents |
| --- | --- |
| `Links` | in order of `Start`: `Kind` (`wiki`, `markdown` or `property`), `Target`, `Subpath`, `Alias`, `Embed`, `Key` (for `property`), `Start`, `End`, `Line` |
| `Tags` | `Name` (without `#`), `Front` (from frontmatter), `Line` |
| `Headings` | `Level` (1–6), `Text`, `Line` |
| `Blocks` | `ID` (without `^`), `Line` |
| `Tasks` | `Status`, `Text`, `Line` |
| `Aliases` | strings, in order |
| `Props` | the whole frontmatter mapping as decoded, `tags` and `aliases` included, as JSON values: a scalar by its YAML type (null, bool, integer, float), other scalars, infinities and dates as the text written |
| `PropError` | the YAML error, when there is one |
| `Words` | an integer |

A property link's `Start` and `End` cover the `[[…]]` inside the value.

## Link resolution

`markdown.Resolve(source, target, files)` returns the vault path of the
file a link names, or nothing when the link is unresolved. `source` is the
linking note's vault path; `target` is the link's `Target` (percent-decoded
for markdown links). The subpath and alias take no part. A vault path is
slash-separated and relative to the vault root. Comparisons **ignore
case**: both sides are lower-cased (Unicode, Go's `strings.ToLower`).
(dump: a case difference in a folder path and one in a bare name both
resolved)

1. An empty target names the source note itself. (Obsidian docs)
2. **Names.** A leading `/` marks the target as rooted and is removed. If
   the target ends in `.md` its only name is the target; otherwise its
   names are the target with `.md` added, then the target as written. So
   `[[soil-map]]` does not find `soil-map.png`. Each name is cleaned
   (`path.Clean`); a name that leaves the vault (`..`) matches nothing.
   Each step below tries the names in order and stops at the first name
   that matches a file. (Obsidian docs)
3. **Rooted:** a rooted target resolves only to the file whose path equals
   a name; otherwise it is unresolved. (choice) Inside lodge, the app
   follows an unresolved rooted link as lodge's address rather than
   creating the note ([ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md)).
4. **Relative:** the file at a name joined to the source's folder. (dump:
   25 of the 29 bare-name links Obsidian resolved point into the linking
   note's own folder, 14 of them with 3 files of that name; 21 links with
   folders and 8 `./` and `../` links resolve this way)
5. A target starting with `./` or `../` that step 4 did not resolve is
   unresolved, even if its name exists elsewhere. (dump: one link)
6. **Suffix:** the files whose path equals a name or ends with `/` and the
   name: a whole-segment suffix. (dump: the other 34 resolved links, 10 of
   them by their whole vault path; and 9 links with folders whose file name
   exists elsewhere stayed unresolved: a folder path never falls back to the
   bare name)
7. Nothing else resolves: not an alias, not a folder. (Obsidian docs; dump:
   no unresolved link matches an alias)

**Ties.** Within steps 3 and 4 all matches have the same path ignoring
case; the one equal in case wins, then the first in byte order. Within step
6 the order is: a file in the source's folder; a file whose matched part
equals the name in case; the fewest folders; the first in byte order.
(test vault: from a note in folder `Z`, `[[Shared]]` with matches one, two
and three folders deep went to the one-folder match; `[[Same]]` with two
matches one folder deep, to the first in byte order, though the other was
created first; `[[Root]]` to `Root.md` over `K/Root.md`; `[[x/Deep]]` to
the shallower of two suffix matches)

`files(name)` is the caller's lookup: every vault path whose last segment,
lower-cased, equals `name`, which is a cleaned name's last segment,
lower-cased. Every rule above needs only those files, so the index can
answer it from one indexed column.

### Fixture vault

The cases below run against this vault. Its names are made up; it
reproduces the user's patterns: one name in five folders, date-named notes
repeated across per-person folders, nested folders that share a name, an
attachment folder beside its notes, case differences and an alias.
`Recipes/Apple Butter.md` has `aliases: [Butter]`.

```text
2026-09-14.md
Garden/Beds.md
Garden/North Block/Soil.md
Garden/Overview.md
Inbox.md
Members/Ilsa/2026-09-14.md
Members/Ilsa/Café.md
Members/Overview.md
Members/Teodor/2026-09-14.md
Members/Teodor/2026-09-21.md
Members/Wren/2026-09-14.md
Members/Wren/Index.md
Members/Wren/Todo.md
Members/Wren/todo.md
Orchard/Harvest.md
Orchard/Index.md
Orchard/North Block/Overview.md
Orchard/North Block/Plan.md
Orchard/North Block/Soil.md
Orchard/North Block/media/soil-map.png
Orchard/Overview.md
Orchard/Plan.md
Overview.md
Plan.md
Recipes/Apple Butter.md
```

### Fixture cases

`—` is unresolved; `(empty)` is the empty target of `[[#Heading]]`.

| Source | Target | Resolves to | Why |
| --- | --- | --- | --- |
| `Inbox.md` | `Plan` | `Plan.md` | 4: relative to the vault root |
| `Orchard/Index.md` | `Plan` | `Orchard/Plan.md` | 4: the note's folder beats a shorter path |
| `Orchard/North Block/Soil.md` | `Plan` | `Orchard/North Block/Plan.md` | 4 |
| `Garden/Beds.md` | `Plan` | `Plan.md` | 6: none in the folder; fewest folders |
| `Garden/North Block/Soil.md` | `Overview` | `Overview.md` | 6: five matches, none in the folder; fewest folders, not the nearest |
| `Orchard/North Block/Plan.md` | `Overview` | `Orchard/North Block/Overview.md` | 4 |
| `Members/Wren/Index.md` | `2026-09-14` | `Members/Wren/2026-09-14.md` | 4: date names repeat across folders |
| `Members/Teodor/2026-09-21.md` | `2026-09-14` | `Members/Teodor/2026-09-14.md` | 4 |
| `Orchard/Harvest.md` | `2026-09-14` | `2026-09-14.md` | 6: fewest folders |
| `Members/Wren/Index.md` | `2026-09-21` | `Members/Teodor/2026-09-21.md` | 6: one match anywhere |
| `Garden/Beds.md` | `Orchard/Plan` | `Orchard/Plan.md` | 6: a path from the vault root |
| `Orchard/Index.md` | `North Block/Plan` | `Orchard/North Block/Plan.md` | 4: a folder path relative to the note |
| `Inbox.md` | `North Block/Soil` | `Garden/North Block/Soil.md` | 6: two matches at one depth; byte order |
| `Orchard/North Block/Plan.md` | `North Block/Soil` | `Orchard/North Block/Soil.md` | 6: the match in the note's folder |
| `Members/Wren/Index.md` | `Block/Soil` | — | 6: a suffix is whole segments |
| `Garden/Beds.md` | `Garden/Plan` | — | 6: a folder path never falls back to the name |
| `Orchard/Index.md` | `./Overview` | `Orchard/Overview.md` | 4 |
| `Recipes/Apple Butter.md` | `./Overview` | — | 5: no fallback for `./` |
| `Orchard/North Block/Plan.md` | `../Harvest` | `Orchard/Harvest.md` | 4 |
| `Orchard/North Block/Plan.md` | `./media/soil-map.png` | `Orchard/North Block/media/soil-map.png` | 4: an attachment beside its notes |
| `Inbox.md` | `../Plan` | — | 2: leaves the vault |
| `Inbox.md` | `soil-map.png` | `Orchard/North Block/media/soil-map.png` | 6 |
| `Inbox.md` | `soil-map` | — | 2: an attachment needs its extension |
| `Inbox.md` | `apple butter` | `Recipes/Apple Butter.md` | 6: case is ignored |
| `Inbox.md` | `recipes/APPLE BUTTER` | `Recipes/Apple Butter.md` | 6: in folders too |
| `Inbox.md` | `Apple Butter.md` | `Recipes/Apple Butter.md` | 2: `.md` written out |
| `Inbox.md` | `CAFÉ` | `Members/Ilsa/Café.md` | 6: Unicode case |
| `Inbox.md` | `Butter` | — | 7: an alias is not a name |
| `Inbox.md` | `Orchard` | — | 7: a folder is not a note |
| `Inbox.md` | (empty) | `Inbox.md` | 1 |
| `Orchard/Index.md` | `/Plan` | `Plan.md` | 3: rooted |
| `Orchard/Index.md` | `/Harvest` | — | 3: rooted, and not at the root |
| `Members/Wren/Index.md` | `Todo` | `Members/Wren/Todo.md` | 4: equal in case |
| `Members/Wren/Index.md` | `TODO` | `Members/Wren/Todo.md` | 4: then byte order |
| `Members/Wren/Index.md` | `todo` | `Members/Wren/todo.md` | 4: equal in case |

## Parse fixtures

Each fixture is a `###` heading starting with `F` and two digits, then a
fenced block with the info string `md` holding a whole note, then one with
the info string `json` holding what `Parse` returns. A note that contains
a fence is wrapped in a longer one. Fields the JSON omits must be empty or
zero; in a link,
`kind` defaults to `wiki`, `embed` to false and the strings to empty, and
`propError` is true when there is an error. Fixtures check lines, not
offsets; the Go tests check offsets. The Go and Vitest tests read the
fixtures from this file.

### F01 — Wikilink forms

```md
See [[Plan]], [[Orchard/Plan|the orchard plan]] and [[Plan#Goals]].
Blocks: [[Soil#^ph-test|pH]] and this note's [[#Tasks]].
![[soil-map.png]]
```

```json
{
  "links": [
    {"target": "Plan", "line": 1},
    {"target": "Orchard/Plan", "alias": "the orchard plan", "line": 1},
    {"target": "Plan", "subpath": "#Goals", "line": 1},
    {"target": "Soil", "subpath": "#^ph-test", "alias": "pH", "line": 2},
    {"subpath": "#Tasks", "line": 2},
    {"target": "soil-map.png", "embed": true, "line": 3}
  ],
  "words": 14
}
```

### F02 — Code and math hide links and tags

````md
Use `[[Not a link]]` and `#not-a-tag`, but [[Real]] #real.

```
[[Fenced]] #fenced
```

    [[Indented code]] #indented

$$
[[In math]]
$$
Inline math $[[x]]$ too.
````

```json
{
  "links": [{"target": "Real", "line": 1}],
  "tags": [{"name": "real", "line": 1}],
  "words": 20
}
```

### F03 — Escaped backticks

```md
| Old | New |
| --- | --- |
| `e.g. \`[[Alpha]]\`, \`[[Beta]]\`` | done |

Escaped \`[[Gamma]]\` is a link.
```

```json
{
  "links": [
    {"target": "Alpha", "line": 3},
    {"target": "Beta", "line": 3},
    {"target": "Gamma", "line": 5}
  ],
  "words": 11
}
```

### F04 — Markdown links to vault files

```md
[the plan](Plan.md), [soil](Orchard/North%20Block/Soil.md#pH%20test), ![map](./media/soil-map.png)
[angle](<Orchard/North Block/Plan.md>) and [here](#Tasks).
Not vault links: [site](https://example.com/x.md), <https://example.com>, [mail](mailto:wren@example.com).
A [ref link][r] uses a definition.

[r]: Recipes/Apple%20Butter.md
```

```json
{
  "links": [
    {"kind": "markdown", "target": "Plan.md", "alias": "the plan", "line": 1},
    {"kind": "markdown", "target": "Orchard/North Block/Soil.md", "subpath": "#pH test", "alias": "soil", "line": 1},
    {"kind": "markdown", "target": "./media/soil-map.png", "alias": "map", "embed": true, "line": 1},
    {"kind": "markdown", "target": "Orchard/North Block/Plan.md", "alias": "angle", "line": 2},
    {"kind": "markdown", "subpath": "#Tasks", "alias": "here", "line": 2},
    {"kind": "markdown", "target": "Recipes/Apple Butter.md", "alias": "ref link", "line": 4}
  ],
  "words": 22
}
```

### F05 — Tags

```md
#idea at the start, an #area/work-life tag, and #café_notes.
Not tags: a#b, http://example.com/#frag, #123, (#paren), \#escaped, [[Note #nope]].
## Heading with #heading-tag
- [ ] task with #todo
```

```json
{
  "links": [{"target": "Note", "subpath": "#nope", "line": 2}],
  "tags": [
    {"name": "idea", "line": 1},
    {"name": "area/work-life", "line": 1},
    {"name": "café_notes", "line": 1},
    {"name": "heading-tag", "line": 3},
    {"name": "todo", "line": 4}
  ],
  "headings": [{"level": 2, "text": "Heading with #heading-tag", "line": 3}],
  "tasks": [{"status": " ", "text": "task with #todo", "line": 4}],
  "words": 24
}
```

### F06 — Frontmatter tags, aliases and links

```md
---
title: Harvest log
tags: [harvest, "#orchard/north"]
aliases:
  - Picking log
  - Harvest
related: "[[Orchard/Plan]]"
---
Body with #inline and #Harvest.
```

```json
{
  "props": {
    "title": "Harvest log",
    "tags": ["harvest", "#orchard/north"],
    "aliases": ["Picking log", "Harvest"],
    "related": "[[Orchard/Plan]]"
  },
  "tags": [
    {"name": "harvest", "front": true, "line": 3},
    {"name": "orchard/north", "front": true, "line": 3},
    {"name": "inline", "line": 9}
  ],
  "aliases": ["Picking log", "Harvest"],
  "links": [{"kind": "property", "key": "related", "target": "Orchard/Plan", "line": 7}],
  "words": 5
}
```

### F07 — Tags and aliases as strings

```md
---
tags: harvest, orchard north
aliases: Picking log, autumn
---
```

```json
{
  "props": {"tags": "harvest, orchard north", "aliases": "Picking log, autumn"},
  "tags": [
    {"name": "harvest", "front": true, "line": 2},
    {"name": "orchard", "front": true, "line": 2},
    {"name": "north", "front": true, "line": 2}
  ],
  "aliases": ["Picking log, autumn"]
}
```

### F08 — Invalid frontmatter

```md
---
tags: [unclosed
---
Still parsed: [[Plan]] #body
```

```json
{
  "propError": true,
  "links": [{"target": "Plan", "line": 4}],
  "tags": [{"name": "body", "line": 4}],
  "words": 4
}
```

### F09 — Headings

```md
# Plan
Setext one
==========
## Goals ##
A paragraph line
---
#NotAHeading is a tag
####### Seven hashes is text
```

```json
{
  "headings": [
    {"level": 1, "text": "Plan", "line": 1},
    {"level": 1, "text": "Setext one", "line": 2},
    {"level": 2, "text": "Goals", "line": 4},
    {"level": 2, "text": "A paragraph line", "line": 5}
  ],
  "tags": [{"name": "NotAHeading", "line": 7}],
  "words": 15
}
```

### F10 — Block IDs

```md
A paragraph with an ID. ^para-1

- first item ^item-a
- second item

| a | b |
| - | - |
| 1 | 2 |

^table-1

`^not-an-id` and text ^not-at-end here
```

```json
{
  "blocks": [
    {"id": "para-1", "line": 1},
    {"id": "item-a", "line": 3},
    {"id": "table-1", "line": 10}
  ],
  "words": 21
}
```

### F11 — Tasks

```md
- [ ] open
- [x] done
* [X] done too
1. [/] in progress
    - [-] nested, cancelled
- [] not a task
Text [ ] not a task
```

```json
{
  "tasks": [
    {"status": " ", "text": "open", "line": 1},
    {"status": "x", "text": "done", "line": 2},
    {"status": "X", "text": "done too", "line": 3},
    {"status": "/", "text": "in progress", "line": 4},
    {"status": "-", "text": "nested, cancelled", "line": 5}
  ],
  "words": 18
}
```

### F12 — Highlights, comments and callouts

```md
Visible ==highlighted [[Plan]]== text.
Hidden %%[[Secret]] #secret%% inline.
%%
A block comment with [[Hidden]]
%%
> [!important] Watering
> Check [[Orchard/North Block/Soil]] before #watering.
```

```json
{
  "links": [
    {"target": "Plan", "line": 1},
    {"target": "Secret", "line": 2},
    {"target": "Hidden", "line": 4},
    {"target": "Orchard/North Block/Soil", "line": 7}
  ],
  "tags": [{"name": "secret", "line": 2}, {"name": "watering", "line": 7}],
  "words": 13
}
```

### F13 — A wikilink in a table cell

```md
| Note | Why |
| --- | --- |
| [[Recipes/Apple Butter\|Butter]] | tasty |
```

```json
{
  "links": [{"target": "Recipes/Apple Butter", "alias": "Butter", "line": 3}],
  "words": 5
}
```

### F14 — Not links

```md
[[]] and [[ ]] and [[a
b]] are not links, nor is \[[Escaped]] or [Single].
```

```json
{
  "words": 12
}
```

## References

- Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md) (invalid UTF-8), [ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md) (comments, ties)
- Context: [design/vault.md](../design/vault.md) (parse, index, link resolution), [design/app.md](../design/app.md#extensions) (the editor's parser runs the same fixtures)
- Followed: [ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md) (an unresolved rooted link inside lodge)
- Rendered: [specs/render.md](render.md) renders the same tree into HTML
- Built in: [plan 0003](../plans/0003-m1-headless-vault.md); evidence from [plan 0001, spike 4](../plans/0001-m0-foundations.md#phase-5--spikes-go-or-no-go)
