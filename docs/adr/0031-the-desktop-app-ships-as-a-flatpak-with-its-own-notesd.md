# 0031 — The desktop app ships as a Flatpak with its own notesd

- **Status:** Proposed
- **Date:** 2026-10-26

## Context

Frostyard ships its desktop apps as Flatpaks
([core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)),
and [Plan 0010, Phase 5](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)
asks for frostnotes' first: `org.frostyard.Frostnotes`, with notesd and
`notesctl` inside, vault access, and the Background portal for sync at
login. The app does not start notesd
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)); it connects to its
socket and retries. A Flatpak raises 6 questions the repository has not
answered:

- **Build.** CI builds inside the Flatpak SDK, offline. The build needs
  the Rust crates of the Tauri shell, a pnpm store for the web build, and
  the Go modules of notesd as sources with checksums.
- **The socket.** Each sandbox of an app has its own `$XDG_RUNTIME_DIR`,
  except `$XDG_RUNTIME_DIR/app/<app-id>`, which Flatpak shares between all
  sandboxes of the app. The window, notesd and `notesctl` run in separate
  sandboxes (a `flatpak run --command=notesctl`, the Background portal's
  autostart), so the default socket
  `$XDG_RUNTIME_DIR/frostnotes/notesd.sock` is not shared.
- **The vaults.** A vault is a folder anywhere in the home directory.
  notesd watches it with inotify and writes by atomic rename
  ([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)).
- **Lifetime.** Sync must go on without a window
  ([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)), and a Flatpak has
  no systemd user unit. A process in the window's sandbox dies with that
  sandbox, and so does one in a duplicate window that
  `tauri-plugin-single-instance` closes at once.
- **The data.** The app reads notesd's blob store directly
  ([ADR-0023](0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)).
- **Secrets and network.** notesd keeps device tokens in the Secret
  Service and falls back to a file; sync needs the network.

## Decision

**The app ships as the Flatpak `org.frostyard.Frostnotes`**, built from
`packaging/flatpak/org.frostyard.Frostnotes.json` on `org.gnome.Platform//51`.

- **Offline sources.** The manifest builds 2 modules with no network. The
  `notesd` module builds `notesd` and `notesctl` with the Go SDK
  extension, from a `file` source per module `.mod` and `.zip` and a
  `file://` `GOPROXY`. The `frostnotes` module builds the Tauri shell
  with the Rust and Node SDK extensions, from vendored crates and an
  offline pnpm store (`pnpm install --offline --frozen-lockfile
  --trust-lockfile`, with the pnpm binary from the lockfile's own
  `@pnpm/exe` tarball). The 3 source files, `cargo-sources.json`,
  `node-sources.json` and `go-sources.json`, are generated and committed:
  `make flatpak-sources` runs `scripts/flatpak-sources.sh`, which runs
  `flatpak-builder-tools` at a pinned commit (cargo and node) and
  `tools/flatpakgo` (Go). Every step runs without Node, pnpm or Rust on
  the host. The generators' Python dependencies come from
  `generator-requirements.txt`, installed with `pip install
  --require-hashes`, as [core ADR-0023](https://github.com/frostyard/core/blob/main/docs/adr/0023-verified-pinned-downloads.md)
  asks of a download. One patch,
  `packaging/flatpak/patches/pnpm-git-path.patch`, goes on the node
  generator: for a git-hosted package with `#path:<dir>` (lodge's
  `client` and `ui`), pnpm adds only that subdirectory to its store, so the
  generator's store index must hold the files of `<dir>` at the package
  root, not those of the whole tarball.
- **Install layout.** `/app/bin/frostnotes` is a launcher (the manifest's
  `command`); `/app/bin/notesd` and `/app/bin/notesctl` are the Go binaries,
  so `flatpak run --command=notesctl org.frostyard.Frostnotes` works;
  `/app/libexec/frostnotes-app` is the Tauri binary. Metainfo, desktop file
  and icons are named for the app ID.
- **The socket lives in the shared directory.** `config.DefaultSocket`
  returns `$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock`
  when `FLATPAK_ID` is `config.AppID`, and the shell's `paths.rs` does the
  same. `FROSTNOTES_SOCKET` still wins, and outside a Flatpak nothing
  changes. The data directory needs no change: Flatpak gives the app one
  `XDG_DATA_HOME`, so the app and notesd share `blobs/`.
- **notesd runs in its own sandbox, once per data directory.** notesd
  takes an exclusive `flock` on `<data dir>/notesd.lock` before it opens the
  database and holds it until it exits; a second notesd on the same data
  directory exits at once with "another notesd uses <dir>", whatever its
  socket. The launcher `frostnotes` (POSIX sh) works so:
  - `frostnotes --background` runs `exec notesd`, with the output appended
    to `$XDG_STATE_HOME/frostnotes/notesd.log`, also when the Background
    portal's autostart runs it at login. A log over 1 MiB moves to
    `notesd.log.1` first.
  - `frostnotes [ARGS]`: when `notesctl hello` fails, it runs
    `flatpak-spawn frostnotes --background` (without `--watch-bus`). The
    Flatpak portal
    starts that in a new sandbox of the same app, so notesd lives on when the
    window closes, until logout. The launcher waits for about 10 s for
    `notesctl hello`, then ends `flatpak-spawn` with `SIGKILL`.
    `flatpak-spawn` waits for its command, so it would keep the window's
    sandbox alive as long as notesd runs. It forwards each signal it can
    catch to the command, so only `SIGKILL` ends it alone; without
    `--watch-bus` the portal keeps the command, which already holds its
    own output. When no `notesctl hello` answers (no portal), it logs an error and
    starts no notesd of its own: one in the window's sandbox would die with
    a duplicate window while the primary window still needs it. The app
    then shows that notesd is not running, and the next launch tries again.
    It then asks the Background portal to run `frostnotes --background` at
    login, and `exec`s the app, so the app gets the signals and the exit
    status.
- **The portal request is a Python helper.** `gdbus call` exits as soon as
  the portal returns the request handle, and the portal closes a request when
  its caller leaves the bus, before the user has answered. The helper
  `frostnotes-request-background` (PyGObject, which the GNOME platform ships)
  subscribes to the request's `Response` signal, calls
  `org.freedesktop.portal.Background.RequestBackground` (`reason`,
  `autostart`, and `commandline` `frostnotes --background`), and stays on the
  bus in a GLib main loop until the `Response` or 10 minutes. It exits 0
  whatever happens: a missing portal or a refusal changes nothing for the
  app.
- **Device tokens go to the Secret Service's default collection**, which
  notesd makes when the session has none. notesd reads the `default` alias
  (`ReadAlias`); when there is none, or the collection it names does not
  exist, it calls `CreateCollection` with the alias `default` and the
  label "Default keyring", as libsecret does, and the keyring asks the
  user for a password. A session that GNOME Keyring did not unlock at
  login, as after an autologin, has only the `session` collection, which
  is lost at logout. Before a device asks a hub to pair, it writes and
  deletes a probe secret, so a keyring that cannot keep a token fails
  the join first. A device that cannot keep the token it collected
  withdraws the pairing (`DELETE /sync/device`), and the hub forgets the
  device, which has made no request
  ([sync-protocol.md](../specs/sync-protocol.md#pairing)).
  `FROSTNOTES_SECRETS=file` (`flatpak override --env=…`) still keeps
  tokens in a 0600 file.
- **Permissions** (`finish-args`): `--socket=wayland`,
  `--socket=fallback-x11`, `--share=ipc`, `--device=dri`, `--share=network`
  (sync with the hub), `--talk-name=org.freedesktop.secrets` (device
  tokens) and `--filesystem=home`. The Background portal and the file
  chooser portal need no finish-arg.
- **Why `--filesystem=home`.** A vault is a folder anywhere in the home
  directory, and notesd is a daemon that starts at login. The document
  portal mounts a FUSE file system that gives no inotify events and no
  stable path, so notesd could not watch a vault there, and a path chosen
  in one run would be stale in the next. Users can narrow the permission
  with `flatpak override --user --nofilesystem=home --filesystem=~/Notes
  org.frostyard.Frostnotes`. Where `HOME` is `/var/home/<user>` (Snow,
  Fedora's atomic desktops), the host's `/home` does not exist in the
  sandbox; notesd reads a vault path under `/home` as the same path under
  `/var/home` when `/home` is missing and `/var/home` exists.
- **CI.** `.github/workflows/flatpak.yml` runs on pull requests and on
  pushes to main. One job runs `make flatpak-sources` and fails on a diff.
  The other builds in the pinned `flatpak-github-actions` image and runs
  the manifest's `test-commands`: the shell's Rust tests,
  `desktop-file-validate`, `appstreamcli validate`, a compile and import
  check of the Python helper, and a smoke test. The smoke test runs
  `frostnotes --background` at the Flatpak's socket path, checks that a
  second notesd on the same data directory exits, and writes and reads a note
  through `notesctl`, without a Secret Service. It starts notesd through
  the launcher with a stand-in `flatpak-spawn` and checks that the
  launcher ends it, that notesd lives on, and that the log turns over.
  It does not test the real portals: the build sandbox has none.

## Consequences

- A dependency bump in `Cargo.lock`, `pnpm-lock.yaml`, `go.mod` or `go.sum`
  needs `make flatpak-sources` in the same change; else the sources job
  fails. The sources are large (about 480 KB of JSON).
- The socket path has 2 sources of truth, as the data directory has:
  `internal/config` and `app/src-tauri/src/paths.rs` must stay in step, and
  both have unit tests.
- notesd lives on after the last window closes, until logout; the user
  has no switch for it in the app yet (`flatpak kill
  org.frostyard.Frostnotes` stops it). It runs again at login only when
  the user allows the Background portal's request. A refusal stops only
  the autostart at login: the notesd that runs keeps running. Without the
  Flatpak portal (`flatpak-spawn`), no notesd starts, and the app shows
  that it is not connected.
- A data directory has one notesd, whatever the socket. Tests and
  evidence that start several notesd processes need one data directory each
  (`internal/evidence` already gives each its own).
- The portal paths of the launcher (`flatpak-spawn`, the Background
  request) have no automated test against a real portal; the smoke test
  cannot reach one. A change to them needs a test on a real desktop, as
  below.
- The Background request's helper stays in the window's sandbox until
  the portal answers, at most 10 minutes. The portal asks the user only
  once; after that it answers at once.
- A first hub join in a session without a default keyring shows the
  keyring's prompt for a new password. A keyring made so is not unlocked
  by an autologin, so its unlock prompt comes again at each login, when
  notesd first reads the token.
- `--filesystem=home` gives the app and its notesd the whole home
  directory. That is wider than a vault, and it is the price of a watched,
  stable vault path.
- The Flatpak builds with the SDK extensions' Go and Rust, which can lag
  the toolchains CI uses elsewhere (`rust-stable` 1.98.0 against 1.99.0
  in `ci.yml`; `Cargo.toml`'s `rust-version` is 1.90).
- The pnpm store comes from a generator that writes pnpm's v11 store
  format; a pnpm release that changes the format needs a newer generator.
- Publishing the OCI image to GHCR, with the attestation the Frostyard
  remote requires, is a later step; this change builds and tests only.

### Evidence: a real Snow install, 2026-10-26

The bundle from CI run 40389390295 (PR #97, head `7f1f23b`) was tested on
Snow (`ghcr.io/frostyard/snow:latest`, from the installer ISO of
2026-10-26) in a disposable incus VM: GNOME 48.7 on Wayland,
xdg-desktop-portal 1.20.3, xdg-desktop-portal-gnome 48.0, Flatpak
1.16.6, `org.gnome.Platform//51` from Flathub, a user install. The hub
was a scratch notesd in the same VM. All 6 items passed:

1. A vault opens: `notesctl vault add` added it, and the window showed
   its notes.
2. notesd starts through `flatpak-spawn`: the window and notesd ran in 2
   sandbox instances (`flatpak ps`), with notesd under `flatpak-portal`,
   and the socket in the shared directory.
3. notesd outlives the window: after the window closed, a write on the hub
   and a plain file write on the device synced both ways in less than 8 s.
4. Autostart at login: the portal granted `background` and `autostart`
   with no dialog and wrote `~/.config/autostart/org.frostyard.Frostnotes.desktop`;
   logout stopped notesd, and at the next login gnome-session started
   it, and sync went on with the token from the keyring. A new window
   used that notesd and started no `flatpak-spawn`.
5. The lock: a second notesd, through `flatpak run --command=notesd` or
   `frostnotes --background`, exited at once with "another notesd uses
   …".
6. `--filesystem=home`: vaults in `~` and in `~/Documents` opened and
   were watched; a path outside the home directory failed, as expected.

The test found 2 defects and 2 smaller points; the decisions above hold
the fixes:

- A hub join failed in a session with no `login` keyring (`Object does
  not exist at path "/org/freedesktop/secrets/collection/login"`), and
  the hub kept the failed pairing as a device never seen. Fixed by the
  default collection, the probe and the withdrawal.
- The window's sandbox stayed alive after the window closed, because
  `flatpak-spawn` waited for notesd. Fixed: the launcher ends it.
- notesd from autostart logged to the journal. Fixed: `--background`
  writes `notesd.log`.
- A vault path under `/home` failed on Snow. Fixed: notesd reads it under
  `/var/home`.

Items 3 and 4 are to be run again on the same VM with the fixes.

## Alternatives considered

- **The document portal for vaults** (the file chooser's grants):
  rejected. Its FUSE mount gives no inotify events and no stable path for
  a daemon that starts at login.
- **`--filesystem=xdg-documents` only:** rejected. Obsidian vaults and
  notes live in any folder; a user with `~/Notes` would meet a silent
  failure. It remains a user override.
- **A systemd user unit for notesd:** rejected. A Flatpak cannot install
  one, and the host's systemd cannot start a process in the app's sandbox.
  The Background portal is the supported way to run at login.
- **notesd started and owned by the window's launcher** (the first
  design): rejected. It ran in the window's sandbox and died with it, also
  for a duplicate window that closes at once.
- **A wrapper that sets `FROSTNOTES_SOCKET`:** rejected. It does not reach
  `flatpak run --command=notesctl`, nor the Background portal's autostart,
  and every caller would have to repeat the path. `FLATPAK_ID` is set in
  every sandbox of the app.
- **`gdbus call` for the Background request:** rejected. The caller leaves
  the bus before the user answers, and the portal closes the request.
- **Vendoring Go modules with `go mod vendor`:** rejected. It adds
  thousands of files to the repository; a `file` source per module has
  the checksums in one generated file.
- **Building notesd outside the SDK, as frostmail's local Flatpak does:**
  rejected by core ADR-0058, which requires a build in CI inside the SDK.

## References

- Shapes: [design/app.md](../design/app.md#the-flatpak), [design/overview.md](../design/overview.md), [design/sync.md](../design/sync.md#pairing-and-sessions), [specs/rpc-protocol.md](../specs/rpc-protocol.md#transports), [specs/sync-protocol.md](../specs/sync-protocol.md#pairing)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) (device tokens in the Secret Service), [ADR-0023](0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md), [ADR-0026](0026-notesd-runs-as-a-lodge-app.md) (the other way notesd runs)
- Core: [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md), [Plan 0010, Phase 5](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)
- Built in: [roadmap, M6](../plans/0002-roadmap.md)
