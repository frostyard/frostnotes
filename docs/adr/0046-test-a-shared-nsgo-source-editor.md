# 0046 — Test a shared NSGo source editor

- **Status:** Superseded by [ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md)
- **Date:** 2026-11-23

## Context

FrostNotes ships separate desktop and Lodge editors. Both implement note selection, editing, saves and conflict feedback. PocketGo has a shared NSGo workspace that runs on Linux and in a browser.

## Decision

Build an opt-in Markdown source editor in its own Go module under `cmd/frostnotes-nsgo`. Both targets use the same view, the existing FrostNotes session and document model, and the versioned PocketGo workspace components. The native entry point connects over the Unix socket. The browser opens a WebSocket, then uses the same generated RPC client. Run the first cross-client editing trial against the existing loopback development gateway.

## Consequences

The experiment exercises autosave, event delivery and conflict recovery on both targets against one daemon. A separate module and app ID allow side-by-side evaluation. A cutover still requires evidence for the richer editing workflows and an authenticated Lodge deployment.

## Alternatives considered

Porting the full live-preview editor first would couple the transport and persistence trial to a separate rendering project.

## References

- [App design](../design/app.md#shared-nsgo-editor-preview)
- [Every write names its base](0006-every-write-names-the-version-it-changes.md)
- [Try the preview](../development.md#try-the-shared-nsgo-editor)
