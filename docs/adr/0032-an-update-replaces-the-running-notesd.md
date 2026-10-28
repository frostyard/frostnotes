# 0032 — A new window replaces a notesd of an older build

- **Status:** Proposed
- **Date:** 2026-10-28

## Context

The Flatpak's notesd runs in a sandbox of its own, which the Flatpak portal
started (`flatpak-spawn`), and lives until logout
([ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
`flatpak update` installs a new deployment but does not touch a sandbox
that runs. So after an update the notesd that runs stays at the old build
until logout, and every new window, which runs the new app, connects to
it. A method the new app calls and the old notesd lacks answers
`methodNotFound`. A migration the new notesd brings waits too. Plan 0008's
review recorded this as gap G3
([plan 0008, Phase 4](../plans/0008-m6-the-standalone-desktop-app.md#phase-4--gaps-found-by-the-review)).

Facts that bound the answer:

- One notesd holds a data directory's lock (`notesd.lock`) at a time; a
  second exits at once.
- The socket is in `$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/`, the
  same for every build, so nothing tells a client which build answers.
- `rpc.hello` names notesd's version, which a rebuild of one version does
  not change. A Flatpak instance's `/.flatpak-info` names its deployment's
  commit (`app-commit` in `[Instance]`), which differs for every build.
- `flatpak-spawn --latest-version` runs a command in a new sandbox of the
  latest installed deployment of the caller's app, whatever deployment
  the caller runs from.
- notesd's writes are atomic, and the store refuses a database whose
  schema is newer than its own (`internal/store`, `migrate`).
- frostmail settled the same question for its local maild
  ([frostmail ADR-0046](https://github.com/frostyard/frostmail/blob/main/docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md),
  "An update replaces the engine, only forward"), on frostnotes' launcher.
  The suite's apps should behave alike.

## Decision

When a window opens, the launcher replaces a notesd of an older build with
one of the latest installed build. It only goes forward.

- **notesd knows its build.** `rpc.hello` gains an optional `build`: the
  `app-commit` of the instance's `/.flatpak-info`. It is absent outside a
  Flatpak. `notesd -build` prints the same and exits, and `notesctl hello
  --build` prints the `build` of the notesd that answers (an empty line
  when it has none). These are additive within protocol 1
  ([specs/rpc-protocol.md](../specs/rpc-protocol.md)).
- **A local caller can stop notesd.** `rpc.shutdown` (caller `local` only)
  answers first, then stops notesd as `SIGTERM` does: the contexts of the
  requests in hand are cancelled and notesd waits for their handlers to
  return, sync and the vault actors stop, the store closes, the lock is
  released, and notesd exits 0. A request in hand can complete, return an
  error, or lose its reply. A write is never half done: the file write is
  atomic, a committed write stays whole and is indexed, and a file that
  reached the disk is indexed by the next scan. A client must not take an
  unanswered write as absent. `notesctl shutdown` calls it and waits,
  30 s in all, until the socket no longer answers and the data
  directory's lock is free.
- **The launcher compares and replaces.** `frostnotes [ARGS]`, when a
  notesd answers and `flatpak-spawn` exists, compares `notesctl hello
  --build` with `flatpak-spawn --latest-version notesd -build`. When both
  are known and differ, it runs `notesctl shutdown`, then starts notesd as
  before. Each call is bounded in time, so a stuck notesd or portal
  delays the window, never blocks it.
- **Every start is of the latest build.** The launcher starts notesd with
  `flatpak-spawn --latest-version frostnotes --background`, never in the
  deployment it runs from. A launcher of an old deployment therefore
  cannot bring back an old notesd.
- **Windows that are open reconnect.** The app's transport already
  reconnects when notesd goes away, and the editor sends its unsaved text
  again with the base it names
  ([ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
  [ADR-0006](0006-every-write-names-the-version-it-changes.md)), so a save
  cut by the stop is not lost: it applies in the new notesd. A save whose
  reply was lost after its write committed comes again on the same base
  and merges with no conflict.

## Consequences

- After `flatpak update`, the next window to open makes `notesctl hello`
  name the new build, with no logout. Until a window opens, the old
  notesd keeps running and syncing, which is harmless: only old windows
  use it.
- The update that brings this change does not use it: the notesd that
  runs then is older, has no `build` and no `rpc.shutdown`, and keeps
  running until logout. Every later update uses it.
- Opening a window after an update costs a short gap: the old notesd
  stops, and the new one starts, migrates and scans. Open windows show
  that they reconnect, for a few seconds.
- Old windows then talk to a newer notesd. Protocol 1 changes only by
  addition, so that works.
- The launcher spawns one short sandbox per window to ask the latest
  build (`notesd -build`). That costs a fraction of a second.
- A deliberate downgrade (`flatpak update --commit=…`) makes the older
  build the latest installed one; its notesd refuses a database that a
  newer one migrated, says so in `notesd.log`, and the app shows that
  notesd is not running. A downgrade across a migration needs the user's
  care; it is not a supported path.
- `rpc.shutdown` stops the notesd of the user who owns the socket, and
  only a `local` caller (the app's bridge, `notesctl`) may call it.
  lodge's callers (`session`, `agent`) get `methodNotFound`, as for any
  method they may not use.
- The paths through a real portal (`--latest-version`, the stop and start
  on an update) have no automated test; the smoke test fakes
  `flatpak-spawn`. Plan 0008's Phase 3, item 5, tests them on a real
  desktop with the second release.

## Alternatives considered

- **notesd watches the portal's update monitor** (`CreateUpdateMonitor`,
  the plan's first candidate) and hands over on `UpdateAvailable`: it
  replaces notesd while no new window needs it, and it adds a D-Bus
  watch, a hand-over protocol and a race with the user's own update
  tools. The launcher already runs at the moment a new app needs a new
  notesd.
- **Restart notesd on any version change, or compare versions:** a
  rebuild of one version (a new runtime, a fixed dependency) keeps the
  version, so the old binary would run on; the commit names the build.
- **Leave it until logout (today):** a new app meets an old notesd, and a
  method it needs answers `methodNotFound` for a whole session.

## References

- Shapes: [design/app.md](../design/app.md#the-flatpak),
  [specs/rpc-protocol.md](../specs/rpc-protocol.md),
  [specs/lodge-app.md](../specs/lodge-app.md#methods-and-callers) (`rpc.shutdown`'s callers),
  [plan 0008, Phase 4, G3](../plans/0008-m6-the-standalone-desktop-app.md#phase-4--gaps-found-by-the-review),
  `packaging/flatpak/frostnotes.sh`
- Builds on: [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
  [ADR-0004](0004-a-notes-daemon-with-thin-clients.md),
  [ADR-0006](0006-every-write-names-the-version-it-changes.md),
  [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)
- Sibling: [frostmail ADR-0046](https://github.com/frostyard/frostmail/blob/main/docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md)
