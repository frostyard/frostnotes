# 0037 — Queries are Obsidian Bases, a subset that notesd compiles to SQL

- **Status:** Proposed
- **Date:** 2026-11-09

## Context

[Roadmap M5, Graph and queries](../plans/0002-roadmap.md), names the query
ADR, a query compiler to SQL, `.base` table and card views, and
`query_notes` for MCP. Its query done-when is that the user's Dataview
dashboards are rebuilt as `.base` files and render.

[Obsidian Bases](https://help.obsidian.md/bases) is a core plugin, available
since Obsidian 1.9. A base is YAML in a `.base` file or a `base` code block.
Its syntax has `filters`, `formulas`, `properties`, `summaries` and `views`.
Views include table, cards, list, kanban and map. A base includes every
file in the vault, attachments too, until filters narrow it. An embed is
`![[File.base]]` or `![[File.base#View]]`.

[Dataview](https://blacksmithgu.github.io/obsidian-dataview/) is a community
plugin. Its query language, DQL, has `TABLE`, `LIST`, `TASK`, `FROM` and
`WHERE`. It also has inline fields (`key:: value`) and `dataviewjs`, which
runs JavaScript. [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)
says rendered notes never run code; `dataviewjs` does not work.

Files are the truth, and the vault stays readable by Obsidian
([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)). notesd
owns the index; clients are thin
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)). The
[index](../design/vault.md#index) already holds files, frontmatter as JSON
in `notes.props`, and `props` rows per scalar and scalar list item, with
folded text and a number. It holds folded tags, including nested tags,
and links with their resolved `dest_id`. `ListNotes` filters by folder,
tag and property today. Frontmatter dates stay text as written. The index
keeps no file creation time. `.base` files are attachments in the index;
[sync merges them as text](../design/sync.md#merge).

[ADR-0012](0012-agents-reach-notes-through-mcp.md) already names
`query_notes`, "once M5 has them".

## Decision

**The query format is Obsidian Bases:** a `.base` file or a `base` code
block, in Obsidian's syntax. Frostnotes adds no keys of its own. In M5,
Frostnotes never rewrites a base file; the user edits it as text in the
editor. Within the subset below, a base that Obsidian opens works in
Frostnotes, and the reverse.

**notesd evaluates every query.** The app, the web app and agents get one
answer. M5 adds `note.query` to the `note` domain, additively in protocol
1. Its contract starts in `schema/rpc`, per
[ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md):

- Input: the vault; either `path` (a `.base` file) or `source` (a code
  block's text); `from` (the embedding note's path, which sets `this`);
  `view` (a name, with the first view as default); and `limit`.
- Output: the view's name and type; columns with key and display name;
  rows with path, title and column values; groups when the view has
  `groupBy`; `total`; and the column keys it could not show.
- Rows are files, notes and attachments, as in Obsidian. The schema pins
  the exact shape with M5's code.

**The M5 subset:**

- **Top-level keys:** `filters`, `properties` (with `displayName`) and
  `views`.
- **Filters:** a statement string, or an object with exactly one of `and`,
  `or` or `not`. That key holds a list of statements and objects, nested.
  `not` means none is true. Global and view filters join with AND.
- **Statements:** string, number and boolean literals; property references
  `note.key`, `note["key"]` and a bare `key`; and file properties
  `file.name`, `file.basename`, `file.path`, `file.folder`, `file.ext`,
  `file.size`, `file.mtime`, `file.tags` and `file.links`.
  `this.file.*` exposes the same file properties. Operators are `==`,
  `!=`, `>`, `<`, `>=`, `<=`, `&&`, `||`, `!` and parentheses.
- **File functions:** `file.hasTag(...)` matches any of the tags, including
  nested tags, ignoring case, as `ListNotes` does. `file.inFolder(f)`
  matches the folder or below it. `file.hasLink(x)` takes a name, a path
  or `this`/`this.file`. Resolve its argument with `markdown.Resolve`, as
  a link written in the base file. For a code block or embed, use the
  embedding note instead. With neither (`query_notes` with only
  `filters`), resolve from the vault root. The Obsidian fixtures confirm
  whether an embedded `.base` file resolves from itself or from the
  embedding note. `file.hasProperty(name)` tests whether the
  property exists.
- **Value functions:** on text, `contains`, `startsWith` and `endsWith`;
  on lists, `contains`, `containsAny` and `containsAll`; and `isEmpty()`.
  Dates have `date("…")`, `today()` and `now()`, and a date plus or minus
  a duration string, such as `now() - "1 week"`.
- **Views:** `type: table` and `type: cards`, with `name`, `filters`,
  `order` (the columns), `sort`, `limit` and `groupBy` (`property`,
  `direction`). Cards show the view's image property. The other card
  settings (size, fit and aspect ratio) go to the app as display settings.
  `sort` and the cards' `image` key are assumptions, not keys the docs
  show. The spec must confirm them and pin the exact view keys from files
  Obsidian writes: the syntax page lists only some of them.
- **Rendering:** `![[File.base]]`, `![[File.base#View]]` and `base` code
  blocks render in reading view and live preview. The app runs the query
  again on the vault's `note.*` events.

**Types come from the vault's settings, else from the comparison.** The
index keeps no property types. Obsidian keeps them in
`.obsidian/types.json`, and notesd already loads them for
`note.properties` (`vault.LoadTypes`: `.frostnotes/types.yaml`, else
Obsidian's file). A property with a type there (`number`, `checkbox`,
`date`, `datetime`, `text`, `multitext`, `tags`, `aliases`) compares as
that type. `multitext`, `tags` and `aliases` are lists; `text` is text.
For a property with no type, the comparison's other side sets it:

- A number literal compares numbers.
- A text literal compares text exactly; case counts.
- `date(…)`, `today()` or `now()` compares dates. A property's text is
  read as an ISO 8601 date: `YYYY-MM-DD`, with an optional time.

Every statement is true or false, never unknown. `!=` is exactly
`!(==)`. A missing value, or a value that does not convert, makes `==`,
`<`, `>`, `<=` and `>=` false and `!=` true. This follows Obsidian's
JavaScript semantics: `undefined != 5` is true. Attachment rows have no
note properties; every note property is missing for them.

For an untyped property, `x.contains("a")` tests a list item when the
row's value is a list, and a substring when it is text. Choose per row
with `json_type`; substrings use `instr()`, with case kept.

Frontmatter dates with no zone, `today()` and `now()` use notesd's local
time zone, the one its daily notes use. Missing values sort last. Where
Obsidian's docs say nothing, the spec pins its behavior with fixtures
checked against Obsidian's answers.

**Compile to SQL over the existing index.** A new package, `internal/query`,
parses the YAML and checks the subset. It folds constants in Go:
`today()`, `now()`, date arithmetic and `this`'s values from the index.
It compiles the filters into one SQL condition over `files LEFT JOIN
notes`, with bound parameters:

- `file.path`: `files.path`.
- `file.name`: the last segment of `files.path`, with case kept.
  `files.name` is lower-cased, so it is not used.
- `file.basename`: `file.name` without its last extension.
- `file.ext`: the text after the last `.` of `file.name`, or empty.
- `file.folder`: the path before the last `/`.
- `file.size` and `file.mtime`: `files.size` and `files.mtime`.
- `file.tags` reads `tags`; `file.links` reads `links`.
- `file.hasTag`: an `EXISTS` over `tags`, as in `ListNotes`.
- `file.inFolder`: a path range, as in `ListNotes`.
- `file.hasLink`: an `EXISTS` over `links`. When the argument resolves,
  match `links.dest_id`. Otherwise match `dest_id IS NULL` and the same
  `links.name`.
- `file.hasProperty`: `json_each(notes.props)`.
- List `contains`: `props` rows.
- Scalar comparisons: `notes.props` through SQLite's JSON functions,
  with `props.num` for numbers.
- Text functions: case-sensitive SQL, including `instr()` for substrings,
  never `LIKE`, which ignores ASCII case.

Match property keys with `json_each(notes.props)` and `key = ?`, never a
JSON path built from the key. A key matches as written in `notes.props`;
case counts. Look up its type lower-cased, as `LoadTypes` stores types.
The Obsidian fixtures confirm the key case rule. The spec also pins the
exact text of `file.folder` at the vault root and of `file.tags` items
(with or without `#`) from Obsidian's answers.

The compiler writes each statement so SQL never gives NULL, for example
with `COALESCE(…, 0)` and `IS` / `IS NOT`. Thus `!`, `not` and `||` never
lose a row without warning.

SQL supplies `sort`, `limit` and the total. With `groupBy`, rows order by
the group key first, then by `sort`, then by `files.path`. Without groups,
`files.path` is still the last sort key. `limit` counts rows across all
groups; `total` counts every matching row. Go reads groups and column
values from the rows' `notes.props`, tags and links. No text from a base
is spliced into SQL: property keys and literals are always bound
parameters. M5 adds no table; a migration comes only if a measurement
asks for one. The store (`store.DB`) runs the query; the engine serves
`note.query`.

**Ignore keys outside the subset.** A key outside the subset in
`properties` or `views`, or at the top level, is ignored. This includes
`formulas`, `summaries` and a plugin view's keys, so a base Obsidian writes
still opens. A formula used as a column stays empty. `groupOrder` is the
exception: it hides the groups it does not list, so to ignore it would
show more rows than Obsidian. A view with `groupOrder` shows a notice in
place of its rows, as a view type outside the subset does.

**Refuse a filter that cannot translate exactly.** The compiler's error
names the expression and its location, for example `views[0].filters`.
It never drops a condition and never returns a partial result: a dropped
condition shows too many rows without warning. A column it cannot show
(a formula or an unknown kind) stays empty, and the result names its key.
A view type outside the subset shows a notice in its place; the view list
still names it.

**Outside the subset at first:**

- `formulas`, `formula.*` and `summaries`;
- list, kanban, map and plugin views, and `groupOrder`;
- editing a base through a view's toolbar, and editing cells;
- `file.backlinks`, `file.embeds`, `file.properties`, `file.ctime` (the
  index keeps no creation time) and `file.file`;
- regular expressions, `filter`/`map`/`reduce`, and arithmetic on properties;
- `link()`, `image()`, `icon()` and `html()`;
- a base in a sidebar;
- Dataview DQL, `dataviewjs` and Dataview inline fields;
- Obsidian Tasks queries, which are later work in the roadmap.

A later ADR or the spec grows the subset.

**MCP `query_notes` is a read tool.** Its rough shape is:

- Input: `filters` (a Bases statement string or an `and`/`or`/`not`
  object), or `path` of a `.base` file with an optional `view`;
  `properties` (the property names to return, default none); `sort`;
  and `limit` (1–1000, default 200).
- Output: `files: [{path, title, url, properties, writable}]`, `total` and
  `notice`. For an attachment, `title` is `file.basename`. `writable`
  appears under a limited scope, as in `list_notes`. `notice` is the
  sentence every read tool's result carries
  ([MCP results](../specs/mcp.md#results)), never a view's notice.

**A view that shows a notice is a tool error in `query_notes`.** In the
app, a view with `groupOrder` or a view type outside the subset shows a
notice in place of its rows. `query_notes` with `path` and that `view`, or
with no `view` when the base's first view is such a view, returns a tool
error result, as for a refused filter. Its text names the view and the
key or view type outside the subset, at its location, for example
`views[1]: view type kanban is outside the subset`. It never returns
empty `files` with a `total` of 0: an agent would read that as a view
that matches no rows, which is a wrong answer without warning, as a
partial result is. `query_notes` with only `filters` has no view, so it
never meets this case.

It calls `note.query`. With only `filters` and no `path`, there is no
`this`; a filter that uses `this` is refused. It returns the named
properties, never a note's body. Through lodge its vault is the scope's
vault, under
[ADR-0028](0028-an-agents-scope-can-limit-writes-to-folders.md) and the
[MCP rules](../specs/mcp.md#rules). `docs/specs/mcp.md` changes with M5's
code: the read tool list grows from 5 to 6. Its rule that a result never
carries the content of a note not asked about gains a clause:
`query_notes` returns the frontmatter values the call names, never a body.

## Consequences

- One file format serves Obsidian and Frostnotes; the user rebuilds
  dashboards once. One evaluator serves every client. The compiler
  reuses the index and the `ListNotes` patterns.
- Bases is Obsidian's format and changes without Frostnotes. The subset
  lags it. A base with a feature outside the subset can work in Obsidian
  and error in Frostnotes.
- No Obsidian code may be read. Its docs leave gaps: case, sort order,
  missing values, and lists against text. Fixtures with Obsidian's
  answers must pin them, as the 179 checked links pin
  [link resolution](../design/vault.md#link-resolution).
- There is no translator from DQL; dashboards are rewritten by hand.
  `TASK` queries and inline fields have no equivalent in the subset.
  Inline fields are not in the index. Some dashboards can lose parts;
  M5's done-when can need a change after the inventory below.
- M5 must write `docs/specs/bases.md` with the compiler's code: the subset,
  types, errors and fixtures. It must add `query_notes` to
  [specs/mcp.md](../specs/mcp.md), generate
  [specs/rpc-api.md](../specs/rpc-api.md) from the schema, and add the
  query path to [design/vault.md](../design/vault.md).

**M5's plan must add these checks:**

1. Inventory the user's Dataview dashboards. Map each to a base in the
   subset, or name its gap. Use made-up names in every fixture, per the
   repository boundary in [AGENTS.md](../../AGENTS.md#repository-boundary).
2. Add golden fixtures: a base and a generated vault give expected rows
   in unit tests.
3. Open the fixture vault in Obsidian. Record its rows (export CSV or
   copy), and compare them with Frostnotes' rows.
4. Add a fuzz test that no base text reaches SQL text, only parameters.
5. Add a refusal test for each feature outside the subset: an error
   names it, never a partial result. Test ignored keys, empty columns and
   view notices where those rules apply instead.
6. Benchmark a `tools/vaultgen` vault with `b.Loop`, against a time budget
   the plan names.
7. Test `query_notes` in the MCP read tool list and under a read scope,
   with `notice`, pinned to the scope's vault through lodge. Test that a
   view with `groupOrder` and a view type outside the subset each give a
   tool error that names the view, and no `files`.
8. Test rendering: base views render values as text through the existing
   sanitizer, never script, per
   [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md).
9. Check that Frostnotes never rewrites a `.base` file in M5, byte for byte.
10. Test each operator, `!` and `not` on a missing property, on a value
    that does not convert, and on an attachment row.

## Alternatives considered

- **A Frostnotes condition language:** full control and a smaller grammar,
  but Obsidian cannot read it. The user keeps 2 formats, and the roadmap
  leans to Bases.
- **Dataview DQL:** it matches today's dashboards, but it is a community
  plugin's grammar with inline fields and `dataviewjs`, code ADR-0007
  forbids. Bases replaces it in Obsidian's core.
- **All of Bases in M5:** formulas, summaries and every view need an
  expression language evaluated in Go. That is too large for 1 milestone;
  the subset grows later.
- **Evaluate in the app, in TypeScript:** MCP needs the same answer, so
  there would be 2 evaluators, against ADR-0004.
- **Evaluate in Go over every note in memory:** simple for 666 notes, but
  each query reads every note's properties. The roadmap names a compiler
  to SQL over the index that exists.

## References

- Shapes: [roadmap — M5](../plans/0002-roadmap.md); with M5's code,
  [design/vault.md — Index](../design/vault.md#index),
  [specs/mcp.md](../specs/mcp.md),
  [specs/rpc-api.md](../specs/rpc-api.md), and a new `docs/specs/bases.md`.
- Builds on:
  [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md),
  [ADR-0004](0004-a-notes-daemon-with-thin-clients.md),
  [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md),
  [ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md),
  [ADR-0012](0012-agents-reach-notes-through-mcp.md),
  [ADR-0028](0028-an-agents-scope-can-limit-writes-to-folders.md).
- External: [Bases](https://help.obsidian.md/bases),
  [syntax](https://help.obsidian.md/bases/syntax),
  [functions](https://help.obsidian.md/bases/functions),
  [views](https://help.obsidian.md/bases/views),
  [table](https://help.obsidian.md/bases/views/table),
  [cards](https://help.obsidian.md/bases/views/cards),
  [create a base](https://help.obsidian.md/bases/create-base), and
  [Dataview](https://blacksmithgu.github.io/obsidian-dataview/).
