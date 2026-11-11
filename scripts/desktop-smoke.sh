#!/bin/sh
# The native desktop app's smoke test (docs/plans/0009-mygo-desktop.md,
# Phase 6): the release build starts under Xvfb against a real notesd with
# a vault, and is still running when the test stops it. A build that cannot
# load GTK or start exits or crashes before then; the test does not check
# that the window connected or drew, which the Tester's tests do. Everything
# lives in a temporary directory, and nothing touches the caller's notes.
#
# Usage: scripts/desktop-smoke.sh [BUILD_DIR]. It needs notesd, notesctl
# and frostnotes in BUILD_DIR (default build/), and xvfb-run and
# dbus-run-session on the path.
set -u

build=$(cd "${1:-build}" && pwd)
seconds=10

tmp=$(mktemp -d)
pid=
cleanup() {
    [ -n "$pid" ] && kill "$pid" 2>/dev/null
    rm -rf "$tmp"
}
trap cleanup EXIT

export HOME="$tmp/home" XDG_RUNTIME_DIR="$tmp/run" XDG_CONFIG_HOME="$tmp/config" XDG_DATA_HOME="$tmp/data" XDG_STATE_HOME="$tmp/state"
export LIBGL_ALWAYS_SOFTWARE=1 GDK_BACKEND=x11
unset FROSTNOTES_SOCKET FROSTNOTES_DATA_DIR
mkdir -p "$HOME" "$XDG_RUNTIME_DIR" "$tmp/vault"
chmod 700 "$XDG_RUNTIME_DIR"
printf '# Garden\n\nSome **bold** text.\n' >"$tmp/vault/Garden.md"

"$build/notesd" >"$tmp/notesd.log" 2>&1 &
pid=$!
tries=0
until "$build/notesctl" hello >/dev/null 2>&1; do
    tries=$((tries + 1))
    if [ "$tries" -gt 100 ] || ! kill -0 "$pid" 2>/dev/null; then
        echo "desktop-smoke: notesd did not answer:" >&2
        cat "$tmp/notesd.log" >&2
        exit 1
    fi
    sleep 0.1
done
"$build/notesctl" vault add "$tmp/vault" >/dev/null || exit 1

status=0
dbus-run-session -- xvfb-run -a -s "-screen 0 1280x800x24" \
    timeout "$seconds" "$build/frostnotes" >"$tmp/app.log" 2>&1 || status=$?
if [ "$status" -ne 124 ]; then
    echo "desktop-smoke: frostnotes ended with status $status before the test stopped it:" >&2
    cat "$tmp/app.log" >&2
    exit 1
fi
echo "desktop-smoke: the app ran for ${seconds}s"
