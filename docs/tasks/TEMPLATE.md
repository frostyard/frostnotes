---
id: "NNNN"
title: Imperative title, under 60 characters
milestone: M0
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - path/of/file/to/create/or/edit.go
given:
  - path/of/contract_test.go # copied from NNNN-slug/_given/ when the task starts
acceptance: go test ./path -run 'TestName' -count=1
---
# T-NNNN: Imperative title

## Goal

Two to four sentences: what exists afterwards and why it matters.

## Read first

- Each file the executor must read, and what to look for in it.
- docs/tasks/EXECUTOR.md

## Contract

Exact signatures, file names, output formats and error behavior. Anything the
given tests check must be stated here in words too.

## Tests (given, do not edit)

The given files and their test names.

## Gotchas

Traps a model is likely to fall into for this task.

## Out of scope

What not to do, even if it looks related.

## Done when

`make accept T=NNNN` passes, `make check` passes, and only the files under
touch changed.
