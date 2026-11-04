#!/usr/bin/env bash
# Take the Flatpak metainfo's screenshots (make screenshots) in the nsl
# machine: notesd on a copy of the made-up vault in dev/screenshots/vault,
# with its own data directory, the Vite dev server, and shoot.py, which drives
# the UI in WebKitGTK (WebKitWebDriver and MiniBrowser) under Xvfb and writes
# 1280x800 PNGs to docs/images. Run from the repository root, after make build.
#
# The vault is made up: no real person, address or note. The copy keeps the
# committed vault as it is, since the UI may write to the notes it opens.
set -euo pipefail

NOTESD=${NOTESD:-build/notesd}
NOTESCTL=${NOTESCTL:-build/notesctl}
OUT=${OUT:-docs/images}
[[ -x "$NOTESD" && -x "$NOTESCTL" ]] || { echo "screenshots: run make build on the host first" >&2; exit 1; }
# Another Vite on its port would serve another notesd's notes: refuse.
if curl -fs -o /dev/null http://127.0.0.1:5173/; then
	echo "screenshots: port 5173 is in use; stop that server first (make ui-dev?)" >&2
	exit 1
fi

state=$(mktemp -d /var/tmp/frostnotes-screenshots.XXXXXX)
export FROSTNOTES_DATA_DIR=$state/notesd
export FROSTNOTES_SOCKET=$state/run/notesd.sock
mkdir -p "$FROSTNOTES_DATA_DIR" && mkdir -m 0700 "$state/run"
cp -r dev/screenshots/vault "$state/Notebook"
FROSTNOTES_DEVGW_TOKEN=$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')
export FROSTNOTES_DEVGW_TOKEN
devgw=127.0.0.1:7891

children=()
trap 'kill "${children[@]}" 2>/dev/null; wait 2>/dev/null; rm -rf "$state"' EXIT
trap 'exit 143' TERM INT

"$NOTESD" -devgw "$devgw" &
children+=($!)
for _ in $(seq 50); do [[ -S "$FROSTNOTES_SOCKET" ]] && break; sleep 0.1; done
[[ -S "$FROSTNOTES_SOCKET" ]] || { echo "screenshots: notesd did not start" >&2; exit 1; }
"$NOTESCTL" vault add "$state/Notebook" --name Notebook

# The dev gateway admits only the Vite dev server's Origin (internal/devgw),
# so Vite must take its usual port.
(cd app && VITE_NOTESD_WS="ws://$devgw/?token=$FROSTNOTES_DEVGW_TOKEN" exec node_modules/.bin/vite --host 127.0.0.1 --strictPort) &
vite=$!
children+=("$vite")
for _ in $(seq 100); do curl -fs -o /dev/null http://127.0.0.1:5173/ && break; sleep 0.2; done
kill -0 "$vite" 2>/dev/null || { echo "screenshots: Vite did not start" >&2; exit 1; }

mkdir -p "$OUT"
env -u WAYLAND_DISPLAY GDK_BACKEND=x11 xvfb-run -a -s "-screen 0 1600x1200x24" python3 dev/screenshots/shoot.py "$OUT"
