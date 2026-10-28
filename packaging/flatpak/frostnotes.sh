#!/bin/sh
# Frostnotes' launcher in the Flatpak (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
# The manifest installs it as /app/bin/frostnotes, and the Tauri app as
# /app/libexec/frostnotes-app.
#
#   frostnotes --notesd       Run notesd and nothing else: how a window's
#                             launcher and `--background` start it. A second
#                             notesd on the same data directory exits at once
#                             (notesd locks it), so this is safe to run twice.
#                             notesd appends its output to notesd.log in the
#                             state directory, and moves a log over 1 MiB to
#                             notesd.log.1 itself, once it holds the data
#                             directory's lock, so no live log ever moves.
#   frostnotes --background   The autostart entry's command line, which the
#                             Background portal runs at login, and which
#                             entries that an older release wrote still hold:
#                             run notesd as --notesd does, unless the app's
#                             setting has background activity off, and then
#                             append one line to notesd.log and exit 0.
#   frostnotes [ARGS]         Make sure the latest notesd runs, ask the
#                             Background portal for autostart, and run the app.
#
# The app's setting "Keep syncing when no window is open" is a file:
# $XDG_CONFIG_HOME/frostnotes/background-off when it is off, and no file when
# it is on (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md).
# In the Flatpak, XDG_CONFIG_HOME is the app's own config directory, which all
# of its sandboxes share.
#
# So Off holds at a login too, and not only for the autostart entry: the
# request helper reads the file itself and asks the portal for no autostart,
# which removes the entry; and `--background`, which an entry of an older
# release still runs, starts no notesd while the file exists. The app's shell
# stops notesd when its last window closes (no launcher can: see
# app/src-tauri/src/background.rs).
#
# FROSTNOTES_LIBEXEC_DIR, when set, is the directory of frostnotes-app and
# frostnotes-request-background: /app/libexec in the Flatpak, and a test's
# own directory in its tests.
#
# A notesd that the window's sandbox started would die with the sandbox, and
# with a duplicate window that tauri-plugin-single-instance closes at once.
# So the launcher has the Flatpak portal start `frostnotes --notesd` in a
# new sandbox of this app, and then ends the flatpak-spawn process it made
# once that notesd answers: flatpak-spawn forwards catchable signals to the
# command it spawned, but only SIGKILL ends flatpak-spawn alone, and the
# portal keeps the spawned notesd without --watch-bus. That notesd lives on
# when the window closes, until logout. There is no fallback in this sandbox:
# a notesd here would die with a duplicate window while the primary window
# still needs it. Without the portal the launcher says so, and the app shows
# that notesd is not running.
#
# --latest-version makes the portal start the command from the newest
# installed deployment of this app, never from the one this window runs. An
# old window cannot bring back an old notesd. An update replaces the notesd
# that runs, only forward: when its build (`notesctl hello --build`, the
# app-commit of its deployment) differs from the latest deployment's
# (`notesd -build`, run the same way), the launcher stops it with
# `notesctl shutdown`, which returns once notesd has released the data
# directory's lock, and starts the latest. Every call that waits on notesd or
# on the portal is bounded, so neither can keep the window from starting
# (docs/adr/0032-an-update-replaces-the-running-notesd.md).

set -u

libexec="${FROSTNOTES_LIBEXEC_DIR:-/app/libexec}"
app="$libexec/frostnotes-app"
request_background="$libexec/frostnotes-request-background"

state="${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes"

# bounded SECONDS COMMAND...: run the command for at most SECONDS (coreutils
# timeout, which the GNOME runtime has). Every call that waits on notesd or
# on the portal goes through it, so a notesd that accepts and never answers,
# or a portal that hangs, never keeps the window from starting.
if command -v timeout >/dev/null 2>&1; then
    bounded() {
        secs=$1
        shift
        # --kill-after: a command that catches SIGTERM, or a flatpak-spawn
        # that forwards it to a portal call that hangs, gets SIGKILL 1 s on.
        timeout --kill-after=1 "$secs" "$@"
    }
else
    bounded() {
        shift
        "$@"
    }
fi

# hello: true when a notesd answers within 2 s.
hello() {
    bounded 2 notesctl hello >/dev/null 2>&1
}

off="${XDG_CONFIG_HOME:-$HOME/.config}/frostnotes/background-off"

# run_notesd replaces this shell with notesd, which lives on when the caller
# that started it goes.
run_notesd() {
    mkdir -p "$state"
    # notesd rotates the log itself, with -log, once it holds the data
    # directory's lock: a shell check cannot tell 2 launchers apart, and a
    # duplicate must not move the log of the notesd that runs. The redirect
    # keeps what notesd says before the lock, such as "another notesd uses".
    # Both the shell and notesd open the file for appending.
    # shellcheck disable=SC2094
    exec notesd -log "$state/notesd.log" >>"$state/notesd.log" 2>&1
}

if [ "${1:-}" = "--notesd" ]; then
    run_notesd
fi

if [ "${1:-}" = "--background" ]; then
    # The autostart entry's command line, also from an older release's entry.
    # With the setting off, a login starts no notesd at all.
    if [ -e "$off" ]; then
        mkdir -p "$state"
        echo "frostnotes: background activity is off in Settings; notesd does not start at login" >>"$state/notesd.log"
        exit 0
    fi
    run_notesd
fi

mkdir -p "$state"

have_spawn=
if command -v flatpak-spawn >/dev/null 2>&1; then
    have_spawn=1
fi

# An update replaces the engine, only forward: when the running notesd's
# build is not the latest installed one, stop it. Either build empty, or a
# failed or late call, keeps the notesd that runs. The calls are bounded: 2 s
# for a probe, 10 s for the latest build's query, 35 s for the shutdown
# (notesctl's own limit is 30 s).
if hello && [ -n "$have_spawn" ]; then
    running=$(bounded 2 notesctl hello --build 2>/dev/null) || running=
    # The query's output goes to a regular file, not to a command
    # substitution: a substitution waits for EOF on its pipe, and
    # flatpak-spawn hands that pipe to the portal and the new sandbox, which
    # the timeout does not kill, so a stuck query would hold the window
    # after its bound. Reading a regular file never waits.
    latest=
    latest_file=$(mktemp "$state/latest-build.XXXXXX" 2>/dev/null) || latest_file=
    if [ -n "$latest_file" ]; then
        if bounded 10 flatpak-spawn --latest-version notesd -build </dev/null >"$latest_file" 2>/dev/null; then
            read -r latest <"$latest_file" || true
        fi
        rm -f "$latest_file"
    fi
    if [ -n "$running" ] && [ -n "$latest" ] && [ "$running" != "$latest" ]; then
        bounded 35 notesctl shutdown >>"$state/notesd.log" 2>&1 || true
    fi
fi

if ! hello; then
    spawned=
    if [ -n "$have_spawn" ]; then
        # No --watch-bus: the spawned notesd must outlive this sandbox. It
        # holds its own stdout and stderr, so flatpak-spawn may end.
        flatpak-spawn --latest-version frostnotes --notesd </dev/null >>"$state/notesd.log" 2>&1 &
        spawned=$!
    fi
    # Wait, for about 10 s in all, until that notesd answers. The deadline is
    # the clock's, since each probe may take up to 2 s.
    deadline=$(($(date +%s) + 10))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if hello; then
            break
        fi
        if [ -z "$spawned" ] || ! kill -0 "$spawned" 2>/dev/null; then
            break
        fi
        sleep 0.5
    done
    # flatpak-spawn forwards catchable signals (TERM, INT, HUP) to the
    # command it spawned, so only SIGKILL ends flatpak-spawn alone; without
    # --watch-bus the portal keeps the spawned notesd, and notesd already
    # holds its own stdout and stderr. This runs on every path before the
    # app starts: no flatpak-spawn outlives the launcher.
    if [ -n "$spawned" ]; then
        kill -KILL "$spawned" 2>/dev/null
        wait "$spawned" 2>/dev/null
    fi
    if ! hello; then
        msg="frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn); see $state/notesd.log"
        echo "$msg" >&2
        echo "$msg" >>"$state/notesd.log"
    fi
fi

# Ask the Background portal to run `frostnotes --background` at login, or,
# with the setting off, to stop doing so: the helper reads the setting
# itself, and takes its own lock (packaging/flatpak/request-background.py).
# It stays on the bus until the portal answers (it asks the user once), and
# writes that answer to $state/background.json, which the app shows.
"$request_background" >>"$state/background.log" 2>&1 &

exec "$app" "$@"
