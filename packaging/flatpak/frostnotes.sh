#!/bin/sh
# Frostnotes' launcher in the Flatpak, which holds notesd and notesctl and no
# window (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md,
# amended by ADR-0050; docs/design/app.md#the-flatpak). The manifest installs
# it as /app/bin/frostnotes, the Flatpak's command. A desktop's editor
# (Obsidian among them) edits the vault folders, and notesd syncs them.
#
#   frostnotes                Make sure the latest notesd runs, ask the
#                             Background portal to start it at login, wait
#                             for the answer, and print `notesctl status`
#                             (its exit status is this command's).
#   frostnotes --notesd       Run notesd and nothing else: how the launcher's
#                             spawn starts it. A second notesd on the same
#                             data directory exits at once (notesd locks it),
#                             so this is safe to run twice. notesd appends its
#                             output to notesd.log in the state directory, and
#                             moves a log over 1 MiB to notesd.log.1 itself,
#                             once it holds the data directory's lock, so no
#                             live log ever moves.
#   frostnotes --background   The autostart entry's command line, which the
#                             Background portal runs at login: run notesd as
#                             --notesd does, unless background activity is
#                             off, and then append one line to notesd.log and
#                             exit 0.
#   frostnotes --help         Say this, and exit.
#
# Background activity is on by default; `notesctl background off` turns it
# off, and `notesctl background on` back on. Off is a file,
# $XDG_CONFIG_HOME/frostnotes/background-off, and On its absence
# (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md). In the
# Flatpak, XDG_CONFIG_HOME is the app's own config directory, which all of
# its sandboxes share. Off holds at a login: the request helper reads the file
# itself and asks the portal for no autostart, which removes the entry; and
# `--background`, which an entry that is left still runs, starts no notesd
# while the file exists.
#
# FROSTNOTES_LIBEXEC_DIR, when set, is the directory of
# frostnotes-request-background: /app/libexec in the Flatpak, and a test's
# own directory in its tests.
#
# A notesd that this sandbox started would die with it when the launcher
# exits. So the launcher has the Flatpak portal start `frostnotes --notesd` in
# a new sandbox of this app, and then ends the flatpak-spawn process it made
# once that notesd answers: flatpak-spawn forwards catchable signals to the
# command it spawned, but only SIGKILL ends flatpak-spawn alone, and the
# portal keeps the spawned notesd without --watch-bus. That notesd lives on
# until logout. There is no fallback in this sandbox. Without the portal the
# launcher says so in notesd.log, and `notesctl status` shows that line.
#
# --latest-version makes the portal start the command from the newest
# installed deployment of this app, never from the one this launcher runs.
# An update replaces the notesd that runs, only forward: when its build
# (`notesctl hello --build`, the app-commit of its deployment) differs from
# the latest deployment's (`notesd -build`, run the same way), the launcher
# stops it with `notesctl shutdown`, which returns once notesd has released
# the data directory's lock, and starts the latest. Every call that waits on
# notesd or on the portal's spawn is bounded
# (docs/adr/0032-an-update-replaces-the-running-notesd.md).

set -u

libexec="${FROSTNOTES_LIBEXEC_DIR:-/app/libexec}"
request_background="$libexec/frostnotes-request-background"

state="${XDG_STATE_HOME:-$HOME/.local/state}/frostnotes"

usage() {
    cat <<'EOF'
Usage: flatpak run org.frostyard.Frostnotes [--notesd | --background | --help]

Frostnotes has no window: notesd syncs the vault folders that an editor of
Markdown files, Obsidian among them, edits. With no option this starts
notesd, asks the desktop to start it at login, and shows its status.

  --notesd       run notesd and nothing else
  --background   run notesd unless background activity is off (the
                 autostart entry's command line)
  --help         show this

Set it up with notesctl:

  alias notesctl='flatpak run --command=notesctl org.frostyard.Frostnotes'
  notesctl vault add ~/Notes            register a folder as a vault
  notesctl hub join URL NAME            pair with a hub and sync the vault
  notesctl status                       notesd, its vaults and their sync
  notesctl background on|off            start notesd at login, or not
EOF
}

# bounded SECONDS COMMAND...: run the command for at most SECONDS (coreutils
# timeout, which the GNOME runtime has). Every call that waits on notesd or
# on the portal's spawn goes through it, so a notesd that accepts and never
# answers, or a portal that hangs, never keeps the launcher from finishing.
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

case "${1:-}" in
--notesd)
    run_notesd
    ;;
--background)
    # The autostart entry's command line. With the setting off, a login
    # starts no notesd at all.
    if [ -e "$off" ]; then
        mkdir -p "$state"
        echo "frostnotes: background activity is off (notesctl background on turns it on); notesd does not start at login" >>"$state/notesd.log"
        exit 0
    fi
    run_notesd
    ;;
--help | -h)
    usage
    exit 0
    ;;
"") ;;
*)
    echo "frostnotes: unknown option '$1': Frostnotes has no window; see --help" >&2
    usage >&2
    exit 2
    ;;
esac

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
    # the timeout does not kill, so a stuck query would hold the launcher
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
        echo "frostnotes: replacing the notesd of build $running with the latest build, $latest"
        bounded 35 notesctl shutdown >>"$state/notesd.log" 2>&1 || true
    fi
fi

if ! hello; then
    echo "frostnotes: starting notesd"
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
    # holds its own stdout and stderr. This runs on every path: no
    # flatpak-spawn outlives the launcher.
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
# It stays on the bus until the portal answers (it may ask the user once; at
# most 10 minutes), and writes that answer to $state/background.json, which
# `notesctl status` shows. The launcher waits for it: with no window, this
# sandbox ends when the launcher exits, and the portal drops a request whose
# caller left the bus.
echo "frostnotes: asking the desktop to start notesd at login"
"$request_background" >>"$state/background.log" 2>&1

exec notesctl status
