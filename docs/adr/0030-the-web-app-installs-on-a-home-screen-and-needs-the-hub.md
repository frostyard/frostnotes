# 0030 — The web app installs on a home screen, and needs the hub

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

The user will open the web app on their phone, over the tailnet
([ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).
In a phone's browser a page shares the screen with the address bar and
the browser's tabs. A page with a web app manifest can be added to the
home screen and open on its own, without browser chrome.

A progressive web app may also register a service worker. A service
worker intercepts the page's requests, can serve them from a cache with
no network, and stays installed between visits until replaced.

[ADR-0004](0004-a-notes-daemon-with-thin-clients.md) says the web app
needs the hub; only devices keep a copy of a vault, and only the hub
merges between them
([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).
The editor already keeps unsaved text while its connection is down and
saves it with its base when the connection returns
([ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)).
A mobile app (Tauri mobile) with its own copy of the vault is later
work in the roadmap.

## Decision

- **The web app has a manifest,** `manifest.webmanifest`, linked from
  its page:
  - the name Frostnotes and the icon from the app's master
    ([ADR-0013](0013-use-a-sibling-frostyard-notebook-icon.md)), at 192
    and 512 pixels, plus a maskable one;
  - `display: standalone`, `start_url` and `scope` `/`;
  - theme and background colors from the UI's tokens.

  So the phone can add it to the home screen and open it like an app.
- **It registers no service worker.** The hub's content security policy
  sets `worker-src 'none'`, so none can be registered. The app works
  while it reaches the hub, and is a page like any other when it does
  not.
- **Offline, it keeps what was typed and says so.** The status bar
  shows that the hub is out of reach. Open editors keep their text,
  which saves with its base when the hub answers again. Nothing else
  works until then. The app keeps no notes in the browser.
- **Per-browser state is ADR-0021's:** the workspace and preferences in
  the browser's local storage
  ([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)).
  The phone keeps its own layout.

## Consequences

- On the phone, the web app opens from the home screen without the
  browser's chrome.
- With no tailnet, the phone shows no notes. Reading notes offline
  waits for a mobile app with its own copy of the vault.
- Text typed while the hub is out of reach lives in the page. Closing
  the page before it saves loses it. The page asks before it closes
  with unsaved text, as the desktop app's window does, where the
  browser allows.
- No script stays installed between visits, so an injected script
  cannot persist through a worker, and a reload always runs the build
  the hub serves.
- Browsers differ in what Add to Home screen makes of a page without a
  service worker. Phase 6 checks on the user's phone that it opens
  standalone.

## Alternatives considered

- **A service worker that caches the app's files:** the app would start
  offline, but with nothing to show. A cached build could also lag
  behind the hub's protocol, and a worker is a script that outlives the
  page.
- **Notes kept in the browser (IndexedDB) for offline use:** a second
  replica in each browser. It would be a sync client with agreed states
  and merges, which is the device's role, better served by a mobile app
  with notesd's engine.
- **No manifest:** the app works in a browser tab, but on a phone the
  address bar and tab strip take space from every note.

## References

- Shapes: [design/app.md](../design/app.md#in-a-browser), [specs/hub-http.md](../specs/hub-http.md#the-web-apps-files), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md), [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md), [ADR-0029](0029-the-hub-serves-the-web-apps-build-from-a-directory.md)
