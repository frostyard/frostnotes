# 0015 — Index what comments hide, as Obsidian does

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Obsidian hides `%%…%%` comments when it renders a note. Whether it indexes
the links inside them was unconfirmed, and so was which file it picks
among several partial-path matches when none is in the linking note's
folder. [specs/markdown.md](../specs/markdown.md) had chosen to drop
everything inside a comment, and to break ties by fewest folders, then
byte order. Plan 0003 left both as an open question for a test vault in
the user's Obsidian, since the user's vault has no comments and no such
ties ([plan 0003](../plans/0003-m1-headless-vault.md#open-questions)).

On 2026-10-16 the user opened a test vault of made-up notes in Obsidian
and recorded, with a read-only console snippet, every link Obsidian
indexed and the file it resolves to:

- A note had one link outside a comment, one inside an inline comment
  and one inside a comment block. Obsidian indexed all three.
- From a note in folder `Z`:
  - `[[Shared]]`, with matches one, two and three folders deep, went to
    the one-folder match.
  - `[[Same]]`, with two matches one folder deep, went to the first in
    byte order, though the other was created first.
  - `[[Root]]`, matching both `Root.md` and `K/Root.md`, went to
    `Root.md`.
  - `[[x/Deep]]`, with suffix matches two and three folders deep, went
    to the two-folder one.

## Decision

- **Comments hide text when a note is rendered, not from the index.**
  Links inside `%%…%%` are indexed like any other (the test vault). So
  are tags, headings, block IDs and tasks inside comments: Obsidian's
  index evidently reads a comment's text as ordinary markdown, and
  dropping only some of it would make an index no Obsidian user sees.
  That part is a choice by analogy, which a later test can overturn.
- **`Words` still counts only text outside comments**: a word count
  describes what the reader sees.
- **The tie rule stands:** a match in the source's folder, then one equal
  in case, then the fewest folders, then the first in byte order. Every tie
  in the test vault went the way the rule says; `Root.md` won by having
  the fewest folders.

## Consequences

- The parser stops skipping comments when it collects links, tags,
  headings, block IDs and tasks; finding comments now only bounds `Words`.
- Fixture F12 and the parser's tests of comments change with the spec.
  The user's 179 links are unaffected, since that vault has no comments.
- Link resolution keeps its code and fixtures; the spec cites the test
  vault as their evidence.

## Alternatives considered

- **Keep dropping comment content:** contradicts what Obsidian indexes,
  so backlinks and the graph would miss links Obsidian shows.
- **Index links in comments, but drop the rest:** follows the evidence to
  the letter, yet splits one piece of text into indexed and hidden parts
  with no evidence that Obsidian does.

## References

- Shapes: [specs/markdown.md](../specs/markdown.md), [design/vault.md](../design/vault.md#parse)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)
- Resolves: [plan 0003's open question](../plans/0003-m1-headless-vault.md#open-questions)
