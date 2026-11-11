#!/bin/sh
# Regenerate the Flatpak's offline sources (make flatpak-sources;
# docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md):
#
#   packaging/flatpak/go-sources.json   the Go modules, from go.mod and go.sum
#
# Run it after any change to go.mod or go.sum; CI regenerates the file and
# fails when it differs. It needs the network and Go.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
out=packaging/flatpak

go run ./tools/flatpakgo >"$out/go-sources.json.tmp"
mv "$out/go-sources.json.tmp" "$out/go-sources.json"
