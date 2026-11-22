# 0045 — The Flatpak keeps no device token in a plain file unless the user asks

- **Status:** Accepted (Brian, 2026-11-22)
- **Date:** 2026-11-22

## Context

A device keeps its hub token in the Secret Service
([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)),
and the Flatpak's notesd uses the default collection
([ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
Both ADRs, and the README, say that the token goes to a 0600 file only when
the user sets `FROSTNOTES_SECRETS=file`.

The code did more than that: when no Secret Service answered on the
session bus, `secrets.Open` returned the file store (`secrets.json` in
notesd's data directory), with only a warning in notesd's log. On a desktop
with no Secret Service, the pairing probe of ADR-0031 then passed on the
file, and the user was not told that the token sat in a plain file
(issue #249). The README's note that `flatpak uninstall --delete-data`
keeps the token in the keyring was wrong for such a device.

Frostmail met the same question and answered it in its ADR-0046: in the
Flatpak there is no file fallback, and outside the Flatpak maild still
falls back with a warning. The apps of the suite should behave alike.

## Decision

- **In the Flatpak** (`config.InFlatpak`), with `FROSTNOTES_SECRETS` unset
  and no Secret Service, notesd keeps no plain file. It runs, logs 1
  Error line at start, and its secret store fails every `Get`, `Set` and
  `Delete` with an error that wraps `secrets.ErrNoKeyring` and ends with
  the way out: run `flatpak override --user
  --env=FROSTNOTES_SECRETS=file org.frostyard.Frostnotes` to keep tokens
  in a plain file instead.
  - The store never answers `ErrNotFound`, which sync reads as a revoked
    token.
  - The pairing probe fails, so `sync.pair` refuses with "this device
    cannot keep a hub token" and the hint, before the hub hears of the
    device.
  - A `secrets.json` that is there stays as it is (a Warn line says so).
    When a Secret Service answers on a later start, its tokens move in once,
    as today, and the file is removed.
- **Outside the Flatpak** (a source build, the .deb, a hub), the fallback
  stays: with no Secret Service, notesd keeps the tokens in the 0600 file
  and logs a warning. A hub host or a server has no desktop session, and
  the file is the only store it has.
- `FROSTNOTES_SECRETS=file` keeps choosing the file in both cases. In the
  Flatpak the user sets it with `flatpak override`.
- `secrets.Options` carry `NoFallback` and the hint that ends an
  `ErrNoKeyring` message. `secrets.OptionsFor(inFlatpak)` picks them:
  `FlatpakOptions()` or `HostOptions()`.

This amends ADR-0016 and ADR-0031: their "the Secret Service keeps the
token" now has the exception above, for a notesd outside the Flatpak.

## Consequences

- A Flatpak user on a desktop with no Secret Service cannot pair until a
  keyring runs or they set the override. The error says which, at the
  join, and not after the token is collected.
- A Flatpak device that paired on the old fallback, and still has no
  Secret Service, stops syncing after the update. Sync reports the
  `ErrNoKeyring` error with the hint, and the pairing stays. The token in
  `secrets.json` is not lost: the override, or a keyring on a later start,
  brings it back.
- The README and the design docs say where a token is kept in each case.
- notesd outside the Flatpak can still keep a token in a plain file
  without the user asking. The warning in the log is the only signal there.

## Alternatives considered

- **No fallback anywhere:** a hub host and a server have no Secret
  Service, so each would need `FROSTNOTES_SECRETS=file` to start syncing,
  and Frostmail would behave differently from Frostnotes.
- **Keep the fallback everywhere, and say so:** the Flatpak is the app
  most users install, and a plain file without their consent is what
  ADR-0016 set out to avoid.

## References

- Shapes: [design/sync.md](../design/sync.md),
  [design/overview.md](../design/overview.md),
  [README](../../README.md)
- Builds on: [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md),
  [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
  Frostmail's [ADR-0046](https://github.com/frostyard/frostmail/blob/main/docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md)
- Issue: [#249](https://github.com/frostyard/frostnotes/issues/249)
