---
id: "0039"
title: Match queries as Obsidian's fuzzy search does
milestone: M3
size: S
touch:
  - app/src/lib/fuzzy.ts
given:
  - app/src/lib/fuzzy.test.ts
acceptance: make ui-vitest F=src/lib/fuzzy.test.ts
---
# T-0039: Match queries as Obsidian's fuzzy search does

## Goal

The quick switcher, the command palette and link suggestions rank what
they offer as the user's Obsidian ranks it (`docs/specs/ui.md`,
Prompts). Implement Obsidian 1.14.4's fuzzy search exactly as this card
states it (read from Obsidian's own code): a score and the matched
ranges of a text, the quick switcher's way of matching a path, and a
stable sort by score.

## Read first

- `app/src/lib/fuzzy.ts`: `Range`, `FuzzyMatch`, `FuzzySearch` and the
  stubs.
- `docs/specs/ui.md`, "Prompts: quick switcher and command palette".
- The given test `app/src/lib/fuzzy.test.ts`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the file's
"Task T-0039 …" sentence with a description of the search.

Three character classes, as constant regular expressions (space,
punctuation, CJK):

```ts
/\s/
/[ -⁯⸀-⹿\\'!"#$%&()*+,\-./:;<=>?@[\]^_`{|}~]/
/[ༀ-࿿぀-ヿ㐀-䶿一-鿿豈-﫿ｦ-ﾟ]/
```

**`prepareFuzzySearch(query)`** lower-cases the query once and makes two
lists of parts:

- **tokens:** walk the lower-cased query; a space ends the current
  token (an empty token is dropped); a punctuation or CJK character ends
  the current token and is a token by itself; other characters join the
  current token. `"garden plan"` gives `["garden", "plan"]`, `"a/b"`
  gives `["a", "/", "b"]`, `"日記"` gives `["日", "記"]`.
- **characters:** every character of the lower-cased query except
  `" "`.

The returned function, for a text: when the query is exactly `""`, it
returns `{ score: 0, matches: [] }`. Otherwise it matches the tokens;
when they do not match, the characters; when neither does, it returns
null. Matching a list of parts that is empty fails, so a query of spaces
matches nothing.

**Matching a list of parts** against a text, where `lower` is the text
lower-cased and `single` says the parts are the characters:

1. Start at position 0 with no ranges and a count of 0.
2. For each part in order, find it in `lower` at or after the position
   (`indexOf`); when it is missing, the match fails.
3. Where it was found, at `at`, it is **inside a word** when `at > 0`,
   the text's character there (`c`, in its original case) is neither
   punctuation nor CJK, and either `c` is upper case (`c.toLowerCase()
   !== c`) and so is the character before it (`p.toLowerCase() !== p`),
   or `c` is lower case (`c.toUpperCase() !== c`) and the character
   before it is not punctuation, space or CJK. A part inside a word adds
   1 to the count, except a character part (`single`) found right where
   the previous part ended (`at` equals the position).
4. If the last range ends at or after `at`, it is extended to end at
   `at + part.length`; otherwise a new range `[at, at + part.length]`
   is added.
5. The position becomes `at + part.length`.

The match's score, from its ranges, the query's length (the query as
given, spaces included), the text's length and the count:

```
- max(0, ranges - 1)                                       // gaps
- count / 10                                               // parts inside words
- (last range's end - first range's start + 1 - query length) / 100   // spread
- first range's start / 1000
- text length / 10000
```

**`matchPath(search, path)`** matches the path's last segment (after its
last `/`, or the whole path): when it matches, it returns that score with
each range moved by the length of what precedes the segment; otherwise
it matches the whole path and returns that match with its score less 1;
null when neither matches.

**`sortByScore(items)`** returns a new array, highest score first,
equal scores in their original order; it never changes `items`.

## Tests (given, do not edit)

`app/src/lib/fuzzy.test.ts`: scores and ranges for tokens, characters,
words, acronyms, punctuation, CJK and spaces; no match; ranking; paths;
and the stable sort. Scores are compared to 10 decimal places.

## Gotchas

- Tokens are tried before characters, and the characters only when the
  tokens fail: `"gp"` against "Garden plan" is no token match, then two
  character matches.
- An upper case letter right after a lower case one starts a word
  (`DailyPlan`'s `P`); one right after another upper case letter does not
  (`PDFs`'s `D`). Digits are neither case and never count.
- Ranges merge when a part starts where the last range ends: tokens
  `"a"`, `"/"`, `"b"` in `"a/b.md"` make one range, `[0, 3]`.
- The query's length in the score is the query as given, not trimmed or
  lower-cased; the text's length is the lower-cased text's.

## Out of scope

What each prompt matches against and how it shows matches (later cards),
and every file except `app/src/lib/fuzzy.ts`.

## Done when

`make accept T=0039`, `make check` and `make ui-check` pass, and only
`app/src/lib/fuzzy.ts` changed.
