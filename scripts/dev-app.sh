#!/usr/bin/env bash
# Run notesd and the app together inside the nsl machine (make app-dev,
# make app-run, make ui-dev). Run from the repository root.
#
# Unix sockets cannot cross the host/VM boundary and SQLite WAL must not live
# on /mnt/host, so this notesd keeps machine-local state under
# /var/tmp/frostnotes (docs/adr/0009-development-environment-and-gates.md).
#
#   scripts/dev-app.sh             notesd and tauri dev
#   APP_BINARY=… scripts/dev-app.sh  notesd and a built app (make app-build)
#   scripts/dev-app.sh --browser   notesd -devgw and Vite, for a browser on the
#                                  host: nsl forwards the machine's loopback
#                                  ports (nsl ports), so open
#                                  http://127.0.0.1:5173 there
set -euo pipefail

NOTESD=${NOTESD:-build/notesd}
[[ -x "$NOTESD" ]] || { echo "dev-app: $NOTESD is missing; run make build on the host first" >&2; exit 1; }

browser=
if [[ "${1:-}" == "--browser" ]]; then
	browser=1
	shift
fi

export FROSTNOTES_DATA_DIR=/var/tmp/frostnotes/notesd
export FROSTNOTES_SOCKET=${XDG_RUNTIME_DIR:?}/frostnotes/notesd.sock

notesd_args=()
if [[ -n "$browser" ]]; then
	# The gateway listens on loopback only and needs this token and the Vite
	# dev server's Origin (internal/devgw).
	DEVGW_ADDR=${DEVGW_ADDR:-127.0.0.1:7890}
	FROSTNOTES_DEVGW_TOKEN=${FROSTNOTES_DEVGW_TOKEN:-$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')}
	export FROSTNOTES_DEVGW_TOKEN
	notesd_args+=(-devgw "$DEVGW_ADDR")
fi

# Everything started here stops with the script, also when it is killed
# rather than interrupted. (Killing `nsl run` on the host does not signal the
# machine's processes: stop them with pkill -f '^bash scripts/dev-app' there.)
children=()
trap 'kill "${children[@]}" 2>/dev/null; wait 2>/dev/null' EXIT
trap 'exit 143' TERM INT

"$NOTESD" "${notesd_args[@]}" &
children+=($!)
for _ in $(seq 50); do [[ -S "$FROSTNOTES_SOCKET" ]] && break; sleep 0.1; done
[[ -S "$FROSTNOTES_SOCKET" ]] || { echo "dev-app: notesd did not create $FROSTNOTES_SOCKET" >&2; exit 1; }
echo "dev-app: notesd is on $FROSTNOTES_SOCKET; register a vault with" >&2
echo "  FROSTNOTES_SOCKET=$FROSTNOTES_SOCKET build/notesctl vault add DIR" >&2

if [[ -n "$browser" ]]; then
	echo "dev-app: open http://127.0.0.1:5173 in a browser on the host (nsl forwards the port)" >&2
	cd app
	# Vite itself rather than pnpm dev, so the trap reaches it.
	VITE_NOTESD_WS="ws://$DEVGW_ADDR/?token=$FROSTNOTES_DEVGW_TOKEN" node_modules/.bin/vite "$@" &
	children+=($!)
	wait "$!"
elif [[ -n "${APP_BINARY:-}" ]]; then
	"$APP_BINARY" "$@"
else
	cd app
	pnpm tauri dev --no-watch "$@"
fi
