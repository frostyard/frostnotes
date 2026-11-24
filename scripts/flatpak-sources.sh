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
"$tmp/flatpakgo" >"$tmp/daemon.json"
(cd cmd/frostnotes-nsgo && "$tmp/flatpakgo") >"$tmp/desktop.json"
jq -s 'add | unique_by(.url) | sort_by(.dest, .url)' "$tmp/daemon.json" "$tmp/desktop.json" >"$tmp/combined.json"
mv "$tmp/combined.json" "$out/go-sources.json"
