---
id: "0003"
title: Add notesctl vault list, add and rm
milestone: M0
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - cmd/notesctl/vault.go
  - cmd/notesctl/root.go
given:
  - cmd/notesctl/vault_test.go # copied from 0003-notesctl-vault/_given/ when the task starts
acceptance: go test ./cmd/notesctl -run 'TestVaultCommand' -count=1
---
# T-0003: Add notesctl vault list, add and rm

## Goal

`notesctl vault` lists, registers and forgets the vaults notesd owns,
over the existing `vault.list`, `vault.add` and `vault.remove` methods.
It is the first notesctl command over a real domain and completes M0
Phase 3's check (`notesctl hello` and `notesctl vault add/list` against a
running notesd). This is an M0 calibration card for the executor (plan
0001, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- cmd/notesctl/root.go, cmd/notesctl/hello.go and hello_test.go: the command
  pattern (`newXxxCmd(opts)`, `opts.dial`, `clix.OutputJSON`) and the test
  helper `runNotesctl`.
- docs/specs/rpc-api.md, section "vault": the methods, `Vault`, and errors.
- api/zz_generated.go: `VaultClient`, `VaultAddParams`, `VaultRemoveParams`.
- cmd/notesctl/vault_test.go (given).

## Contract

`cmd/notesctl/vault.go` defines `func newVaultCmd(opts *rootOptions) *cobra.Command`,
and `root.go` adds it to the root command after `newHelloCmd(opts)`.

- `notesctl vault list` (alias `ls`), no arguments: one line per vault in
  the order `vault.list` returns, `ID<TAB>NAME<TAB>PATH`, or `no vaults`
  when there are none. With `--json`, the `[]api.Vault` result through
  `clix.OutputJSON` and nothing else.
- `notesctl vault add [--name NAME] PATH`, exactly one argument: PATH is
  made absolute with `filepath.Abs` (relative to the working directory)
  before `vault.add`; `--name` is sent only when the flag was given. Prints
  `added vault ID "NAME" at PATH` (NAME quoted with `%q`, PATH as notesd
  returned it). With `--json`, the `api.Vault` result only.
- `notesctl vault rm ID`, exactly one argument: ID must parse as a base-10
  int64, else the error `vault ID "x" is not a number`. Prints
  `removed vault ID`.
- notesd's errors are returned unchanged (for example
  `invalidParams: vault path … is not a directory`, `notFound: vault 99 does
  not exist`).

## Tests (given, do not edit)

`cmd/notesctl/vault_test.go`: `TestVaultCommands` and
`TestVaultCommandErrors`. They use `runNotesctl` from `hello_test.go` and a
real notesd from `internal/rpctest`.

## Gotchas

- Dial inside each subcommand's `RunE`, after argument checks, and close the
  client with `defer c.Close()`.
- `cobra.ExactArgs(1)` produces the "accepts 1 arg(s)" error the tests
  expect.
- Write with `fmt.Fprintf(cmd.OutOrStdout(), …)`, as hello does.

## Out of scope

Any change to notesd, the schema, the engine, or other commands.

## Done when

`make accept T=0003` passes, `make check` passes, and only the files under
touch changed.
