---
id: "0009"
title: Make generated vaults realistic, and a vaultgen command
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/vaultgen/vaultgen.go
  - tools/vaultgen/main.go
given:
  - internal/vaultgen/realism_test.go # copied from 0009-vaultgen-command/_given/ when the task starts
  - tools/vaultgen/main_test.go
acceptance: go test ./internal/vaultgen ./tools/vaultgen -count=1
---
# T-0009: Make generated vaults realistic, and a vaultgen command

## Goal

T-0008's vaults pass every pattern test but are not realistic: at 666
notes (and at 5,000) they hold 9 folders, notes of about 300 bytes that
share one paragraph, 2 attachments, and quoted `created` dates, where the
user's vault has 97 folders, notes of about 3.3 KB and unquoted dates
(plan 0003, Phase 2 result). M1's timing, `kill -9`
and search tests need vaults that look like the real one at scale. This
card makes `internal/vaultgen` realistic and adds `tools/vaultgen`, a
command that writes a vault and its manifest for manual runs.

## Read first

- docs/tasks/EXECUTOR.md
- docs/tasks/done/0008-vaultgen.md: every rule of the vault and its
  manifest. They all still hold.
- internal/vaultgen/vaultgen.go and vaultgen_test.go (T-0008): the
  generator you change, and the tests it must keep passing.
- internal/vaultgen/realism_test.go and tools/vaultgen/main_test.go
  (given).
- tools/taskrun/main.go: a command's shape (`run` returns an error; `main`
  prints it).

## Contract

### Realistic vaults (`internal/vaultgen/vaultgen.go`)

`Generate`'s signature, determinism, safety rules, link forms, decoys and
manifest stay as T-0008 defines them. Add:

- **Folders.** Besides T-0008's topic folders (`Garden`, `Orchard`,
  `Orchard/North Block`, `Orchard/North Block/Seed Beds`), make
  `max(4, Notes/9) - 4` more. Name each with a word from one list and a
  word from another (`Upper Meadow`, `Stone Cellar`), adding a counter
  when the pairs run out, so names stay unique ignoring case and never
  equal an existing folder name, a member's name or `media`. About a third
  go in the root; the others go under an earlier folder, at most three
  folders deep. Every new top-level folder gets an `Overview.md`, and
  about a third of them a `media/` folder with one `img-N.png` (N
  continuing the vault's count), which their Overview embeds as
  `![a map](./media/img-N.png)` (T-0008's rule). Notes go one to each
  folder first, then at random, as now.
- **Text.** A topic note gets 3 to 12 paragraphs of made-up sentences
  under its `## Goals` heading, and about one in 40 gets 30 to 39, so a
  vault has some notes over 8 KB. A paragraph is 3 to 7 sentences of 8 to
  20 words drawn from a vocabulary of at least 150 orchard and co-op
  words, on one line; some end with an inline tag. A topic note's first
  paragraph replaces the shared sentence after its title, and "Keep the
  co-op's growing records together." goes. Member meetings get 1 to 4
  paragraphs; some daily notes get a few after `## Log`. Daily notes keep
  their template exactly up to `## Log`.
- **Attachments** grow with the topic folders, as above, and stay sorted
  in the manifest, as T-0008 requires.
- **Dates** in frontmatter are written unquoted, as the user's vault
  writes them: `created: 2026-10-16`. `title` stays quoted.

With these, a 666-note vault has about 90 folders and notes of about
3.4 KB on average.

### The command (`tools/vaultgen/main.go`)

```go
func main()                                 // run(os.Args[1:], os.Stdout); on error print "vaultgen: <err>" to stderr, exit 2
func run(args []string, stdout io.Writer) error
```

Flags (a `flag.FlagSet` named `vaultgen` with `ContinueOnError`, its
output discarded so errors come back from `run`):

| Flag | Default | Meaning |
| --- | --- | --- |
| `-out` | (required) | the folder to write; missing or empty |
| `-notes` | 666 | how many notes |
| `-seed` | 1 | the seed |
| `-manifest` | "" | when set, write the manifest there |

- No `-out`: an error mentioning `-out`. Arguments after the flags: an
  error naming them. Errors from `Generate` (too few notes, a folder that
  is not empty) are returned as they are.
- The manifest is `encoding/json/v2` with `jsontext.WithIndent("  ")` and
  a final newline, written with `os.WriteFile` (0644) after the vault. A
  write error is wrapped as `write manifest: %w`.
- On success, print one line:
  `wrote 40 notes, 3 attachments and 52 links to <out>`.
- The package comment says how to run it:
  `go run ./tools/vaultgen -out DIR [-notes 666] [-seed 1] [-manifest FILE]`.

## Tests (given, do not edit)

- `internal/vaultgen/realism_test.go`: `TestVaultIsRealistic` (666 notes:
  80 to 120 folders, 2,500 to 4,500 bytes per note on average, a largest
  note of 8,000 to 65,536 bytes, at least 6 attachments, at least 4 prose
  lines per topic note, no prose line in more than 2% of topic notes, at
  least 500 `created:` lines, every one an unquoted date) and
  `TestVaultScales` (20, 100 and 3,000 notes: folders, mean size, and at
  least 2 + Notes/150 attachments).
- `tools/vaultgen/main_test.go`: `TestRunWritesTheVaultAndManifest`,
  `TestRunDefaults`, `TestRunErrors`.
- T-0008's `vaultgen_test.go` must keep passing.

## Gotchas

- The realism test calls a *prose line* any line of 40 bytes or more that
  starts with an ASCII letter. A sentence every topic note shares fails
  it, so no fixed sentence of that length may appear in topic notes.
- New folder names must keep T-0008's rule that a full path is never
  ambiguous: unique ignoring case, never a member's name, never `media`.
  Otherwise links that T-0008's tests resolve with `markdown.Resolve` break.
- Draw everything from the one `rand.Rand`, in a fixed order; never range
  over a map while generating.
- A test generates 3,000 notes: build each note with one
  `strings.Builder`, and never scan every note for each link.
- The test at 100 notes allows at most 25 folders: follow the folder
  formula above, since T-0008's fixed folders already make about 10.
- `created: 2026-10-16` unquoted is still valid YAML that T-0008's test
  parses; only `title` needs quotes, because a date-named note's title
  would otherwise read as a date.

## Out of scope

Parsing, the index, timing tests, other link forms, and every file other
than the two under touch.

## Done when

`make accept T=0009` passes, `make check` passes, and only the files under
touch changed.
