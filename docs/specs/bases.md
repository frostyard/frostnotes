# Spec: Obsidian Bases queries

The subset of [Obsidian Bases](https://help.obsidian.md/bases) that notesd
evaluates, the method `note.query` that evaluates it, and `notesctl query`.
`internal/query` parses a base and compiles it to SQL over the index;
`store.DB` runs the SQL; the engine serves `note.query` to every caller
kind of the `note` domain. The MCP tool `query_notes`
([specs/mcp.md](mcp.md#queries)) is a client of this contract; the app's
base views and the web app's are later steps of
[roadmap M5](../plans/0002-roadmap.md). Rationale:
[ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md); where
it fits: [design/vault.md](../design/vault.md#queries). Built in
[plan 0010](../plans/0010-m5-queries.md).

**Evidence.** Each rule names its source, as in
[specs/markdown.md](markdown.md):

- **Obsidian docs**: Obsidian's help on
  [syntax](https://help.obsidian.md/bases/syntax),
  [functions](https://help.obsidian.md/bases/functions) and
  [views](https://help.obsidian.md/bases/views).
- **ADR-0037**: the rule as the ADR decides it.
- **choice**: Frostnotes' rule where Obsidian's behavior is unconfirmed.
  The fixture vault has not yet been opened in Obsidian
  ([plan 0010](../plans/0010-m5-queries.md), check 3). A choice changes,
  with its code and fixtures, when Obsidian's answers disagree.

## Interface

### `note.query`

Params:

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `vaultId` | int | yes | a registered vault |
| `path` | string | one of `path`, `source` | a vault file, read as the base's YAML; at most 1 MiB |
| `source` | string | one of `path`, `source` | a base's YAML (a `base` code block's text); at most 1 MiB |
| `from` | string | no | a vault note's path: the note that embeds the base; it sets `this` |
| `view` | string | no | a view's `name`; the first view by default |
| `limit` | int | no | 1 to 1000; 1000 by default |

Result, type `QueryResult`:

| Field | Type | Meaning |
| --- | --- | --- |
| `view` | `QueryView` | the view evaluated: `name` and `type` as written |
| `views` | `[]QueryView` | every view in the base, in order, each with its name and type, also the views outside the subset |
| `notice` | string, optional | set when the view is outside the subset (below); then `columns`, `rows` and `groups` are empty and `total` is 0 |
| `columns` | `[]QueryColumn` | the view's columns: `key` as written in `order`, and `displayName` |
| `rows` | `[]QueryRow` | the matching files, sorted, at most the limit |
| `groups` | `[]QueryGroup`, optional | with `groupBy`: each group's `value` (JSON; null for the missing value) and `count`, in order; group *i* holds the next `count` rows of `rows` |
| `total` | int | every matching file, before the limit |
| `unshown` | `[]string` | the column keys whose values notesd cannot show, each once, in column order |
| `image` | string, optional | a cards view's `image` key |

A `QueryRow` has `path`, `title` (a note's title; an attachment's
`file.basename`), `kind` (`FileKind`), `values` (one JSON value per column,
in column order; null for a missing value or an unshown column) and, for
a cards view with `image`, `image` (that property's JSON value).

Errors: `notFound` (no such vault, or no file at `path` or `from`);
`invalidParams` (bad params, YAML that does not parse, a base outside the
subset, a filter that cannot be compiled exactly). An `invalidParams`
message for a base names the location and the expression, for example
`views[0].filters.and[1]: "file.backlinks.contains(this)": file.backlinks
is outside the subset`.

### `notesctl query`

```text
notesctl query PATH [--view NAME] [--limit N] [--from NOTE] [--json]
```

`PATH` is the vault path of a `.base` file; `-` reads the base's YAML from
standard input and sends it as `source`. Without `--json` it prints a
table: a header line of display names, then one line per row, values
separated by tabs (a list joins with `, `; a missing value is empty), then
`N of TOTAL rows`. A group starts with a line `## VALUE`. A notice prints
as `notice: TEXT` and exits 0. With `--json` it prints the `QueryResult`.

### Example

```yaml
filters:
  and:
    - file.inFolder("Projects")
    - 'status != "done"'
properties:
  note.status:
    displayName: Status
views:
  - type: table
    name: Open
    order: [file.name, note.status, note.due]
    sort:
      - property: note.due
        direction: ASC
    limit: 50
```

## Rules

### The document

- A base is a YAML mapping. Its keys in the subset are `filters`,
  `properties` and `views`; every other top-level key (`formulas`,
  `summaries`, …) is ignored. (ADR-0037)
- `properties` maps a property key to a mapping; only `displayName` (a
  string) is read. A key is `note.KEY`, `file.KEY`, `formula.KEY` or a
  bare `KEY`, which means `note.KEY`. (Obsidian docs)
- `views` is a list of mappings. A view has `type` and `name`; `table` and
  `cards` are in the subset. Their keys are `filters`, `order`, `sort`,
  `limit`, `groupBy` and, for cards, `image`. Other view keys are ignored,
  except `groupOrder`. (ADR-0037)
- A base without `views` has one view, `{type: table, name: ""}`. A view
  without `type` gives a notice, as a view type outside the subset does.
  (choice)
- A view of another type (list, kanban, map, a plugin's) or with
  `groupOrder` gives a `notice` and no rows, never an error:
  `views[1]: view type kanban is outside the subset`, or
  `views[1]: groupOrder is outside the subset`. (ADR-0037)
- `view` that names no view is `invalidParams`. When names repeat, the
  first view with the name wins. (choice)
- notesd checks the global filters and the evaluated view; it does not
  check the other views' filters. (choice)
- notesd never writes a base file. (ADR-0037)

### Filters

- A filter is a statement string, or a mapping with exactly one key,
  `and`, `or` or `not`, whose value is a list of filters, nested. `and`
  is true when every item is true; `or` when one is; `not` when none is.
  An empty `and` or `not` is true, an empty `or` is false. (Obsidian docs;
  the empty lists: choice)
- The global `filters` and the view's `filters` join with AND. A missing
  `filters` is true. (Obsidian docs)
- Anything else (another key, two keys, a list where a filter belongs, a
  YAML number or boolean such as `filters: true`) is `invalidParams` with
  its location; write a statement in quotes, `'true'`. (ADR-0037; YAML
  scalars: choice)
- An error in the document itself (YAML that does not parse, a document
  that is not a mapping) has the location `base`. YAML anchors and aliases
  are refused in the keys notesd reads. (choice)

### Statements

Grammar, in order of precedence from low to high:

```text
statement := or
or        := and ("||" and)*
and       := unary ("&&" unary)*
unary     := "!" unary | compare
compare   := sum (("==" | "!=" | ">" | "<" | ">=" | "<=") sum)?
sum       := postfix (("+" | "-") postfix)*
postfix   := primary ("." IDENT "(" args? ")")*
primary   := literal | reference | call | "(" statement ")"
call      := ("date" | "today" | "now") "(" args? ")"
           | "file." ("hasTag" | "inFolder" | "hasLink" | "hasProperty") "(" args? ")"
reference := "note." IDENT | "note[" STRING "]" | "file." IDENT
           | "this.file." IDENT | IDENT
literal   := STRING | NUMBER | "true" | "false"
```

- A string is in double or single quotes, with `\"`, `\'`, `\\`, `\n` and
  `\t` escapes. A number is decimal, with an optional sign, fraction and
  exponent. (choice)
- A bare `IDENT` is a note property, except the reserved words `file`,
  `note`, `this`, `formula`, `true`, `false`, `date`, `today` and `now`.
  `note.KEY` and `note["KEY"]` name the key as written; case counts.
  (ADR-0037)
- Outside the subset, and refused with the location: `formula.*`,
  `this.KEY`, `this.note.*`, the file properties `file.backlinks`,
  `file.embeds`, `file.properties`, `file.ctime` and `file.file`, any other
  function or method, `null`, regular expressions, arithmetic other than
  date arithmetic, and a comparison of two properties. A comparison with
  `this.file.*` is not a comparison of two properties: `this` folds to a
  constant. (ADR-0037)
- A literal on the left of a comparison is moved to the right with the
  operator mirrored (`5 < x` is `x > 5`). A comparison of two constants
  folds to true or false. (choice)

### Values and types

- A statement is true or false, never unknown. `!=` is exactly `!(==)`. A
  missing value, or one that does not convert, makes `==`, `<`, `>`, `<=`
  and `>=` false, and `!=` true. An attachment has no note properties.
  (ADR-0037)
- A property with a type in the vault's settings (`vault.LoadTypes`,
  looked up lower-cased) compares as that type: `number`, `checkbox`,
  `date`, `datetime`, `text`; `multitext`, `tags` and `aliases` are lists.
  Without a type, the literal on the other side sets the comparison. A
  literal of the wrong kind for a typed property (`priority == "5"` when
  `priority` is a `number`) is `invalidParams`, except a string literal
  for a `date` or `datetime` property, which is read as a date in the
  forms below. (ADR-0037; the refusal: choice)
- **Typed lists.** A `multitext`, `tags` or `aliases` property holds a
  list; a text value in it (`tags: work`) is a list of that 1 item.
  `==`, `!=` and the orders on such a property are `invalidParams`: use
  `contains`, `containsAny` or `isEmpty()`. (choice)
- **Numbers.** A number literal, or the type `number`, compares a JSON
  number. Text never converts to a number. (choice)
- **Text.** A string literal, or the type `text`, compares a JSON string
  exactly, case kept, by code point order (SQLite `BINARY`). A number or a
  boolean never equals a string. (ADR-0037; the order: choice)
- **Booleans.** `true` and `false`, or the type `checkbox`, compare JSON
  `true` and `false`; only `==` and `!=` apply. (choice)
- **Dates.** `date("…")`, `today()` and `now()`, or the type `date` or
  `datetime`, compare instants at millisecond precision. A property's text
  converts when it is ISO 8601: `YYYY-MM-DD`, optionally followed by `T`
  or a space and `HH:MM`, `HH:MM:SS` or `HH:MM:SS` with 1 to 9 fraction
  digits (cut to milliseconds), and after a time optionally by `Z` or
  `±HH:MM`. Without a zone it is in notesd's local time zone; a
  date alone is midnight. `today()` is today's local midnight; `now()` is
  the time of the call. `date()` takes the same forms; another argument is
  `invalidParams`. (ADR-0037; the forms: choice)
- **Durations.** A date plus or minus a string such as `"1 week"` is a
  date. The string is a whole number and a unit: `y`, `year`, `years`,
  `M`, `month`, `months`, `w`, `week`, `weeks`, `d`, `day`, `days`, `h`,
  `hour`, `hours`, `m`, `minute`, `minutes`, `s`, `second`, `seconds`, with
  an optional space between. Years, months, weeks and days add calendar
  units in the local time zone (Go's `AddDate`), so a day across a
  daylight-saving change is not 24 hours; hours, minutes and seconds add
  exact time. notesd folds it to a constant; a duration on a property is
  outside the subset. (Obsidian docs; the units: choice)
- **Lists.** A list value never equals, and never orders against, a
  scalar: `==`, `<`, `>`, `<=` and `>=` are false, and `!=` is true.
  (choice)
- **A bare reference** as a statement (`done`, `!archived`) is true when
  the value is `true`, a number other than 0, a string that is not empty,
  or a list that is not empty; else false. (choice)

### File properties

| Property | Value | Compares as |
| --- | --- | --- |
| `file.path` | the vault path | text |
| `file.name` | the last path segment, case kept | text |
| `file.basename` | `file.name` without its last extension | text |
| `file.ext` | the text after the last `.` of `file.name`, or empty | text |
| `file.folder` | the path before the last `/`; empty at the vault root | text |
| `file.size` | bytes | number |
| `file.mtime` | the modification time | date |
| `file.tags` | the note's tags as the index spells them, without `#`; empty for an attachment | list |
| `file.links` | the note's link targets as written, each once, in order; empty for an attachment | list |

(ADR-0037; the root folder's text and the tags without `#`: choice.) A
literal of the wrong kind for a file property (`file.size == "big"`) is
`invalidParams`. `this.file.*` is the same property of the `this` file,
folded to a constant. `file.links` takes only `isEmpty()`; use
`file.hasLink` for a link. (choice)

### Functions

- `file.hasTag(t, …)` is true when the note has one of the tags, or a tag
  nested under one, ignoring case; a leading `#` is ignored. (ADR-0037)
- `file.inFolder(f)` is true for a file in folder `f` or below it; `""`
  and `"/"` are the whole vault; case counts. (ADR-0037)
- `file.hasLink(x)` takes a string, `this` or `this.file`. A string
  resolves with `markdown.Resolve`, as a link written in the `this` file;
  with no `this`, from the vault root. When it resolves, a note matches
  that links to the file; else a note matches with an unresolved link of
  the same lower-cased name. (ADR-0037)
- `file.hasProperty(k)` is true when the note's frontmatter has key `k`,
  as written, whatever its value. (ADR-0037)
- `x.contains(s)`: on text, `s` is a substring, case kept; on a list, an
  item equals `s` (a string item exactly, a number item numerically).
  For an untyped property notesd decides per row. (ADR-0037)
- `x.startsWith(s)`, `x.endsWith(s)`: text only, case kept; false on a
  list or a missing value. (Obsidian docs)
- `x.containsAny(a, …)`, `x.containsAll(a, …)`: lists only; false on text
  or a missing value. (Obsidian docs)
- `x.isEmpty()`: true for a missing value, null, `""`, `[]` and `{}`.
  (Obsidian docs)
- These methods apply to note properties and to `file.name`,
  `file.basename`, `file.path`, `file.folder`, `file.ext` (text) and
  `file.tags` (list). (choice)
- A list constant, such as `this.file.tags`, can stand for its items as
  the arguments of `file.hasTag`, `containsAny` and `containsAll`. A
  method on a constant (`"abc".contains("b")`) and a bare date as a
  statement are `invalidParams`. (choice)

### `this`

- With `from`, `this` is that note. With `path` and no `from`, `this` is
  the base file. With `source` and no `from`, there is no `this`, and a
  statement that uses it is `invalidParams`. (Obsidian docs; ADR-0037)

### Columns, sort, groups and limit

- `order` lists the columns. Without it, the one column is `file.name`.
  (choice)
- A column's `displayName` comes from `properties`, matched by the key
  with a bare `KEY` read as `note.KEY`; else it is the key without a
  leading `note.`. (Obsidian docs; the default: choice)
- A column of a note property gives its JSON value as the frontmatter
  holds it; a file property gives its value from the table above, with
  `file.mtime` as RFC 3339 text in UTC with milliseconds
  (`2026-11-16T01:02:03.000Z`). `file.links` leaves out a link to a
  heading of the note itself (`[[#Heading]]`), whose target is empty. A `formula.*` column, an unknown
  `file.*` column and `this.*` are unshown: their values are null and
  `unshown` names them. (ADR-0037)
- `sort` is a list of `{property, direction}`, `direction` `ASC` (the
  default) or `DESC`. `groupBy` is `{property, direction}`. Their
  property is a note property or a file property of the table above, but
  not `file.tags` or `file.links`; any other property is `invalidParams`,
  because a wrong order with a limit gives wrong rows. (ADR-0037; the
  refusal: choice)
- Order: the group key first, then each sort key, then `file.path`
  ascending. A missing or null value sorts last in either direction.
  Values of different JSON types order as numbers, then strings, then
  booleans, then lists and objects; strings order by code point; lists
  and objects order by their JSON text, so equal values sit together. A
  JSON null sorts as a missing value. A `date` or `datetime` property
  sorts by its text as written, which orders ISO 8601 dates correctly
  when they share a form and a zone. (ADR-0037; the type order and dates:
  choice)
- A group's value is the property's JSON value; the files without it form
  the last group, with value null. (choice)
- The limit is the smaller of the view's `limit` and the param `limit`;
  it counts rows across all groups. `total` counts every matching file.
  (ADR-0037)

### SQL

- notesd compiles the filters to one condition over `files LEFT JOIN
  notes` for the vault. Every property key and literal is a bound
  parameter; no text of a base reaches the SQL text. (ADR-0037)
- Property keys match with `json_each(notes.props)` and `key = ?`, never a
  JSON path built from the key. List items and text are read from
  `json_each` with their JSON type, not from `props` rows, because a
  `props` row does not keep the difference between `5` and `"5"`. Text
  functions use `instr()` and `substr()`, never `LIKE`. (ADR-0037; the
  `props` rows: choice)
- Each compiled statement is a 0 or 1 value, never NULL, so `!`, `not`
  and `||` keep every row. (ADR-0037)
- Dates convert in a SQL function that notesd registers, which reads the
  forms above in the local time zone. (choice)

## Fixtures

`internal/query/testdata/vault/` holds a fixture vault of made-up notes
and attachments, with `.obsidian/types.json`. The table tests in
`internal/query` index it with notesd's own scan, run each statement and
base against it, and compare the rows with the expected rows they hold. Each statement form, operator, function
and the rules above for missing values, values that do not convert and
attachment rows has a row there. A refusal test covers each feature
outside the subset.

## References

- Rationale: [ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md),
  [ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md),
  [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)
- Context: [design/vault.md — Queries](../design/vault.md#queries),
  [specs/mcp.md — Queries](mcp.md#queries) (`query_notes`),
  [specs/rpc-api.md](rpc-api.md) (`note.query`, generated),
  [specs/markdown.md — link resolution](markdown.md#link-resolution)
- Plan: [plan 0010](../plans/0010-m5-queries.md)
