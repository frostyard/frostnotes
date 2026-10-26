#!/bin/sh
# The Flatpak's smoke test, run by flatpak-builder's test-commands: the
# launcher starts notesd at the Flatpak's socket path, and a note written
# through notesctl reads back. It needs no display and no Secret Service
# (notesd keeps its secrets in a file without one).
#
# FROSTNOTES_BIN_DIR is where frostnotes, notesd and notesctl are; the
# default is the Flatpak's /app/bin. The test cleans up after itself.

set -eu

bin="${FROSTNOTES_BIN_DIR:-/app/bin}"
PATH="$bin:$PATH"
export PATH

work="$(mktemp -d)"
pid=
cleanup() {
    if [ -n "$pid" ]; then
        kill -TERM "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT

# A private runtime directory (mode 0700, as the real one is), home and data.
mkdir -p "$work/run" "$work/home" "$work/data" "$work/vault"
chmod 0700 "$work/run"
export XDG_RUNTIME_DIR="$work/run" HOME="$work/home" XDG_DATA_HOME="$work/data"
export XDG_STATE_HOME="$work/state"
export FLATPAK_ID=org.frostyard.Frostnotes
# No session bus: the Secret Service is absent, as in the build sandbox.
export DBUS_SESSION_BUS_ADDRESS=unix:path="$work/no-bus"
unset FROSTNOTES_SOCKET FROSTNOTES_DATA_DIR

"$bin/frostnotes" --background >"$work/notesd.log" 2>&1 &
pid=$!

ready=
for _ in $(seq 1 40); do
    if notesctl hello >/dev/null 2>&1; then
        ready=1
        break
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
        break
    fi
    sleep 0.5
done
if [ -z "$ready" ]; then
    echo "smoke test: notesd did not answer within 20 s; its log:" >&2
    cat "$work/notesd.log" >&2
    exit 1
fi

socket="$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock"
if [ ! -S "$socket" ]; then
    echo "smoke test: no socket at $socket" >&2
    find "$XDG_RUNTIME_DIR" >&2
    exit 1
fi

notesctl vault add "$work/vault" --name smoke
printf 'hello from the smoke test\n' | notesctl write --create smoke.md
got="$(notesctl cat smoke.md)"
if [ "$got" != "hello from the smoke test" ]; then
    echo "smoke test: read back '$got'" >&2
    exit 1
fi
if [ ! -f "$work/vault/smoke.md" ]; then
    echo "smoke test: the note is not a file in the vault" >&2
    exit 1
fi

kill -TERM "$pid"
wait "$pid" || true
pid=
echo "smoke test passed"
