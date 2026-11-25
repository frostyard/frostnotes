# 0047 — Use the shared NSGo editor on Linux and Lodge

- **Status:** Superseded by [0050](0050-frostnotes-has-no-desktop-window.md) (its MyGo fallback first by [0049](0049-make-the-nsgo-client-the-only-client.md))
- **Date:** 2026-11-23

## Context

The [shared editor trial](0046-test-a-shared-nsgo-source-editor.md) demonstrated cross-client saves, conflict recovery and an authenticated Lodge deployment. Linux now supports first-run vault registration and device pairing. The MyGo desktop still offers richer views, including version history, attachments and backlinks.

## Decision

The default Linux entry point and the Lodge release use the same NSGo workspace. Keep the shipped Flatpak ref `org.frostyard.Frostnotes/x86_64/master`, permissions, launcher and notesd storage. Include the MyGo frontend in that deployment and select it with `--mygo`. Build and test the Flatpak and web payload for the same source revision before publishing the attested OCI image.

## Consequences

Existing installs receive the editor by updating. Both frontends open the same registered vaults and share the device pairing and background setting. The fallback increases download size. The Lodge gateway needs WebAssembly support. The server package includes the same editor that the Linux release was tested with.

## Alternatives considered

- Continuing to publish only a separate preview leaves existing desktop users on the old editor and splits their device registrations.
- Removing MyGo at cutover would remove access to its richer editing and inspection workflows.

## References

- [App design](../design/app.md#shared-nsgo-editor-preview)
- [Flatpak with its own notesd](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
- [Desktop fallback architecture](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- [Build and run](../development.md)
