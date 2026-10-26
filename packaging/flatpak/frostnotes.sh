#!/bin/sh
# Frostnotes' launcher in the Flatpak (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
# The manifest installs it as /app/bin/frostnotes, and the Tauri app as
# /app/libexec/frostnotes-app. The app talks to notesd and does not start it,
# so the launcher makes sure a notesd runs for as long as the window does.
#
#   frostnotes --background   Run notesd and nothing else, unless one runs
#                             already. The Background portal starts this at login.
#   frostnotes [ARGS]         Start notesd when none runs, ask the Background
#                             portal for autostart, run the app, and stop the
#                             notesd this launcher started when the app exits.

set -u

app=/app/libexec/frostnotes-app

if [ "${1:-}" = "--background" ]; then
    if notesctl hello >/dev/null 2>&1; then
        exit 0
    fi
    exec notesd
fi

started=
if ! notesctl hello >/dev/null 2>&1; then
    state="${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes"
    mkdir -p "$state"
    notesd >>"$state/notesd.log" 2>&1 &
    started=$!
fi

# Ask the portal to run `frostnotes --background` at login, so sync goes on
# without a window. It asks the user once; errors and a refusal change nothing.
(
    gdbus call --session \
        --dest org.freedesktop.portal.Desktop \
        --object-path /org/freedesktop/portal/desktop \
        --method org.freedesktop.portal.Background.RequestBackground \
        '' "{'reason': <'Keep your notes in sync without a window'>, 'autostart': <true>, 'commandline': <['frostnotes', '--background']>}"
) >/dev/null 2>&1 &

"$app" "$@"
status=$?

if [ -n "$started" ]; then
    kill -TERM "$started" 2>/dev/null
    wait "$started" 2>/dev/null
fi
exit "$status"
