# Security policy

Frostnotes keeps the owner's notes, syncs them through a self-hosted hub,
and lets agents read and write them over MCP. Treat any report that touches
the hub's pairing or device tokens, the sync protocol, the rendering of a
note (notes never run script), the lodge socket, or the folder scopes of
`/mcp/notes` with care.

## Supported versions

Frostnotes is pre-release. Fixes go to `main` and ship in the next release.
Only the latest release (the Flatpak `org.frostyard.Frostnotes` and the
`frostyard-notesd` package) gets fixes; there are no backports to older
releases.

## Reporting a vulnerability

Report it privately, through GitHub's private vulnerability reporting for
this repository: <https://github.com/frostyard/frostnotes/security>
("Report a vulnerability").

Do not open a public issue for a vulnerability. Never post real notes,
device tokens, pairing codes or logs that hold them.

## What to include

- the version (`notesd -version`, `notesctl --version`, or the Flatpak's
  version) or the commit;
- where it runs: the desktop Flatpak, notesd as lodge's `notes` app, a hub,
  or a development build;
- the impact, and what you expected instead;
- the steps to reproduce it, or a made-up vault or note that shows it.
