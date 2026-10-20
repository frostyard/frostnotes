# 0029 — Inside lodge, a rooted link no file answers opens through lodge

- **Status:** Proposed
- **Date:** 2026-10-20

## Context

Following a link no file answers creates the note, as Obsidian does
([design/app.md](../design/app.md#extensions),
[specs/ui.md](../specs/ui.md#editor-and-reading-view)). A target that
starts with `/` is rooted: it names a path from the vault's root, and
resolves only to the file at that path
([specs/markdown.md](../specs/markdown.md#link-resolution), rules 2 and 3).
Obsidian's link formats never write the `/`; a person or a program that
types one does.

Inside lodge, notes' frame is on lodge's origin, and lodge's own addresses
are rooted paths on it: a note's canonical URL is `/notes/<vault id>/<path>`
([ADR-0026](0026-notesd-runs-as-a-lodge-app.md)), and a mail's
`/mail/message/<id>`. lodge-client's `open` takes them, rooted or as
`https://LODGE/…`, and the shell opens the app they name
([lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md#open)).
A web link to lodge's origin already goes there.

On 2026-10-20 a daily brief that Porter, frostyard's assistant, wrote into
the vault held links like `[…](/mail/message/…)` and `[…](/notes/2/…)`.
Following them in the web app on the owner's phone created empty notes
under `mail/message/` and `notes/2/`, which sync carried to every device.
Porter now writes wiki links for notes and `https://` URLs for lodge's
other pages. Older notes, and other writers, still hold rooted links that
mean lodge's addresses. A click is the only input before the note is made.

## Decision

- **Inside lodge, a rooted link that resolves to no vault file is lodge's
  address, never a new note.** Following it, from the editor, the reading
  view or the Outgoing links pane:
  - when its first segment can name a lodge app (lodge-client's
    `validApp`: `[a-z][a-z0-9]{0,15}`, and not `app`, `device`, `lodge`
    or `mcp`), notes sends `open` with the target, each segment
    percent-encoded as `encodeURIComponent` does. The shell opens that
    app; `/notes/…` comes back to notes as `navigate`.
  - otherwise a notice says that nothing in the vault or in lodge is
    there, and nothing else happens.
- **The vault answers first.** A rooted link that resolves opens its
  file, inside lodge too.
- **Outside lodge nothing changes.** In the desktop app and on the dev
  gateway, following an unresolved rooted link creates the note, as
  Obsidian does.
- **Resolution does not change.** notesd's index and the editor still
  call the link unresolved, and draw it dimmed.

## Consequences

- Inside lodge, a click on a note an agent wrote cannot write the vault
  through a rooted link. Unresolved wiki links and unrooted markdown
  links still create their notes, as in Obsidian.
- Inside lodge, a rooted link cannot make its note by a click. Create new
  note, the quick switcher's Shift+Enter, or the link without its `/`
  still make one.
- A rooted link whose first segment names no installed app switches to
  the shell's notice for an app it does not have: lodge-client does not
  list the installed apps.
- The same link does different things in the desktop app and inside
  lodge.
- Targets reach the app percent-decoded, so a `%2F` inside a segment
  comes back as `/` and the address splits there. A mail's Message-ID
  with a `/` needs Porter's `https://` URL, which goes to lodge as
  written.

## Alternatives considered

- **Leave it a vault link, as Obsidian does:** that is how the stray
  notes were made.
- **Ask first, offering to create the note or to open it in lodge:** a
  phone gains a dialog for a rare link, and the question has a safe
  answer.
- **Open lodge only when the first segment can name an app, and create
  the note otherwise:** whether a click writes the vault would depend on
  a folder name's case.
- **Take every rooted link inside lodge as lodge's, even one that
  resolves:** a vault's own rooted links would stop working inside lodge.

## References

- Shapes: [design/app.md](../design/app.md#in-a-browser),
  [specs/ui.md](../specs/ui.md#in-a-browser)
- Builds on: [ADR-0026](0026-notesd-runs-as-a-lodge-app.md);
  [specs/markdown.md](../specs/markdown.md#link-resolution) (rooted
  targets); lodge's
  [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md#open)
  (`open`)
- Built in: [plan 0006 — M4](../plans/0006-m4-notes-inside-lodge.md)
