---
id: "0006"
title: Walk a vault's files with its ignore rules
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/vault/walk.go
  - internal/vault/ignore.go
given:
  - internal/vault/walk_test.go # copied from 0006-walk-vault/_given/ when the task starts
acceptance: go test ./internal/vault -run 'TestWalk|TestMatchIgnore|TestNewIgnoreErrors' -count=1
---
# T-0006: Walk a vault's files with its ignore rules

## Goal

Start `internal/vault` with `Walk`, which lists a vault's notes,
attachments and settings, and `Ignore`, the vault's ignore patterns. The
scan compares this listing with the index to find new, changed, moved and
deleted files (docs/design/vault.md#scan-and-watch), so a listing must be
complete or an error: a folder that cannot be read must never look like a
folder whose notes were deleted.

## Read first

- docs/tasks/EXECUTOR.md
- docs/design/vault.md, sections "On disk" and "Scan and watch".
- docs/specs/markdown.md, section "Notes and files".
- internal/vault/walk_test.go (given).

## Contract

`walk.go` holds the package doc comment:
`// Package vault scans, watches and writes vault directories (docs/design/vault.md).`

```go
const SettingsDir = ".frostnotes"

type Kind int
const (
	KindNote       Kind = iota + 1 // a name ending in ".md" (exactly; "Notes.MD" is an attachment)
	KindAttachment                 // any other file
	KindSettings                   // any file under .frostnotes/
)

type Entry struct {
	Path    string    // slash-separated, relative to the root, no leading "/"
	Kind    Kind
	Size    int64
	ModTime time.Time // from the file's FileInfo, unrounded
}
type Skip struct {
	Path   string
	Reason string // "symlink" or "not a regular file"
}
type Listing struct {
	Files   []Entry // sorted by Path in byte order; never nil
	Skipped []Skip  // sorted by Path in byte order; never nil
}

func Walk(ctx context.Context, root string, ignore *Ignore) (Listing, error)

type Ignore struct{ /* unexported */ }
func NewIgnore(patterns []string) (*Ignore, error)
func (ig *Ignore) Match(path string, dir bool) bool // nil *Ignore matches nothing
```

**Walk** uses `filepath.WalkDir` from `root`, which must be a directory
itself (`os.Lstat`): a missing root, a file or a symlink is an error.

1. Any entry whose name starts with `.` is passed over silently, a folder
   with everything in it, except a **folder** named `.frostnotes` directly
   under the root. Inside `.frostnotes/`, dot-named entries are passed over
   too; every other file there is `KindSettings`. (`.obsidian/`, `.git/`,
   `.trash/`, `.DS_Store`, `Sub/.frostnotes/` and notesd's own temporary
   files all start with `.`.)
2. Outside `.frostnotes/`, an entry `ignore.Match(rel, isDir)` matches is
   passed over silently, a folder with everything in it.
3. Symlinks, to files or folders, inside or outside the vault, are never
   followed: each is a `Skip` with reason `"symlink"`. Other non-regular
   files are a `Skip` with reason `"not a regular file"`.
4. Every error is returned, wrapped with the root (`walk vault %s: %w`):
   one that `WalkDir` reports for an unreadable folder, a failed `Info`,
   and `ctx.Err()`, checked once per entry. Walk never returns a partial
   listing.

**Patterns** are a subset of `.gitignore`'s, matched case-sensitively
against slash-separated vault paths:

- Surrounding spaces are trimmed; empty patterns are skipped.
- A trailing `/` matches only folders. A leading `/`, or a `/` anywhere
  else, anchors the pattern at the root; otherwise it matches a file or
  folder name at any depth.
- Segments are matched with `path.Match` (`*`, `?`, `[…]`, never across
  `/`); a segment that is exactly `**` matches zero or more whole
  segments.
- `Match(path, dir)` is true when the path or any folder above it matches.
  `dir` says whether `path` itself is a folder; every folder above it is
  one.
- `NewIgnore` returns an error that quotes the pattern (`%q`) for a pattern
  starting with `!` (negation is not supported), one that is empty after
  removing slashes, one with an empty segment (`a//b`), and one that
  `path.Match` reports as malformed.

## Tests (given, do not edit)

`internal/vault/walk_test.go`: `TestWalk`, `TestWalkEmptyVault`,
`TestWalkIgnore`, `TestWalkSettingsMustBeAFolder`, `TestWalkErrors`,
`TestWalkFailsOnAnUnreadableFolder`, `TestMatchIgnore` (25 cases),
`TestNewIgnoreErrors`.

## Gotchas

- `WalkDir` visits `a/b.md` before `a-b.md`, but byte order puts `a-b.md`
  (`-` is 0x2d) before `a/b.md` (`/` is 0x2f): sort after walking.
- `WalkDir` reports an unreadable folder by calling your function a second
  time with a non-nil error. Return it; skipping it is the bug this card
  exists to prevent.
- `d.Type()` has `fs.ModeSymlink` for a symlink; `d.IsDir()` is false for a
  symlink to a folder, so check symlinks first.
- `path.Match(pattern, "")` validates a pattern without matching anything.
- Convert with `filepath.ToSlash` once, at the relative path.

## Out of scope

Reading `.frostnotes/vault.yaml`, hashing, the index, watching, and every
file other than the two under touch.

## Done when

`make accept T=0006` passes, `make check` passes, and only the files under
touch changed.
