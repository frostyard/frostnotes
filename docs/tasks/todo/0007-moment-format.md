---
id: "0007"
title: Format dates with moment.js tokens
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/moment/format.go
given:
  - internal/moment/format_test.go # copied from 0007-moment-format/_given/ when the task starts
acceptance: go test ./internal/moment -count=1
---
# T-0007: Format dates with moment.js tokens

## Goal

Add `moment.Format`, which renders a time with a moment.js format string.
Obsidian names daily notes with one (`YYYY-MM-DD` by default, or folders
such as `YYYY/MM/YYYY-MM-DD`) and fills `{{date:…}}` in templates with
another, so notesd needs the same formatter to find today's daily note for
`append_to_daily_note` and, later, to expand templates
(docs/design/vault.md#daily-notes).

## Read first

- docs/tasks/EXECUTOR.md
- docs/design/vault.md, section "Daily notes".
- internal/moment/format_test.go (given).

## Contract

New package `internal/moment`, one file `format.go`, with the package doc
comment `// Package moment formats dates with moment.js tokens, as
Obsidian's daily notes and templates write them
(docs/design/vault.md#daily-notes).`

```go
// Format renders t, in its own location, with a moment.js format string.
func Format(layout string, t time.Time) string
```

Scan `layout` left to right. At each position:

1. `[` starts an escape when a `]` follows with no `[` between them: copy
   the text between the brackets and continue after the `]`. Otherwise the
   `[` is copied.
2. Otherwise, the longest token in the table that starts here is replaced
   by its value.
3. Otherwise copy one byte. (Tokens are ASCII, so multi-byte characters
   are copied whole.)

Letters that are tokens are tokens wherever they appear, as in moment.js:
`Daily` renders as `16amily` (`D`, `a`), which is why formats escape
words (`[Daily]/YYYY-MM-DD`).

| Token | Value | Token | Value |
| --- | --- | --- | --- |
| `YYYY` | year, at least 4 digits (`0987`) | `YY` | last two digits of the year |
| `Q` / `Qo` | quarter 1–4 / `4th` | `M` / `Mo` / `MM` | month `1`–`12` / `10th` / `01`–`12` |
| `MMM` / `MMMM` | `Oct` / `October` | `D` / `Do` / `DD` | day of month / `16th` / `01`–`31` |
| `DDD` / `DDDo` / `DDDD` | day of year / `289th` / `001`–`366` | `d` / `do` / `e` | weekday, Sunday `0` / `5th` / same as `d` |
| `dd` / `ddd` / `dddd` | `Fr` / `Fri` / `Friday` | `E` | ISO weekday, Monday `1` to Sunday `7` |
| `w` / `wo` / `ww` | locale week / `42nd` / two digits | `gggg` | locale week-year, 4 digits |
| `W` / `Wo` / `WW` | ISO week / `42nd` / two digits | `GGGG` | ISO week-year, 4 digits |
| `H` / `HH` | hour `0`–`23` / two digits | `h` / `hh` | hour `1`–`12` / two digits |
| `k` / `kk` | hour `1`–`24` (midnight is 24) / two digits | `m` / `mm`, `s` / `ss` | minute, second / two digits |
| `A` / `a` | `AM` or `PM` / `am` or `pm` (noon is PM) | `X` / `x` | Unix seconds / milliseconds |
| `Z` / `ZZ` | offset `-04:00` / `-0400` | | |

- Names are English. Ordinals add `st`, `nd`, `rd` or `th`; 11, 12 and 13
  (and 111, 112, 113) take `th`.
- **Locale weeks** are moment.js's default English locale: weeks start on
  Sunday and week 1 is the week holding January 1st. A week belongs to the
  year of its Saturday: `gggg` is that year, and `w` is
  `(saturday.YearDay()-1)/7 + 1`.
- **ISO weeks** are `time.Time.ISOWeek`.
- Tokens not in the table (`y`, `N`, `S`, `zz` and others) are not
  supported: their letters are copied, or read as the tokens they contain.

## Tests (given, do not edit)

`internal/moment/format_test.go`: `TestFormat` (20 cases),
`TestOrdinals`, `TestWeeksAtTheTurnOfTheYear`. The week numbers were
computed with moment.js's own week algorithm, independently of any Go
code.

## Gotchas

- Longest match first: `DDDD` before `DDDo` before `DDD` before `Do` and
  `DD` before `D`; `MMMM` before `MMM` before `Mo` and `MM`.
- Format in `t`'s location; never call `t.Local()` or `t.UTC()`.
- `Z` for UTC is `+00:00`; an offset of +05:30 keeps its minutes.
- Midnight is `12 AM` for `h` and `24` for `k`; noon is `12 PM`.
- Do not use Go's `time.Format` layouts for the whole string: moment's
  tokens are different letters.

## Out of scope

Parsing dates, template expansion (`{{date}}`, `{{title}}`), locales other
than English, and every file except `internal/moment/format.go`.

## Done when

`make accept T=0007` passes, `make check` passes, and only the files under
touch changed.
