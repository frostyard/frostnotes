# 0035 — Unit runs filter no test names

- **Status:** Accepted
- **Date:** 2026-11-03

## Context

Until 2026-11-03, `make test`, `make ci` and CI's race job ran `go test`
with `-run '^Test[^I]'`, from core ADR-0022 and
[ADR-0009](0009-development-environment-and-gates.md). The filter was meant
to skip integration tests named `TestI…`. But `go test` applies `-run` to
fuzz targets and examples too, so the filter also skipped the seed corpora
of `FuzzParseScope` (`api/scope_test.go`) and `FuzzSanitize`
(`internal/render/sanitize_test.go`), and would skip any `Example`
function: checks that could never fail. Every integration test here
already sits in a file with the `integration` build tag, so a unit run does
not compile it, and the filter is not needed. Core ADR-0038 superseded
ADR-0022 and limits the name filter to chairlift.

## Decision

Unit and race runs filter no test names. Integration tests carry the
`integration` build tag. Internal repocheck fails when a non-integration
`go test` line in the Makefile or a workflow has a `-run` or `-skip` flag.

## Consequences

The hermetic gate and race run execute every Go test, Fuzz seed corpus and
Example function. Integration tests stay separate from unit runs. The
repository guard catches a future reintroduction of a test-name filter in
unit or race commands.

## Alternatives considered

- **Keep the filter and add a second run for Fuzz and Example:** Rejected.
  This approach keeps unnecessary filtering and adds more gate complexity.

## References

- Shapes: `Makefile` (`test`, `ci`), `.github/workflows/ci.yml` (the race
  job), `internal/repocheck/testfilter_test.go`, `AGENTS.md` (Gates),
  [docs/tasks/EXECUTOR.md](../tasks/EXECUTOR.md)
- Supersedes in part: [ADR-0009](0009-development-environment-and-gates.md)
- Builds on: [core ADR-0038](https://github.com/frostyard/core/blob/main/docs/adr/0038-scope-the-test-name-filter-to-chairlift.md)
