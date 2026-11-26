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

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/flatpakgo" ./tools/flatpakgo
# The Flatpak builds notesd and notesctl only, from the root module; the
# NSGo client's module is not in it (plan 0011, Phase 2).
"$tmp/flatpakgo" >"$tmp/daemon.json"
jq 'unique_by(.url) | sort_by(.dest, .url)' "$tmp/daemon.json" >"$tmp/sorted.json"
mv "$tmp/sorted.json" "$out/go-sources.json"
