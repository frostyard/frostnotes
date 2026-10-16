---
id: "0013"
title: Expand template variables as Obsidian does
milestone: M1
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/templates/expand.go
given:
  - internal/templates/expand_test.go # copied from 0013-expand-templates/_given/ when the task starts
acceptance: go test ./internal/templates -count=1
---
# T-0013: Expand template variables as Obsidian does

## Goal

A missing daily note is created from the vault's template, whose
`{{title}}`, `{{date}}` and `{{time}}` variables Obsidian expands
(docs/design/vault.md, "Daily notes"). Daily templates often link
yesterday's note with `[[{{date-1d:YYYY-MM-DD}}]]`. Add
`internal/templates`, which expands them over T-0007's `moment.Format`, so
the planner's daily-note code (plan 0003, Phase 4) and MCP's
`append_to_daily_note` write what Obsidian would.

## Read first

- docs/tasks/EXECUTOR.md
- docs/design/vault.md, section "Daily notes".
- internal/moment/format.go (T-0007): `Format(layout string, t time.Time)`.
- internal/templates/expand_test.go (given).

## Contract

New package `internal/templates`, one file `expand.go`, with the package
doc comment `// Package templates expands the variables of Obsidian's
templates and daily-note templates (docs/design/vault.md#daily-notes).`

```go
type Values struct {
	Title      string    // {{title}}
	Now        time.Time // {{date}} and {{time}}, formatted in its own location
	DateFormat string    // the moment.js format of {{date}}; "" is "YYYY-MM-DD"
	TimeFormat string    // the moment.js format of {{time}}; "" is "HH:mm"
}

// Expand returns tmpl with its variables replaced; every other byte is
// copied as it is.
func Expand(tmpl []byte, v Values) []byte
```

A variable is `{{`, optional spaces or tabs, a name, optional spaces or
tabs, `}}`, on one line. Names ignore case.

- `{{title}}` is `v.Title`. It takes no offset or format.
- `{{date}}` and `{{time}}` are `moment.Format(layout, t)`, where layout
  is the format after a `:`, trimmed of spaces (`{{time:HH:mm:ss}}`: the
  first `:` ends the name, so a format may hold colons), or else
  `v.DateFormat` or `v.TimeFormat` with the defaults above.
- `t` is `v.Now`, moved by an optional offset written right after the
  name: `+` or `-`, one to six digits, and a unit: `y` years, `M` months,
  `w` weeks, `d` days (calendar days, `AddDate`), `h` hours, `m` minutes,
  `s` seconds (`Add`). A month or year offset keeps the day of the month,
  or the month's last day when it has fewer, as moment.js does: January 31
  plus `1M` is February 28.
- Anything else is copied: `{{foo}}`, `{{title:x}}`, `{{title+1d}}`,
  `{{date+1x}}`, `{{ }}`, an unclosed `{{date`, a variable across lines.

One compiled regular expression at package level and
`ReplaceAllFunc` are enough.

## Tests (given, do not edit)

`internal/templates/expand_test.go`: `TestExpand` (17 cases).

## Gotchas

- `time.AddDate(0, 1, 0)` on January 31 gives March 3; moment.js gives
  February 28. Build the first of the target month, then clamp the day.
- `M` and `m` differ: months and minutes, as in moment.js.
- Format in `v.Now`'s own location; never convert to local time or UTC.
- Copy bytes you do not replace exactly: BOMs, CRLF and invalid UTF-8 stay.

## Out of scope

Reading templates and settings from the vault, creating notes, Templater
syntax (`<% %>`), and every file other than `expand.go`.

## Done when

`make accept T=0013` passes, `make check` passes, and only the files under
touch changed.
