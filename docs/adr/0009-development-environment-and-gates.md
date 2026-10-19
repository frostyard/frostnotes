# 0009 — Development environment and gates

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

The host is image-based and cannot install `-dev` packages. notesd is pure
Go. The app needs Node, pnpm, Rust and WebKitGTK headers. Frostmail
settled this split (frostmail ADR-0006) and the org's gate rules
(frostyard/core ADR-0043, ADR-0044). Unlike frostmail, frostnotes needs no
outside test server: sync's other end is another notesd.

## Decision

- **The engine builds and tests on the host.** That covers notesd,
  `notesctl` and every Go test. Go is pinned once, in `go.mod`'s toolchain
  line; golangci-lint is pinned in `mise.toml` with `mise.lock`.
- **The app toolchain lives in the nsl machine `frostnotes`** (Debian 13),
  provisioned by `dev/nsl/provision.sh` with pinned, checksum-verified
  Node, Go and Rust plus Debian's WebKitGTK `-dev` packages. Build output
  and the pnpm store stay in `/var/tmp/frostnotes` inside the machine. A
  machine-local notesd runs beside the app there, because the socket cannot
  cross the VM.
- **Sync tests run in process:** a hub and several devices, each a notesd
  over its own temporary vault, on loopback. No containers.
- **The web app's dev loop** is Vite's dev server against a hub notesd on
  loopback; `VITE_MOCK=1` runs the UI against the mock transport in any
  browser.
- **Gates (core ADR-0044):** `make verify` is credential-free and leaves
  the tree clean (tidy, `gen-check`, vet, gofmt, lint, unit tests);
  `make check` formats, then verifies; `make ci` adds race tests and the
  app's checks. Unit runs keep core ADR-0022's `TestI` prefix for
  integration tests.
- **GPU behavior is checked on the host:** release binaries built in nsl
  link only libraries the host has, so they run natively.

## Consequences

- Nothing is installed on the host beyond Go, mise and nsl; everything else
  is reproducible from the repository.
- App tests in nsl run with software rendering; typing latency and graph
  frame rates are confirmed with a native run.
- No mail server, so no incus dependency.

## Alternatives considered

- **Everything in nsl:** slower Go loops and no native GPU checks.
- **A toolbox or distrobox:** shares the host's home and drifts; nsl
  machines are provisioned from the repository.

## References

- Shapes: [M0 plan](../plans/0001-m0-foundations.md); `Makefile`, `dev/nsl/` (M0)
- Builds on: frostmail ADR-0006; frostyard/core ADR-0022, ADR-0023, ADR-0043, ADR-0044
- Extended by: [ADR-0024](0024-patch-what-costs-the-editor-its-frames.md) (pnpm patches of dependencies)
