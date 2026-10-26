#!/bin/sh
# Frostnotes' launcher in the Flatpak (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
# The manifest installs it as /app/bin/frostnotes, and the Tauri app as
# /app/libexec/frostnotes-app. The app talks to notesd and does not start it.
#
#   frostnotes --background   Run notesd and nothing else. A second notesd on
#                             the same data directory exits at once (notesd
#                             locks it), so this is safe to run twice. The
#                             Background portal starts this at login.
#   frostnotes [ARGS]         Make sure a notesd runs, ask the Background
#                             portal for autostart, and run the app.
#
# A notesd that the window's sandbox started would die with the sandbox, and
# with a duplicate window that tauri-plugin-single-instance closes at once.
# So the launcher has the Flatpak portal start `frostnotes --background` in a
# new sandbox of this app. That notesd lives on when the window closes, until
# logout. There is no fallback in this sandbox: a notesd here would die with
# a duplicate window while the primary window still needs it. Without the
# portal the launcher says so, and the app shows that notesd is not running.

set -u

app=/app/libexec/frostnotes-app
request_background=/app/libexec/frostnotes-request-background

if [ "${1:-}" = "--background" ]; then
    exec notesd
fi

state="${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes"
mkdir -p "$state"

if ! notesctl hello >/dev/null 2>&1; then
    spawned=
    if command -v flatpak-spawn >/dev/null 2>&1; then
        # No --watch-bus: the spawned notesd must outlive this sandbox.
        flatpak-spawn frostnotes --background >>"$state/notesd.log" 2>&1 &
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
