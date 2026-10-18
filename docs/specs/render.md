# Spec: rendering notes

The HTML notesd renders a note into for the reading view and embeds
(`internal/render`, the `note.render` method): goldmark over the tree
the index reads, Obsidian's syntax drawn as below, then sanitized by an
allowlist. The app sanitizes again with DOMPurify under the same
allowlist and fills in what the HTML names but does not load
([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md),
[ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)).
The fixtures at the end are run by `internal/render`'s tests.

## Interface

```go
package render

type Options struct {
	Path    string                              // the note's vault path
	Resolve func(from, target string) string    // the path a link target leads to from the note at from; "" unresolved
	File    func(path string) (hash string, ok bool) // a vault file's content hash
	Note    func(path string) ([]byte, bool)    // an embedded note's content; false for one over MaxEmbedBytes
}

const MaxEmbedDepth = 3
const (
	MaxEmbeds     = 100     // note embeds one render draws, every level together
	MaxEmbedBytes = 4 << 20 // their notes' bytes, together
)

func Render(note []byte, opts Options) string // Sanitize(HTML(note, opts)): note.render's html
func HTML(note []byte, opts Options) string   // before sanitizing: what the fixtures pin
func Sanitize(html string) string             // the allowlist below
```

`HTML` is goldmark (`markdown.Goldmark`: CommonMark, GFM, footnotes and
the Obsidian extension of [markdown.md](markdown.md)) with the
extension `internal/render` adds: `render.go` (the planner's: source,
embeds' depth), `links.go` (task card T-0045) and `syntax.go` (T-0046).

## Rules

### Source

- The frontmatter is not rendered: the properties block shows it. Lines
  keep their numbers: `data-line` counts the note's lines, frontmatter
  included.
- Each `%%…%%` comment outside code is removed, keeping its newlines, so
  a comment between paragraphs leaves them apart and nothing it hid
  shows; an unclosed one runs to the end, as in Obsidian.
- A single newline inside a paragraph is a line break (`<br>`): Obsidian's
  Strict line breaks is off by default, as it is in the user's vault.
- Raw HTML is passed to the sanitizer, which decides what stays.
- A table column's alignment is an `align` attribute on its cells, not
  a style, which the sanitizer drops.

### Links and embeds

- **A wikilink** `[[target#subpath|alias]]` is `<a class="internal-link"
  data-href="target#subpath" data-path="P">`, where `P` is the vault path
  `Resolve` gives; unresolved, its class is `internal-link is-unresolved`
  and it has no `data-path`. Its text is the alias, else the target, then
  " > " and the subpath without its `#` (`Plan > Goals`, `Plan > ^b1`); a
  link within the note (`[[#Tasks]]`) shows the subpath alone and
  resolves to the note itself.
- **A markdown link** whose destination has no scheme and does not start
  with `//` names a vault file or a heading of this note: an internal
  link as above, its `data-href` the destination percent-decoded, its
  target the part before `#`. Any other link is `<a class="external-link"
  href="…" rel="noopener noreferrer">`, with its title.
- **An embed** `![[target#subpath|alias]]` of a note:
  - **alone in its paragraph** (other note embeds and line breaks
    aside), each such embed replaces the paragraph as `<div class="embed"
    data-href data-path>` holding the note rendered by these rules, or
    with a heading subpath only its section: the heading's line to the
    line before the next heading of its level or above. A block subpath
    embeds the whole note in M3. Embeds nest at most `MaxEmbedDepth`
    levels below the rendered note; one deeper, one of a note already
    being rendered (a cycle), one past the render's budget (`MaxEmbeds`
    note embeds, or `MaxEmbedBytes` of their notes, every level
    together), or one of a note larger than that, is a paragraph holding
    an internal link to it, so a few notes that embed each other many
    times over render in bounded time and size. An embedded note names
    no lines.
  - **with other text in its paragraph**, it is an internal link: HTML
    cannot nest a note's blocks in a paragraph.
- **An embed of another vault file**, by its extension (Obsidian's
  types, as `app/src/lib/fileTree.ts` lists them), carries `data-blob`,
  its hash and extension (`<hash>.png`), from which the app makes its URL,
  and `data-path`; never a URL:
  - an image: `<img class="embed" data-blob data-path alt>`; an alias of
    digits (`|300`) or digits, `x` and digits (`|300x200`) sets
    `width` (and `height`) and the alt is the file name; another alias is
    the alt;
  - audio or video: `<audio class="embed" controls="" data-blob
    data-path></audio>`, or `<video …>`;
  - anything else: `<a class="file-embed" data-blob data-path>` with the
    alias, else the file name, as its text.
- **A markdown image** of a vault file is an embed of it as above, its
  alt the image's text; a remote image stays `<img src alt>`.
- **An unresolved embed**, a file neither `File` nor `Note` knows, is
  `<span class="embed is-unresolved" data-href>` with the link as its
  text. Whether a note exists is asked of `File` first, which does not
  read it.

### Obsidian's syntax

- **A tag** is `<a class="tag" data-tag="name">#name</a>`.
- **A highlight** `==text==` is `<mark>`, with `=` delimiters run as GFM
  runs `~~`: exactly two, not touching another `=`.
- **Math** is `<span class="math" data-tex="…">` inline and `<div
  class="math" data-tex="…">` for a `$$` block, the TeX without its
  delimiters; the app renders it with KaTeX.
- **A callout** is a blockquote whose first line is `[!type]`, then
  optionally `+` or `-` and a title: `<div class="callout"
  data-callout="type"` (lower-cased), with `data-fold` for `+` or `-`,
  holding `<div class="callout-title">` (the title as plain text, else
  the type with its first letter upper-cased) and `<div
  class="callout-content">` (the rest of the quote).
- **A task's checkbox** is `<input type="checkbox" disabled=""`, with
  `checked=""` when done, and `data-line`, the note's line of its list
  item, so the app toggles that line. GFM's tasks only: `[ ]`, `[x]`,
  `[X]`.
- **A heading** carries `data-line`, its line in the note, so the app
  can scroll to it.
- **A block ID** `^id` (as [markdown.md](markdown.md#block-ids) finds it)
  leaves the text and becomes `data-block="id"` on its paragraph, its
  list item, or for a lone `^id` paragraph the list, quote or table
  before it.
- In an embedded note nothing carries `data-line`.

### Sanitizer

`Sanitize` (M3 Phase 4) parses the HTML and keeps only:

| Elements | Attributes besides the shared ones |
| --- | --- |
| `p`, `br`, `hr`, `h1`–`h6`, `blockquote`, `pre`, `code`, `em`, `i`, `strong`, `b`, `del`, `s`, `u`, `ins`, `mark`, `sup`, `sub`, `small`, `kbd`, `abbr`, `q`, `cite`, `span`, `div`, `ul`, `ol`, `li`, `dl`, `dt`, `dd`, `table`, `thead`, `tbody`, `tfoot`, `tr`, `details`, `summary`, `figure`, `figcaption` | `ol`: `start`; `li`: `value` |
| `th`, `td` | `align`, `colspan`, `rowspan` |
| `a` | `href` (`http`, `https`, `mailto`, or `#…`), `rel` |
| `img` | `src` (`http`, `https`), `alt`, `width`, `height` |
| `audio`, `video` | `controls` |
| `input` | `type` (`checkbox` only), `checked`, `disabled` |

- **Shared attributes,** on any kept element: `title`, `lang`, `dir`,
  and `data-href`, `data-path`, `data-blob`, `data-tag`, `data-tex`,
  `data-callout`, `data-fold`, `data-line`, `data-block`; and three that
  keep only what the renderer writes, so a note cannot dress its HTML in
  the app's own styles or name a window property: `class` keeps
  `internal-link`, `external-link`, `is-unresolved`, `embed`,
  `file-embed`, `tag`, `math`, `callout`, `callout-title`,
  `callout-content`, `footnotes`, `footnote-ref`, `footnote-backref` and
  `language-…` (an attribute with none of them goes); `id` stays when it
  starts `fn:` or `fnref`, a footnote's; `role` stays when it is
  `doc-noteref`, `doc-endnotes` or `doc-backlink`.
- Every other attribute goes: `style`, every `on…` handler, `srcset`.
  A URL attribute whose scheme is not listed goes.
- `script`, `style`, `template`, `iframe`, `frame`, `frameset`,
  `object`, `embed`, `applet`, `param`, `portal`, `form` and its fields
  (`input` but a checkbox, `button`, `select`, `option`, `optgroup`,
  `datalist`, `textarea`), `link`, `meta`, `base`, `head`, `title`,
  `svg`, `math`, `noscript`, `noembed`, `noframes`, `xmp` and
  `plaintext` go with their content, an unclosed one with the rest of
  the HTML; any other element not listed goes and its content stays.
  Written self-closing, they go with their content all the same, as a
  browser ignores the slash of an HTML element that is not void
  (`<xmp/>` opens one); only `<svg/>` and `<math/>` are empty.
  Comments and doctypes go.
- It reads the HTML with `golang.org/x/net/html`'s tokenizer, as a
  browser does. Text passes as written, entities included, but a `<`
  is written `&lt;`, so that text joined across a dropped tag
  (`<<x>script>`) cannot open one; attribute
  values are written again, escaped; a self-closing tag is written open
  (`<br>`). A URL is read as a browser reads its scheme, without white
  space or control characters (`jav&#x09;ascript:` is `javascript:`),
  and one with no scheme goes: the app names vault files by `data-blob`
  and `data-path`, never by URL.
- Every fixture's HTML below sanitizes to itself, but R02's `class`
  and `onclick`.

## Fixture vault

Every fixture renders `Notes/Today.md` in this vault. Notes have no hash
here; other files have the hash shown.

| Path | Hash |
| --- | --- |
| `Notes/Today.md` | note |
| `Notes/Plan.md` | note |
| `Notes/Loop.md` | note |
| `Notes/Deep1.md` | note |
| `Notes/Deep2.md` | note |
| `Notes/Deep3.md` | note |
| `Notes/Deep4.md` | note |
| `Media/map.png` | `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa` |
| `Media/song.mp3` | `bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` |
| `Media/clip.webm` | `cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc` |
| `Media/scan.pdf` | `dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd` |
| `Media/data.csv` | `eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee` |

#### `Notes/Plan.md`

```md
---
status: done
---
# Plan

Rows of quince.

## Goals

More pears.

## Later

Cider.
```

#### `Notes/Loop.md`

```md
Loop embeds itself:

![[Loop]]
```

#### `Notes/Deep1.md`

```md
One

![[Deep2]]
```

#### `Notes/Deep2.md`

```md
Two

![[Deep3]]
```

#### `Notes/Deep3.md`

```md
Three

![[Deep4]]
```

#### `Notes/Deep4.md`

```md
Four
```

## Fixtures

Each fixture is a note and the HTML `HTML` returns for it, exactly. The
tag in brackets says which card's code it needs: `base` the planner's,
`links` T-0045's, `syntax` T-0046's (and T-0045's).

### R01 — Frontmatter, line breaks and comments [base]

```md
---
title: x
---
First line
second line.

New %%hidden%% paragraph.
%%
block
%%
After.
```

```html
<p>First line<br>
second line.</p>
<p>New  paragraph.</p>
<p>After.</p>
```

### R02 — Raw HTML, left to the sanitizer [base]

```md
Raw <span class="x" onclick="bad()">html</span> stays.

<div>block</div>
```

```html
<p>Raw <span class="x" onclick="bad()">html</span> stays.</p>
<div>block</div>
```

### R03 — Wikilinks [links]

```md
See [[Plan]], [[Plan#Goals]], [[Plan#^b1|that block]], [[#Tasks]] and [[Missing]].
```

```html
<p>See <a class="internal-link" data-href="Plan" data-path="Notes/Plan.md">Plan</a>, <a class="internal-link" data-href="Plan#Goals" data-path="Notes/Plan.md">Plan &gt; Goals</a>, <a class="internal-link" data-href="Plan#^b1" data-path="Notes/Plan.md">that block</a>, <a class="internal-link" data-href="#Tasks" data-path="Notes/Today.md">Tasks</a> and <a class="internal-link is-unresolved" data-href="Missing">Missing</a>.</p>
```

### R04 — Markdown links [links]

```md
[Plan](Plan.md#Goals), [web](https://example.test/a?b=1&c=2 "Title"), [mail](mailto:a@example.test) and [heading](#Tasks).
```

```html
<p><a class="internal-link" data-href="Plan.md#Goals" data-path="Notes/Plan.md">Plan</a>, <a class="external-link" href="https://example.test/a?b=1&amp;c=2" title="Title" rel="noopener noreferrer">web</a>, <a class="external-link" href="mailto:a@example.test" rel="noopener noreferrer">mail</a> and <a class="internal-link" data-href="#Tasks" data-path="Notes/Today.md">heading</a>.</p>
```

### R05 — A note embed [links]

```md
![[Plan]]
```

```html
<div class="embed" data-href="Plan" data-path="Notes/Plan.md">
<h1>Plan</h1>
<p>Rows of quince.</p>
<h2>Goals</h2>
<p>More pears.</p>
<h2>Later</h2>
<p>Cider.</p>
</div>
```

### R06 — Note embeds in text and on their own lines [links]

```md
Text ![[Plan]] inline, and two on their lines:

![[Plan#Later]]
![[Deep4]]
```

```html
<p>Text <a class="internal-link" data-href="Plan" data-path="Notes/Plan.md">Plan</a> inline, and two on their lines:</p>
<div class="embed" data-href="Plan#Later" data-path="Notes/Plan.md">
<h2>Later</h2>
<p>Cider.</p>
</div>
<div class="embed" data-href="Deep4" data-path="Notes/Deep4.md">
<p>Four</p>
</div>
```

### R07 — Sections and unresolved embeds [links]

```md
![[Plan#Goals]]

![[Missing]]
```

```html
<div class="embed" data-href="Plan#Goals" data-path="Notes/Plan.md">
<h2>Goals</h2>
<p>More pears.</p>
</div>
<p><span class="embed is-unresolved" data-href="Missing">Missing</span></p>
```

### R08 — Cycles and depth [links]

```md
![[Loop]]

![[Deep1]]
```

```html
<div class="embed" data-href="Loop" data-path="Notes/Loop.md">
<p>Loop embeds itself:</p>
<p><a class="internal-link" data-href="Loop" data-path="Notes/Loop.md">Loop</a></p>
</div>
<div class="embed" data-href="Deep1" data-path="Notes/Deep1.md">
<p>One</p>
<div class="embed" data-href="Deep2" data-path="Notes/Deep2.md">
<p>Two</p>
<div class="embed" data-href="Deep3" data-path="Notes/Deep3.md">
<p>Three</p>
<p><a class="internal-link" data-href="Deep4" data-path="Notes/Deep4.md">Deep4</a></p>
</div>
</div>
</div>
```

### R09 — Files [links]

```md
![[map.png]] ![[map.png|300]] ![[map.png|300x200]] ![[map.png|A map]]

![[song.mp3]]

![[clip.webm]]

![[scan.pdf]] ![[data.csv|the data]]
```

```html
<p><img class="embed" data-blob="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png" data-path="Media/map.png" alt="map.png"> <img class="embed" data-blob="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png" data-path="Media/map.png" alt="map.png" width="300"> <img class="embed" data-blob="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png" data-path="Media/map.png" alt="map.png" width="300" height="200"> <img class="embed" data-blob="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png" data-path="Media/map.png" alt="A map"></p>
<p><audio class="embed" controls="" data-blob="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.mp3" data-path="Media/song.mp3"></audio></p>
<p><video class="embed" controls="" data-blob="cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.webm" data-path="Media/clip.webm"></video></p>
<p><a class="file-embed" data-blob="dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd.pdf" data-path="Media/scan.pdf">scan.pdf</a> <a class="file-embed" data-blob="eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.csv" data-path="Media/data.csv">the data</a></p>
```

### R10 — Markdown images [links]

```md
![A map](Media/map.png) ![remote](https://example.test/p.png) ![gone](nowhere.png)
```

```html
<p><img class="embed" data-blob="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png" data-path="Media/map.png" alt="A map"> <img src="https://example.test/p.png" alt="remote"> <span class="embed is-unresolved" data-href="nowhere.png">nowhere.png</span></p>
```

### R11 — Tables and footnotes [links]

```md
| a | b |
| :- | - |
| 1 | [[Plan]] |

Footnote[^1].

[^1]: The note.
```

```html
<table>
<thead>
<tr>
<th align="left">a</th>
<th>b</th>
</tr>
</thead>
<tbody>
<tr>
<td align="left">1</td>
<td><a class="internal-link" data-href="Plan" data-path="Notes/Plan.md">Plan</a></td>
</tr>
</tbody>
</table>
<p>Footnote<sup id="fnref:1"><a href="#fn:1" class="footnote-ref" role="doc-noteref">1</a></sup>.</p>
<div class="footnotes" role="doc-endnotes">
<hr>
<ol>
<li id="fn:1">
<p>The note.&#160;<a href="#fnref:1" class="footnote-backref" role="doc-backlink">&#x21a9;&#xfe0e;</a></p>
</li>
</ol>
</div>
```

### R12 — Tags [syntax]

```md
Tags #orchard and #area/work, not#this, `#code` and #2026.
```

```html
<p>Tags <a class="tag" data-tag="orchard">#orchard</a> and <a class="tag" data-tag="area/work">#area/work</a>, not#this, <code>#code</code> and #2026.</p>
```

### R13 — Highlights [syntax]

```md
A ==highlight== and ===not=== and == spaced ==.
```

```html
<p>A <mark>highlight</mark> and ===not=== and == spaced ==.</p>
```

### R14 — Math [syntax]

```md
Inline $x^2$ and

$$
\frac{a}{b}
$$
```

```html
<p>Inline <span class="math" data-tex="x^2"></span> and</p>
<div class="math" data-tex="\frac{a}{b}"></div>
```

### R15 — Callouts [syntax]

```md
> [!note]
> Plain note.

> [!WARNING]- Mind the frost
> Cover the beds.
>
> Twice.

> [!tip]+ Title only

> Just a quote.
```

```html
<div class="callout" data-callout="note">
<div class="callout-title">Note</div>
<div class="callout-content">
<p>Plain note.</p>
</div>
</div>
<div class="callout" data-callout="warning" data-fold="-">
<div class="callout-title">Mind the frost</div>
<div class="callout-content">
<p>Cover the beds.</p>
<p>Twice.</p>
</div>
</div>
<div class="callout" data-callout="tip" data-fold="+">
<div class="callout-title">Title only</div>
<div class="callout-content">
</div>
</div>
<blockquote>
<p>Just a quote.</p>
</blockquote>
```

### R16 — A callout holding an embed [syntax]

```md
> [!info] Embedded
> ![[Plan#Later]]
```

```html
<div class="callout" data-callout="info">
<div class="callout-title">Embedded</div>
<div class="callout-content">
<div class="embed" data-href="Plan#Later" data-path="Notes/Plan.md">
<h2>Later</h2>
<p>Cider.</p>
</div>
</div>
</div>
```

### R17 — Headings and tasks carry their lines [syntax]

```md
---
a: 1
---
# Heading

- [ ] open
- [x] done
- plain

Setext
======
```

```html
<h1 data-line="4">Heading</h1>
<ul>
<li><input type="checkbox" disabled="" data-line="6"> open</li>
<li><input type="checkbox" disabled="" checked="" data-line="7"> done</li>
<li>plain</li>
</ul>
<h1 data-line="10">Setext</h1>
```

### R18 — Block IDs [syntax]

```md
A paragraph ^p1

- item one ^li1
- item two

> quoted

^q1

Text
^cont
```

```html
<p data-block="p1">A paragraph</p>
<ul>
<li data-block="li1">item one</li>
<li>item two</li>
</ul>
<blockquote data-block="q1"><p>quoted</p>
</blockquote>
<p data-block="cont">Text</p>
```

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `internal/render`'s fixture tests | read the fixture vault and fixtures from this file |
| the app's DOMPurify configuration (M3 Phase 5) | the sanitizer's allowlist |

## References

- Rationale: [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md) (notesd renders, both sides sanitize), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md) (files by hash), [ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md) (comments)
- Context: [design/app.md](../design/app.md#reading-view), [design/vault.md](../design/vault.md#index) (what `note.render` reads); [markdown.md](markdown.md) (the syntax), [rpc-api.md](rpc-api.md) (`note.render`)
- Built in: [plan 0005 — M3](../plans/0005-m3-desktop-app.md), Phases 3 and 4
