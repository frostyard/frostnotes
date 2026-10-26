#!/bin/sh
# Frostnotes' launcher in the Flatpak (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
# The manifest installs it as /app/bin/frostnotes, and the Tauri app as
# /app/libexec/frostnotes-app.
#
#   frostnotes --background   Run notesd and nothing else. A second notesd on
#                             the same data directory exits at once (notesd
#                             locks it), so this is safe to run twice. The
#                             Background portal starts this at login. notesd
#                             appends its output to notesd.log in the state
#                             directory; when no notesd runs already and that
#                             log is over 1 MiB, it moves to notesd.log.1
#                             first, so the log stays bounded.
#   frostnotes [ARGS]         Make sure a notesd runs, ask the Background
#                             portal for autostart, and run the app.
#
# FROSTNOTES_LIBEXEC_DIR, when set, is the directory of frostnotes-app and
# frostnotes-request-background: /app/libexec in the Flatpak, and a test's
# own directory in its tests.
#
# A notesd that the window's sandbox started would die with the sandbox, and
# with a duplicate window that tauri-plugin-single-instance closes at once.
# So the launcher has the Flatpak portal start `frostnotes --background` in a
# new sandbox of this app, and then ends the flatpak-spawn process it made
# once that notesd answers: flatpak-spawn forwards catchable signals to the
# command it spawned, but only SIGKILL ends flatpak-spawn alone, and the
# portal keeps the spawned notesd without --watch-bus. That notesd lives on
# when the window closes, until logout. There is no fallback in this sandbox:
# a notesd here would die with a duplicate window while the primary window
# still needs it. Without the portal the launcher says so, and the app shows
# that notesd is not running.

set -u

libexec="${FROSTNOTES_LIBEXEC_DIR:-/app/libexec}"
app="$libexec/frostnotes-app"
request_background="$libexec/frostnotes-request-background"

state="${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes"

if [ "${1:-}" = "--background" ]; then
    mkdir -p "$state"
    # Rotate only when no notesd answers on this socket: a second
    # `frostnotes --background` must not move the log a running notesd
    # writes. A notesd that starts at the same time has just rotated its own
    # log, so the log is small then.
    if ! notesctl hello >/dev/null 2>&1 &&
        [ -f "$state/notesd.log" ] &&
        [ "$(wc -c <"$state/notesd.log")" -gt 1048576 ]; then
        mv -f "$state/notesd.log" "$state/notesd.log.1"
    fi
    exec notesd >>"$state/notesd.log" 2>&1
fi

mkdir -p "$state"

if ! notesctl hello >/dev/null 2>&1; then
    spawned=
    if command -v flatpak-spawn >/dev/null 2>&1; then
        # No --watch-bus: the spawned notesd must outlive this sandbox. It
        # holds its own stdout and stderr, so flatpak-spawn may end.
        flatpak-spawn frostnotes --background </dev/null >>"$state/notesd.log" 2>&1 &
        spawned=$!
    fi
    # Wait, for about 10 s, until that notesd answers.
    tries=0
    while [ "$tries" -lt 20 ]; do
        if notesctl hello >/dev/null 2>&1; then
            break
        fi
        if [ -z "$spawned" ] || ! kill -0 "$spawned" 2>/dev/null; then
            break
        fi
        tries=$((tries + 1))
        sleep 0.5
    done
    # flatpak-spawn forwards catchable signals (TERM, INT, HUP) to the
    # command it spawned, so only SIGKILL ends flatpak-spawn alone; without
    # --watch-bus the portal keeps the spawned notesd, and notesd already
    # holds its own stdout and stderr.
    if [ -n "$spawned" ] && kill -0 "$spawned" 2>/dev/null; then
        kill -KILL "$spawned" 2>/dev/null
        wait "$spawned" 2>/dev/null
    fi
    if ! notesctl hello >/dev/null 2>&1; then
        msg="frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn); see $state/notesd.log"
        echo "$msg" >&2
        echo "$msg" >>"$state/notesd.log"
    fi
fi

# Ask the Background portal to run `frostnotes --background` at login. The
# helper stays on the bus until the portal answers (it asks the user once).
"$request_background" >>"$state/background.log" 2>&1 &

exec "$app" "$@"
