# 0033 — A setting keeps notesd running without a window

- **Status:** Proposed
- **Date:** 2026-10-28

## Context

The Flatpak's notesd runs in a sandbox of its own, which the Flatpak portal
started, and outlives the app's windows
([ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
The launcher asks the Background portal
(`packaging/flatpak/request-background.py`) to run `frostnotes --background`
at login, so that sync goes on with no window. The desktop's own permission
decides that, and the app has no switch for it: GNOME Settings, Apps,
Frostnotes, Run in Background is the only one. Plan 0008's review recorded
this as gap G2
([plan 0008, Phase 4](../plans/0008-m6-the-standalone-desktop-app.md#phase-4--gaps-found-by-the-review)),
and ADR-0031's Consequences says the same.

Facts from a real GNOME 48 desktop on 2026-10-27
([plan 0008, Phase 3, run 1](../plans/0008-m6-the-standalone-desktop-app.md#record)):

- With the permission off, GNOME stops a notesd that runs with no window
  about 45 s after the last window closes, and one that autostart started at
  login within about 18 s. Neither stop is clean: notesd writes no farewell
  to `notesd.log`.
- A refused request makes the portal delete the autostart entry. Turned on
  again, the permission brings the entry back only at the next launch of the
  app.
- The app is single-instance with one main window: a second launch hands
  over through `tauri-plugin-single-instance` and exits.

Two constraints bound the answer:

- The app's preferences live in the webview's storage
  ([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)),
  which the launcher — a shell script in another sandbox — cannot read.
- The launcher cannot tell a launch a person made from a second launch that
  hands over and exits. Only the running app can tell them apart.

## Decision

The app gains the setting **"Keep syncing when no window is open"**, On by
default, which is today's behaviour. It is kept in a file, and the shell and
the launcher both act on it.

- **Off is a file.** Off is
  `${XDG_CONFIG_HOME:-$HOME/.config}/frostnotes/background-off` existing; On
  is its absence. In the Flatpak, `XDG_CONFIG_HOME` is the app's own config
  directory, which all of its sandboxes share. The app's shell
  (`app/src-tauri/src/background.rs`) writes and removes it, through the
  commands `background_get` and `background_set`; the launcher reads it. A
  file is the one store both can reach.
- **Off stops the autostart, and a login.** The request helper reads the
  file itself, so a change that a person makes while an earlier request
  waits still wins. It takes an exclusive `fcntl.flock` on
  `${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes/background.lock` before
  it reads the setting, and holds the lock until it has written its answer,
  so two helpers never undo each other. While the file exists, the request
  carries `autostart` false, and the portal removes the autostart entry. A
  setting that changed while the portal answered makes the helper ask
  again, at most 3 requests in all, and it writes the last answer only. The
  reason and the commandline do not change, and the launcher and the shell
  start the helper with no arguments.
- **Two modes start notesd.** `frostnotes --notesd` runs notesd and nothing
  else, and every spawn for a window uses it (`flatpak-spawn
  --latest-version frostnotes --notesd`). `frostnotes --background` is the
  autostart entry's command line, which entries that an older release wrote
  still hold: it runs notesd as `--notesd` does, unless the file exists,
  and then it appends one line to `notesd.log` — "frostnotes: background
  activity is off in Settings; notesd does not start at login" — and exits
  0. So Off holds at a login, whatever the portal last answered.
- **Off stops notesd with the last window.** When the app exits, the shell
  runs `notesctl shutdown` — the launcher's own way to stop notesd — and
  waits for it at most 35 s, then kills it. It does this only in the primary
  instance and only in the Flatpak:
  - *Primary:* the setup closure, which runs after the single-instance
    plugin, marks the process. A second launch ends in that plugin first, so
    it never stops the notesd that a window that is open uses.
  - *The Flatpak only:* elsewhere notesd is a service that systemd runs, and
    the window does not own its lifetime. A spike run never marks itself
    primary, so a measurement stops no notesd.
- **The shell shows the desktop's answer.** The helper writes its outcome to
  `${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes/background.json`,
  atomically (a temporary file in the same directory, then `os.replace`):
  `at` (RFC 3339 UTC time), `autostartRequested`, `response` (0 granted,
  1 cancelled, 2 other, null when the portal gave no answer), `background`,
  `autostart`, `error`, and `generation`: the shell's counter of changes, as
  the request read it. `background_get` reads that file, and the settings'
  Background section (`app/src/app/Settings.tsx`'s `backgroundSection`)
  shows the switch, then what the desktop allows. The section is drawn only
  where `background_get` answers, which is only in the Flatpak.
  A change is counted. `background_set` writes or removes the file, then
  bumps a counter in `$XDG_STATE_HOME/frostnotes/background-gen`: it reads
  the counter (missing or bad counts 0), adds 1, and writes the new value
  atomically. Then it starts the helper, and `Background.generation` answers
  the new value, or none when the helper could not start. The shell is the
  counter's only writer, since only the primary instance calls
  `background_set`. Each request of the helper reads the counter under its
  lock, immediately before it reads the file, and writes it into its answer;
  the answer the app reads is the last request's. A failure reads the
  counter when it writes its outcome. A helper that cannot take the lock
  reads the counter too, and it never writes `background.json`: it writes
  `background-failure-<generation>.json` in the same directory instead, and
  removes the failures of lower generations. So it can replace no newer
  answer. The shell reads the answer and those failures, and shows the newer
  of the two; an answer wins a tie. A helper that did take the lock removes,
  after it wrote its answer, the failures up to its generation.
  This holds because the counter is bumped after the setting is written, and
  a helper reads the counter before it reads the setting, under the lock:
  so an answer with `generation >= awaited` read the new setting, and a
  later helper's answer has a generation at least as high. A failure records
  the generation when it fails, so it ends the wait too, unless a newer
  answer or failure is already there.
  A refusal (response 1, or response 0 with the background permission
  false) says how to allow it. A request with no answer, or with response
  2, says that the desktop did not answer, with the helper's error when it
  wrote one: the desktop kept its permission, and the app does not claim
  that it refused. While the dialog is open, the window reads the file again
  every 2 s, with no limit, so a late answer shows: the desktop can keep the
  person in a dialog of its own for minutes, and a launcher's request can
  answer long after the app read the file once. While no answer read the
  counter at or after the change (`generation >= awaited`), the row says
  "Asking the desktop…": it stops only on such an answer, or when the dialog
  closes. There is no timeout, and an old answer never takes the row back.

## Consequences

- A person can stop the background activity from the app, and the app says
  what the desktop allows. Off holds even where the desktop's permission is
  on; with the permission off, the desktop's own stop wins, and the section
  says so.
- Off costs the exit one `notesctl shutdown`: notesd stops within a few
  seconds when it answers, and the shell ends the wait after 35 s whatever
  happens. notesd's writes are atomic, and a committed write stays whole, so
  an abrupt end loses nothing that reached the disk
  ([ADR-0032](0032-an-update-replaces-the-running-notesd.md)).
- With Off, a note written on another device reaches this one only while a
  window is open. The switch says so.
- The request runs in the window's sandbox, as the launcher's own request
  does: a window that closes before the portal answers ends its helper, and
  the answer file keeps the previous answer. The desktop asks at most once,
  so a later launch's request usually answers at once.
- The helpers take turns on the lock, so a helper that waits for the portal
  (at most 10 minutes) holds the next one: a new change's answer waits for
  the earlier request's, and the window's "Asking the desktop…" stays until
  an answer read the counter at or after that change comes.
- While the settings are open, the window reads `background.json` every 2 s,
  and it stops when the dialog closes. A portal that never answers keeps the
  row on "Asking the desktop…" for as long as the dialog stays open: the row
  names what it waits for, and a later answer shows as soon as it is in.
- An old window, from a release before this one, starts notesd with
  `--background` through its own launcher. With Off set — which only this
  release can set — that start finds the file, starts no notesd and says so
  in `notesd.log`: the old window then has no notesd until a window of the
  new release starts one with `--notesd`.
- The setting is per installed app, not per vault: one config directory per
  app ID, which every vault shares.
- ADR-0031's Consequences ("the user has no switch for it in the app yet")
  no longer holds. That ADR is Accepted and stays as it is; this one carries
  the change.
- No automated test reaches a real portal: the build sandbox has none. The
  smoke test checks that a window's spawn says `--notesd`, that the request
  helper starts with no arguments, and that `frostnotes --background`
  starts no notesd and exits 0 while the setting is off; Rust unit tests
  cover the paths, the file and the answer's parse; and Vitest tests cover
  the row's wait, with fake timers. The helper's lock failure has a test in
  `internal/packagecheck` (`TestBackgroundHelperWritesAFailureBesideANewerAnswer`,
  `TestBackgroundHelperWritesAFailureBesideAnOlderAnswer` and
  `TestBackgroundHelperRemovesTheFailuresBelowItsGeneration`), and the
  shell's choice of the newer message has Rust unit tests
  (`the_window_shows_the_newer_of_the_answer_and_a_failure` and
  `an_answer_wins_a_tie_with_a_failure`); the second request
  still has none. Plan 0008's Phase 3, item 4 in each
  state, is still to run on a real desktop.

## Alternatives considered

- **A preference in the webview's storage** (`data/prefs.ts`, ADR-0021):
  rejected. The launcher is a shell script in another sandbox, and it cannot
  read the webview's storage, so it could not drop the autostart request.
- **The launcher stops notesd when the app exits:** rejected. The launcher
  `exec`s the app and sees no exit, and it cannot tell the launch a person
  made from a second launch that must end no notesd.
- **notesd stops itself when its last client goes:** rejected. notesd sees
  connections, not windows: `notesctl`, MCP clients and another device's sync
  hold connections, and the window's bridge reconnects after a lost socket,
  so the wrong clients would decide.
- **Only the desktop's permission:** rejected. The app can then neither keep
  notesd after the last window nor say why the desktop stopped it; GNOME's
  stop 45 s after the last window is invisible in the app.
- **A timer in the shell:** rejected. A window that closes and opens again
  within a few seconds would stop and start notesd each time, and the
  setting must hold at any interval.

## References

- Shapes: [design/app.md](../design/app.md#the-flatpak),
  [specs/ui.md](../specs/ui.md#settings-batch-2),
  [plan 0008, Phase 4, G2](../plans/0008-m6-the-standalone-desktop-app.md#phase-4--gaps-found-by-the-review),
  `app/src-tauri/src/background.rs`, `app/src/data/background.ts`,
  `app/src/app/Settings.tsx`, `packaging/flatpak/frostnotes.sh`,
  `packaging/flatpak/request-background.py`
- Builds on: [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
  (the Flatpak, notesd's lifetime and the Background portal),
  [ADR-0032](0032-an-update-replaces-the-running-notesd.md) (`notesctl shutdown`),
  [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)
  (the app's preferences live in the webview),
  [ADR-0004](0004-a-notes-daemon-with-thin-clients.md) (notesd is a daemon)
