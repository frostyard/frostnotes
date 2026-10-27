# Plan: M6, the standalone desktop app

This plan finishes the desktop side of [M6](0002-roadmap.md): the Flatpak
`org.frostyard.Frostnotes`
([ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
[design/app.md](../design/app.md#the-flatpak)) installs from the Frostyard
remote and works on its own. It follows the suite's product rule (Brian,
2026-10-27): each suite app works alone as a Flatpak by default, with its
UI, daemon and helpers inside, and needs no separate frostyard daemon,
lodge, Tailscale, sysext or other suite app. Sync with a hub in lodge is a
supported mode that the user chooses; data never moves without the user's
choice. Notes is the suite's reference app for this rule: step 2 of the
suite's standalone Flatpaks program, which Odrade coordinates. The
packaging decision is core's
[ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)
and its [Plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md),
Phase 5. The hub's packages (the `.deb` and sysext for lodge) are not in
this plan.

Notes offers **sync**, not a remote connection: each device keeps its
vault's files and its own index, and the hub reconciles them
([ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
[design/sync.md](../design/sync.md)). A device that leaves a hub keeps its
files (`notesctl sync leave`).

Status (2026-10-27):
- **Phase 1 is done:** the review below.
- **Phase 2 is built** in PR #103; the remote's entry, the package's
  visibility and the first tag are still to come.
- **Phase 3,** the clean-install check, runs after the first tagged
  publish.
- **Phase 4** lists what the review found; none of it is built.

## Phase 1 — Review against the standalone contract (done 2026-10-27)

The merged Flatpak (PRs #97 and #98; ADR-0031, with its record of a
real Snow install on 2026-10-26), reviewed against the program's list:

| Item | Result | Where |
| --- | --- | --- |
| One package with the UI, notesd and notesctl | Met | `/app/libexec/frostnotes-app`, `/app/bin/notesd`, `/app/bin/notesctl`; `flatpak run --command=notesctl org.frostyard.Frostnotes` |
| A reliable start, one notesd per data directory | Met | the launcher starts notesd through `flatpak-spawn` in its own sandbox and waits about 10 s; notesd's `flock` on `notesd.lock`; a stale socket is removed only when no notesd answers it (lodge kit's `jsonrpc` listener) |
| Clean updates and IPC paths | Paths met; updates in part | the socket is in `$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/`, the one directory all of the app's sandboxes share, and it does not change between versions. After `flatpak update`, the notesd that runs stays at the old version until logout (gap G3) |
| Background and login start through portals | Met | the Flatpak portal (`flatpak-spawn`) and the Background portal (`frostnotes-request-background`), tested on Snow on 2026-10-26 |
| Clear settings for background activity and lodge | Not met | no switch for background activity in the app (G2); joining a hub is `notesctl hub join` only, and the vault manager names a command that a Flatpak user cannot type as written (G1) |
| Predictable behaviour when permissions are refused | Met, with weak messages | Background refused: notesd runs until logout and does not start at login. No Flatpak portal: no notesd, and the window says it is reconnecting. No Secret Service: tokens go to a 0600 file, with a warning. No network: local work goes on, and sync waits. A narrowed `--filesystem`: a vault outside it fails to open. The window does not say why in the last 2 cases (G5) |

The review also found:
- The metainfo and the desktop file said that notes "sync through your
  own hub", as if a hub were required. Fixed in PR #102.
- The Flatpak's notesd reported its version as `flatpak`, and the
  metainfo's only release is 0.0.0. Phase 2's tag run stamps the tag's
  version.
- A host notesd from before the Flatpak has another data directory and
  another socket, so it and the Flatpak's notesd can both run and both
  watch one vault folder (G4).

- **Done when:** the table above is recorded, and each gap has a phase or
  an owner. Done.

## Phase 2 — Publish to the Frostyard remote

Built in PR #103. Implements [design/app.md](../design/app.md#the-flatpak) ("The build"),
after core ADR-0058's publication rules.

- On a `vX.Y.Z` tag, `.github/workflows/flatpak.yml` stamps the version
  into the build (`packaging/flatpak/stamp-version.sh`: notesd's
  `main.version` and `main.commit`, and a metainfo release), builds and
  tests the Flatpak, exports it with `flatpak build-bundle --oci`, and
  pushes it to `ghcr.io/frostyard/frostnotes` with `GITHUB_TOKEN`, as
  `vX.Y.Z` and `sha-<commit>`. It attests the digest with
  `actions/attest-build-provenance`, then moves `latest` to it. The
  publish job runs on a GitHub-hosted runner, checks out nothing and runs
  no repository code; the build jobs hold no write token. Main pushes and
  pull requests publish nothing.
- [`frostyard/flatpak-index`](https://github.com/frostyard/flatpak-index)
  lists the app: an `apps.json` entry with `"repository":
  "frostyard/frostnotes"`, `"tags": ["latest"]` and `"workflow":
  ".github/workflows/flatpak.yml"`. Lucilla owns that repository and
  adds the entry.
- After the first publish, the package `frostnotes` in the organization's
  packages is made public; flatpak-index skips private images.
- **Done when:** after the first tag, `flatpak remote-ls frostyard` lists
  `org.frostyard.Frostnotes` at that version, and `gh attestation verify
  oci://ghcr.io/frostyard/frostnotes:latest --repo frostyard/frostnotes`
  passes.

## Phase 3 — The clean-install check (the release gate)

The program's release gate for Notes. Run it after the first tagged
publish, and again before a release that changes the launcher, the
portals, the socket or the data directory. It needs a real desktop:
the build sandbox has no portals, so CI's smoke test is no evidence for
them. Record each run below, with the date, the versions and a pass or
a fail for each item.

**The machine.** A fresh Snow install in a disposable VM (as in
ADR-0031's record), with GNOME on Wayland, Flathub's
`org.gnome.Platform//51`, and a user install. A second run on a Debian
GNOME desktop answers the roadmap's "clean installs on snow and Debian".
Items 1 to 4 run with no tailnet and no hub; take a VM snapshot after
item 4, for item 5 on the next release. Item 6 needs a hub the VM can
reach over HTTPS, set up only after item 4: lodge's
`https://lodge.<tailnet>.ts.net/device/notes`, with the VM joined to the
tailnet at that point (Tailscale on the VM is test setup, not part of the
app), or a scratch notesd hub with its own TLS, as in ADR-0031's record.
Item 6's URL is that hub's.

1. **Nothing else is there.** `flatpak list` shows no frostyard app;
   `command -v notesd notesctl lodge tailscale` finds nothing;
   `systemctl --user list-units 'frostnotes*'` and
   `systemctl list-units 'frostnotes*' 'lodge*'` list nothing;
   `~/.local/share/frostnotes` does not exist.
2. **Install from the remote.**
   `flatpak remote-add --if-not-exists --user frostyard https://frostyard.github.io/flatpak-index/frostyard.flatpakrepo`,
   then `flatpak install --user frostyard org.frostyard.Frostnotes`.
   `flatpak info org.frostyard.Frostnotes` shows the tag's version and
   commit. The app shows in the desktop's app grid with its name, icon
   and the summary "Plain markdown notes on your own computer".
3. **The local workflow, with no terminal.**
   - Start Frostnotes from the app grid. The vault manager opens, and
     notesd runs: `flatpak ps` shows 2 instances of the app, notesd's
     under `flatpak-portal`.
   - Create a vault with Create new vault and Browse. The location shown
     is a real path in the home directory, not one under
     `/run/user/<uid>/doc/`.
   - Write notes with a wikilink, a tag and a pasted image. Search finds
     the text, and backlinks and the tag pane show the link and the tag.
   - Edit a note in another editor (`gedit` or `vim`). The window shows
     the edit within 2 s, and no conflict copy appears.
   - Open an existing folder of Markdown files with Open folder as vault.
   - `flatpak run --command=notesctl org.frostyard.Frostnotes vault list`
     lists both vaults.
   - Never during this item does the app ask for a hub, an account,
     lodge or the network.
4. **Restart, login and background.**
   - The first launch asks the Background portal to run at login. GNOME
     can grant it with no dialog (ADR-0031's record); if a dialog shows,
     allow it. `~/.config/autostart/org.frostyard.Frostnotes.desktop`
     exists.
   - Close the window. `flatpak ps` shows only notesd's instance. Reopen
     the app: it uses that notesd and starts no `flatpak-spawn`.
   - Log out and in. notesd runs again with no window, and
     `~/.var/app/org.frostyard.Frostnotes/.local/state/frostnotes/notesd.log`
     shows its start.
   - Turn the app's background permission off (GNOME Settings, Apps,
     Frostnotes, Run in Background), log out and in. Record whether a
     notesd starts at login and whether the desktop stops it; that part
     is the desktop's, not the app's. Opening the app must still start a
     notesd and show the vaults. Turn the permission on again.
5. **Update without loss.** This item needs 2 published releases: go
   back to the snapshot taken after item 4 on the older one, once the
   newer one is published, so the update runs with no hub. With the
   window open and notesd running, `flatpak update
   org.frostyard.Frostnotes`; close the window, and open it again.
   - Every vault and note, the workspace and the version history are
     still there.
   - `flatpak run --command=notesctl org.frostyard.Frostnotes hello`
     names the notesd that answers. Until G3 is built, it names the old
     version until logout; record what it says.
   - After a logout and a login, it names the new version.
6. **Opt into sync with a hub in lodge, as its own step.**
   - `flatpak run --command=notesctl org.frostyard.Frostnotes hub join
     https://lodge.<tailnet>.ts.net/device/notes NAME --vault <vault>`;
     approve the code on the hub (`sudo -u frostnotes notesctl hub
     approve CODE`). The first join asks to create a keyring when the
     session has none (ADR-0031).
   - Notes written on the device show on the hub and in lodge's web app,
     and the reverse, within 8 s, also with the window closed.
   - Until G1 is built, this item uses the terminal; after G1, it uses
     the app only.
7. **Disconnects and mode changes.**
   - Stop the hub, or take the VM off the network. Edit notes on both
     sides. Bring the hub back: both edits are kept, merged or as a
     conflict the app shows, never lost.
   - `flatpak run --command=notesctl org.frostyard.Frostnotes sync leave
     --vault <vault>` on the device: its files stay
     and still open and edit locally; the hub's copy stays.
   - Vaults that were never joined did not change at any time during
     items 6 and 7.

- **Done when:** runs on a fresh Snow VM pass items 1 to 4, 6 and 7 on
  the first published release, and item 5 on the update to the second,
  each recorded in "Record" below with the release's version and commit
  and the VM's versions (GNOME, xdg-desktop-portal, Flatpak).

### Record

No run yet.

## Phase 4 — Gaps found by the review

Each gap is its own change, with an ADR where it is a decision. None
blocks Phase 2.

- **G1 — Sync with a hub from the app.** Today a user joins a hub only
  with `notesctl hub join`, and the vault manager names that command,
  which a Flatpak user must type as `flatpak run --command=notesctl
  org.frostyard.Frostnotes hub join`
  ([specs/ui.md](../specs/ui.md#vault-manager)). The app needs a Join
  form (the hub's URL and vault name, the one-time code to approve, the
  wait), a Leave action, and the sync state in the vault's settings,
  over the existing `sync.pair`, `sync.join` and `sync.leave`
  ([design/sync.md](../design/sync.md#pairing-and-sessions)).
  **Done when:** Phase 3's item 6 passes in the app, with no terminal.
- **G2 — A setting for background activity.** notesd lives on after the
  last window closes, until logout, and only the desktop's background
  permission controls the start at login (ADR-0031, Consequences). The
  app should show both, and let the user turn off "keep syncing when no
  window is open". **Done when:** the setting exists, its off state stops
  notesd when the last window closes, and Phase 3's item 4 passes with it
  in each state.
- **G3 — A clean update of the running notesd.** After `flatpak update`,
  the notesd that runs keeps the old version until logout, and new
  windows use it; a method the new app calls and the old notesd lacks
  answers `methodNotFound`. A candidate: notesd watches the Flatpak
  portal's update monitor (`CreateUpdateMonitor`), and on
  `UpdateAvailable` hands its lock to the new version
  (`flatpak-spawn --latest-version`), when no write is in progress.
  It needs an ADR. **Done when:** after `flatpak update` and a new window,
  `notesctl hello` names the new version with no logout, and no write is
  lost.
- **G4 — Moving from a host notesd.** A desktop that ran notesd from a
  build or the `.deb` has another data directory
  (`~/.local/share/frostnotes`) and socket, so both daemons can run and
  both watch one vault folder; a device joined there is not joined in the
  Flatpak. The move must be explicit and written down: stop the host
  notesd, join each vault again from the Flatpak (a new device), and
  revoke the old device on the hub. **Done when:** the steps are in
  [design/app.md](../design/app.md#operational-notes) and were tested
  once.
- **G5 — Say why when a permission is refused.** With no Flatpak portal,
  the window says only that it is reconnecting; a vault outside a
  narrowed `--filesystem` fails with a plain file error. The window
  should name the cause and the fix (`notesd.log`; `flatpak override`).
  **Done when:** both cases show their cause in the window.

## Open questions

- **Must G1 and G2 come before the first tagged publish?** The Flatpak
  works alone without them; sync then needs the terminal, and background
  activity is the desktop's switch. Brian decides, by Phase 2's first
  tag.

## References

- Implements: [design/app.md](../design/app.md#the-flatpak),
  [design/sync.md](../design/sync.md#pairing-and-sessions),
  [specs/ui.md](../specs/ui.md#vault-manager)
- Rationale: [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
  [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md),
  [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
  [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)
- Core: [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md),
  [Plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md);
  the remote: [frostyard/flatpak-index](https://github.com/frostyard/flatpak-index)
- Roadmap: [plan 0002 — M6](0002-roadmap.md)
