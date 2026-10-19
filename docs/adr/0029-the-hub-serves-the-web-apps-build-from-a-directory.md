# 0029 — The hub serves the web app's build from a directory

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

The web app is the desktop app's React bundle, served by the hub at `/`
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)). Vite builds it
in the nsl machine; the host never runs Node
([ADR-0009](0009-development-environment-and-gates.md)). notesd is one
pure-Go binary whose gates (`make check`) run on the host without the
app's toolchain. M6 packages the hub as a container image, a systemd
unit and Debian packages.

The desktop app's page has a content security policy in its Tauri
configuration. In a browser, the server's response headers set it. The
browser routes check every request's identity and origin
([ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md)), and
whatever serves the files must apply those checks too.

## Decision

- **`make web-build` builds the web bundle** in nsl with
  `vite build --mode web` into `build/web/`.
  - It is the app's own code, choosing the hub's WebSocket transport.
    The mock, the spikes and the Tauri shell's calls stay out.
  - It writes `frostnotes-web.json` with the app's version and the
    protocol major it speaks.
  - It writes a web manifest and icons
    ([ADR-0030](0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md)).
- **notesd serves that directory,** named by `notesd --hub-web DIR`, on
  the hub's tailnet listener, behind ADR-0027's checks.
  - `/` is `index.html`.
  - Other files are served by exact path: regular files inside `DIR`
    only, never a listing, a hidden name, `..` or a symbolic link that
    leaves it.
  - Vite's hashed files under `/assets/` are cached for a year as
    immutable, and everything else is `no-cache`.
  - Without `--hub-web`, `/` answers 404, and `/sync` and `/mcp` still
    serve.
- **notesd checks the build at start:** a directory without
  `index.html`, or whose `frostnotes-web.json` names another protocol
  major, is not served, and the log says why.
- **notesd sets the page's headers,** the same on every browser route:
  - a content security policy as strict as the desktop's, `default-src
    'none'` with the app's own scripts, styles and fonts, images from
    the hub, `data:` and `https:` (the Load remote images preference),
    media from the hub, `connect-src` the hub's own `wss://` origin,
    `worker-src`, `frame-src` and `object-src` `'none'`, `base-uri`
    and `form-action` `'none'`, and `frame-ancestors 'none'`;
  - `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
    `Cross-Origin-Opener-Policy: same-origin` and
    `Cross-Origin-Resource-Policy: same-origin`.

  [specs/hub-http.md](../specs/hub-http.md) pins the exact values.

## Consequences

- Go builds and tests never need the bundle; the web routes' tests use
  a small fixture directory. The desktop and web bundles come from one
  source tree and one `vite` configuration.
- The hub and its web build are two artifacts that a deployment keeps
  in step. The protocol check catches a mismatch that would break, and
  `rpc.hello` still guards each connection.
- An update to the web app alone needs no notesd restart: files are
  read from disk per request, and a page reload picks them up.
- M6 installs the directory beside the binary, or copies it into the
  hub's image.

## Alternatives considered

- **Embedding the build in notesd (`go:embed`):** one artifact. But the
  Go build would need the bundle first, on the host that never runs
  Node, or a committed placeholder that a release overwrites.
- **A separate static server,** such as `tailscale serve`'s file
  serving or a web server beside notesd: a second place for identity,
  origin checks and headers to agree. It also does not fit the hub's
  own tailnet node.
- **Loading the app from a CDN:** public by nature, and outside the
  tailnet.

## References

- Shapes: [design/hub.md](../design/hub.md#the-web-apps-files), [specs/hub-http.md](../specs/hub-http.md#the-web-apps-files), [design/app.md](../design/app.md#in-a-browser), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md) (the CSP forbids inline script), [ADR-0009](0009-development-environment-and-gates.md), [ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md)
