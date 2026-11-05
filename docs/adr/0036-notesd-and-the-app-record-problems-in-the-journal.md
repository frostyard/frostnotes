# 0036 — notesd and the app record problems in the journal

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

A panic of notesd or of the app shell, and a failure that only notesd sees
(a database that does not migrate, a write that does not reach the disk),
left a trace only in a log: notesd's `-log` file, or the shell's standard
error. Nobody reads those logs until something is visibly wrong, and they
hold note paths and text.

frostyard decided how its apps report problems in
[core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md):
local first, opt-in, and one sender per host. An app writes a problem record
to the local systemd journal; chairlift is the only program that can send a
record off the host, and only with the user's consent. The record's fields
are fixed by core's spec
[problem-report.md](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md)
(schema `frostyard.problem/1`, kind S8 `app_problem`). frostyard/std ships
the writer for both of frostnotes' runtimes: the Go package
`github.com/frostyard/std/problems` (std v0.5.0) and the Rust crate
`frostyard-problems` in std's `rust/` (std's
[problems-package.md](https://github.com/frostyard/std/blob/main/docs/specs/problems-package.md)).

## Decision

notesd and the app shell write problem records to the local journal with
those libraries, and send nothing over a network.

- **Identity.** The app ID is `org.frostyard.Frostnotes` for both. notesd's
  component is `daemon` (runtime `go`); the Tauri shell's is `gui` (runtime
  `rust`).
- **Version.** A record carries the release version, a semver without a
  `v`. notesd's is its ldflags version with 1 leading `v` removed (`make
  build` stamps `git describe`; a tag gives `v0.5.0`, the `.deb` and the
  Flatpak stamp `0.5.0`). The shell's is its Tauri `package_info().version`:
  the Flatpak manifest gives `pnpm tauri build --config` the version in
  `FROSTNOTES_VERSION`, which is `0.0.0` until
  `packaging/flatpak/stamp-version.sh` stamps a tag's. A build that is not
  a release (`dev`, `flatpak`, `v0.5.0-3-gabc`, `0.0.0-abc-snapshot`, a
  shell at `0.0.0`) records nothing.
- **Panics.** notesd records a panic of its main goroutine
  (`RecoverPanic` in `runWith`) and of a request's handler (the dispatcher
  that `rpcserver` gives the JSON-RPC kit records it, then the kit answers
  internal as before). A crash of any other goroutine, or a fatal error, is
  in the crash file of `CrashOutput` (under `$STATE_DIRECTORY`, else
  `$XDG_STATE_HOME`, else `~/.local/state`), which the next start turns
  into 1 record. The shell's panic hook records each panic, with its place
  under `src/`, before the hook that was there.
- **Coded errors.** notesd records 5 codes, each at 1 place. Only the code
  is the contract; the error's text stays in the record's `MESSAGE` on the
  host.

  | Code | Meaning | Where |
  | --- | --- | --- |
  | `store.migrate_failed` | The database cannot be opened and migrated: its first transaction fails (the file cannot be opened, or is not a database), a migration fails, or the database is newer than notesd or applied another migration. notesd does not start. | `cmd/notesd`, on `store.Open`'s `*store.MigrateError` |
  | `merge.failed` | The merge library panicked on a merge of the vault. The panic goes on as before. | `internal/vault` (`guardMerge`) |
  | `vault.write_failed` | A write failed for a reason that is not the client's: the blob store, the staged file, its rename, or the index transaction after the rename. A client's error (conflict, invalid params, not found) and a cancelled request are not recorded. | `internal/vault` (`writeFailed`, in `write` and `create`) |
  | `watch.overflow_rescan` | The kernel's inotify queue overflowed: events were lost, and the vault is scanned again. | `internal/vault` (`watchError`) |
  | `sync.protocol_error` | A device's session with a hub ended on a protocol fault: the hub answered parse error, invalid request, method not found, invalid params or protocol mismatch, or sent a reply that does not decode. Once for a run of sessions that end on one. | `internal/syncer` (`protocolFault`, `faultRecord`) |

- **Plumbing.** notesd makes 1 recorder in `runWith` and passes it in the
  packages' configs (`vault.Config.Problems`, `syncer.Config.Problems`,
  `rpcserver.Options.Panics`), never in package state. The packages see it
  as `internal/problem.Recorder`; nil records nothing.
- **Nothing is sent.** A record is 1 datagram to the local journal socket.
  notesd and the shell open no connection for it, and the records stay on
  the host unless chairlift sends them under its own consent.

## Consequences

- A user, or chairlift with consent, finds frostnotes' problems with
  `journalctl MESSAGE_ID=8915bdd215c84cbab0c31c05a1a33e7e`, with no note
  text in any field but `MESSAGE`.
- A new code is a new decision: it needs a row in this ADR's table (or a
  new ADR) and 1 place in the code. Recording an error at many places, or
  recording a client's error, would flood the journal.
- The libraries cap records at 10 a minute for each process, and the
  datagram is dropped when the journal cannot take it at once. A record
  can be lost; notesd never waits for the journal.
- notesd keeps a crash file open in its state folder for its life.
- The shell's Flatpak build now needs `FROSTNOTES_VERSION`, and the stamp
  script edits 1 more line of the manifest. An unset version fails the
  build rather than giving a shell with no version.
- A test of a record binds a datagram socket and passes it with
  `problems.WithSocket` (Go) or `Recorder::with_socket` (Rust); a Go test
  binary stays under the rate cap, or puts the record in a child process.

## Alternatives considered

- **Send reports to a service from notesd or the app:** rejected by core
  ADR-0069: one sender per host, under the user's consent, and notesd
  listens on no network address and dials only its hub.
- **Record every internal error of a request:** rejected. Most are the
  disk or the vault's folder, are already in the reply and the log, and
  would make one code of many causes.
- **Record in each package through a package-level recorder:** rejected:
  frostnotes keeps no mutable package state, and tests need their own
  socket.

## References

- Shapes: [design/overview.md](../design/overview.md#problem-records),
  [design/app.md](../design/app.md#problem-records),
  `README.md` (Problem records), `packaging/flatpak/stamp-version.sh`
- Builds on: [core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md),
  core's [problem-report.md](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md),
  std's [problems-package.md](https://github.com/frostyard/std/blob/main/docs/specs/problems-package.md),
  [ADR-0004](0004-a-notes-daemon-with-thin-clients.md),
  [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
  (the Flatpak and its stamped version)
