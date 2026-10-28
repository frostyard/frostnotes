#!/bin/sh
# The Flatpak's smoke test, run by flatpak-builder's test-commands: the
# launcher's normal path starts notesd through flatpak-spawn, as the
# Background portal does at login, and leaves no flatpak-spawn behind; a
# second notesd on the same data directory exits; a note written through
# notesctl reads back. The portal and the app are fakes, since the build
# sandbox has no portal and no display. It needs no Secret Service (notesd
# keeps its secrets in a file without one).
#
# An update replaces the engine, only forward (docs/adr/0032-an-update-replaces-the-running-notesd.md):
# once a notesd of build A runs, the fake flatpak-spawn answers
# `--latest-version notesd -build` with build B, and the launcher stops that
# notesd (`notesctl shutdown`) and starts one that reports B, with the note
# written before the stop still there. A notesd of the latest build is kept.
# A latest-build query that leaves a detached process holding its output, as
# the portal does, and prints no build: the launcher must not wait for the
# process, must keep the running notesd and must run the app within its bounds.
# The fake flatpak-spawn fails, and records the fault, when the launcher calls
# it without --latest-version.
#
# The setting for background activity
# (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md) is the
# off file: the launcher starts the request helper with no arguments, since
# the helper reads the setting itself; and `frostnotes --background`, the
# autostart entry's command line, starts no notesd and exits 0 while the file
# exists. A window's spawn of notesd must use `--notesd`, which the fake
# flatpak-spawn requires.
#
# FROSTNOTES_BIN_DIR is where frostnotes, notesd and notesctl are; the
# default is the Flatpak's /app/bin. The test cleans up after itself.

set -eu

bin="${FROSTNOTES_BIN_DIR:-/app/bin}"

work="$(mktemp -d)"
pid=
cleanup() {
    # The detached process of the latest-build query case, when it still runs.
    if [ -s "$work/detached.pid" ]; then
        kill -KILL "$(cat "$work/detached.pid")" 2>/dev/null || true
    fi
    if [ -n "$pid" ]; then
        kill -TERM "$pid" 2>/dev/null || true
        wait "$pid" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT

state="$work/state/frostnotes"

# print_logs shows what notesd and the launcher said, for a failure.
print_logs() {
    if [ -f "$state/notesd.log.1" ]; then
        echo "--- $state/notesd.log.1, its end" >&2
        tail -n 20 "$state/notesd.log.1" | tr -d '\000' >&2 || true
    fi
    for f in "$state/notesd.log" "$work/launcher.log"; do
        if [ -f "$f" ]; then
            echo "--- $f" >&2
            cat "$f" >&2
        fi
    done
}

# A private runtime directory (mode 0700, as the real one is), home and data.
mkdir -p "$work/run" "$work/home" "$work/data" "$work/vault" "$work/bin" "$work/libexec" "$work/config" "$state"
chmod 0700 "$work/run"
export XDG_RUNTIME_DIR="$work/run" HOME="$work/home" XDG_DATA_HOME="$work/data"
export XDG_STATE_HOME="$work/state" XDG_CONFIG_HOME="$work/config"
export FLATPAK_ID=org.frostyard.Frostnotes
# No session bus: the Secret Service is absent, as in the build sandbox.
export DBUS_SESSION_BUS_ADDRESS=unix:path="$work/no-bus"
unset FROSTNOTES_SOCKET FROSTNOTES_DATA_DIR FROSTNOTES_FLATPAK_INFO

# A fake flatpak-spawn, first on PATH. It drops --latest-version (the launcher
# must pass it: without the flag, the real portal would run the window's own,
# perhaps old, build), and runs the command with FROSTNOTES_FLATPAK_INFO
# pointing at the "latest" deployment's info file, which the test controls.
# For `frostnotes` it requires --notesd (a window's spawn must not take the
# autostart's --background path, which the setting can turn off), records its
# own PID and its child's, as the portal's spawn does in a sandbox of its own,
# and waits for the child. The app and the Background helper exit at once.
cat >"$work/bin/flatpak-spawn" <<'EOF'
#!/bin/sh
if [ "${1:-}" != "--latest-version" ]; then
    echo "fake flatpak-spawn: called without --latest-version: $*" | tee -a "$SMOKE_SPAWN_FAULT" >&2
    exit 2
fi
shift
export FROSTNOTES_FLATPAK_INFO="$SMOKE_LATEST_INFO"
if [ "${1:-}" = "frostnotes" ]; then
    if [ "${2:-}" != "--notesd" ]; then
        echo "fake flatpak-spawn: a window's notesd must start with --notesd: $*" | tee -a "$SMOKE_SPAWN_FAULT" >&2
        exit 2
    fi
    echo "$$" >"$SMOKE_SPAWN_PID"
    "$@" &
    echo "$!" >"$SMOKE_CHILD_PID"
    wait "$!"
elif [ -n "${SMOKE_SPAWN_DETACH:-}" ] && [ "$*" = "notesd -build" ]; then
    # A process in a session of its own that keeps this command's output open
    # and says nothing, as the portal's sandbox does; the query itself ends.
    setsid sh -c 'echo $$ >"$1"; exec sleep 60' sh "$SMOKE_DETACHED_PID" &
    # Return only once that process has written its PID, so the test reads
    # it and its cleanup can end it (at most 5 s).
    n=0
    while [ ! -s "$SMOKE_DETACHED_PID" ] && [ "$n" -lt 100 ]; do
        sleep 0.05
        n=$((n + 1))
    done
    exit 0
else
    exec "$@"
fi
EOF
chmod +x "$work/bin/flatpak-spawn"
printf '#!/bin/sh\nexit 0\n' >"$work/libexec/frostnotes-app"
# The Background helper records its arguments: the test checks that the
# launcher starts it with none, since it reads the setting itself.
cat >"$work/libexec/frostnotes-request-background" <<'EOF'
#!/bin/sh
printf 'args:%s\n' "$*" >>"$SMOKE_REQUEST_ARGS"
EOF
chmod +x "$work/libexec/frostnotes-app" "$work/libexec/frostnotes-request-background"
SMOKE_SPAWN_PID="$work/spawn.pid"
SMOKE_CHILD_PID="$work/notesd.pid"
SMOKE_SPAWN_FAULT="$work/spawn.fault"
SMOKE_DETACHED_PID="$work/detached.pid"
SMOKE_REQUEST_ARGS="$work/request.args"
export SMOKE_DETACHED_PID SMOKE_REQUEST_ARGS
# info FILE COMMIT writes a minimal .flatpak-info; the build is the app-commit
# of its [Instance] group.
info() {
    printf '[Application]\napp-commit=not-the-build\n\n[Instance]\napp-commit=%s\n' "$2" >"$1"
}
info "$work/info-a" buildA
info "$work/info-b" buildB
SMOKE_LATEST_INFO="$work/info-a"
export SMOKE_SPAWN_PID SMOKE_CHILD_PID SMOKE_SPAWN_FAULT SMOKE_LATEST_INFO
export FROSTNOTES_LIBEXEC_DIR="$work/libexec"
PATH="$work/bin:$bin:$PATH"
export PATH

# A notesd log over 1 MiB, as a session that ran for weeks has: the launcher
# keeps one old log and starts a new one.
dd if=/dev/zero of="$state/notesd.log" bs=1024 count=1025 2>/dev/null

"$bin/frostnotes" >"$work/launcher.log" 2>&1

for _ in $(seq 1 20); do
    if [ -s "$SMOKE_CHILD_PID" ]; then
        break
    fi
    sleep 0.25
done
spawned="$(cat "$SMOKE_SPAWN_PID" 2>/dev/null || true)"
pid="$(cat "$SMOKE_CHILD_PID" 2>/dev/null || true)"
if [ -z "$pid" ]; then
    echo "smoke test: the launcher did not start notesd through flatpak-spawn" >&2
    print_logs
    exit 1
fi
if [ -n "$spawned" ] && kill -0 "$spawned" 2>/dev/null; then
    echo "smoke test: flatpak-spawn $spawned outlived the launcher" >&2
    print_logs
    exit 1
fi

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
    echo "smoke test: notesd did not answer within 20 s" >&2
    print_logs
    exit 1
fi

if [ ! -f "$state/notesd.log.1" ]; then
    echo "smoke test: the old notesd log did not move to $state/notesd.log.1" >&2
    print_logs
    exit 1
fi
size="$(wc -c <"$state/notesd.log")"
if [ "$size" -gt 1048576 ]; then
    echo "smoke test: notesd.log holds $size bytes; want a new log of at most 1 MiB" >&2
    print_logs
    exit 1
fi

socket="$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock"
if [ ! -S "$socket" ]; then
    echo "smoke test: no socket at $socket" >&2
    find "$XDG_RUNTIME_DIR" >&2
    print_logs
    exit 1
fi

# A second `frostnotes --background` while notesd runs must not move the log
# that notesd writes: it rotates only when no notesd answers. It exits
# non-zero, and notesd's lock line lands at the end of notesd.log.
dd if=/dev/zero bs=1024 count=1025 2>/dev/null >>"$state/notesd.log"
rc=0
"$bin/frostnotes" --background >/dev/null 2>>"$work/background.log" || rc=$?
if [ "$rc" -eq 0 ]; then
    echo "smoke test: a second 'frostnotes --background' exited 0; want the lock's error" >&2
    print_logs
    exit 1
fi
if [ ! -f "$state/notesd.log" ]; then
    echo "smoke test: the running notesd's log was moved" >&2
    print_logs
    exit 1
fi
size="$(wc -c <"$state/notesd.log")"
if [ "$size" -le 1048576 ]; then
    echo "smoke test: notesd.log holds $size bytes; want the running notesd's log, over 1 MiB" >&2
    print_logs
    exit 1
fi
if ! tail -n 5 "$state/notesd.log" | grep -q "another notesd uses"; then
    echo "smoke test: notesd.log does not end with the lock's line" >&2
    print_logs
    exit 1
fi

# A second notesd on the same data directory must exit at once: notesd locks
# the directory, so the launcher can start `frostnotes --background` freely.
# Another socket shows that the lock, and not the socket check, stops it.
rc=0
FROSTNOTES_SOCKET="$work/second.sock" timeout 10 notesd >"$work/second.log" 2>&1 || rc=$?
if [ "$rc" -eq 0 ] || [ "$rc" -eq 124 ] || ! grep -q "another notesd uses" "$work/second.log"; then
    echo "smoke test: a second notesd exited with $rc, not an 'another notesd uses' error; its log:" >&2
    cat "$work/second.log" >&2
    print_logs
    exit 1
fi
notesctl hello >/dev/null

got="$(notesctl hello --build)"
if [ "$got" != "buildA" ]; then
    echo "smoke test: the first notesd reports build '$got', not buildA" >&2
    print_logs
    exit 1
fi

notesctl vault add "$work/vault" --name smoke
printf 'hello from the smoke test\n' | notesctl write --create smoke.md
got="$(notesctl cat smoke.md)"
if [ "$got" != "hello from the smoke test" ]; then
    echo "smoke test: read back '$got'" >&2
    print_logs
    exit 1
fi
if [ ! -f "$work/vault/smoke.md" ]; then
    echo "smoke test: the note is not a file in the vault" >&2
    print_logs
    exit 1
fi

# wait_gone PID: wait, for up to 10 s, until the process has gone. notesd is
# not this shell's child (flatpak-spawn started it).
wait_gone() {
    for _ in $(seq 1 40); do
        kill -0 "$1" 2>/dev/null || return 0
        sleep 0.25
    done
    return 1
}

# An update replaces the engine: the latest build is now B, and the notesd
# that runs reports A. The launcher stops it and starts one of build B
# through flatpak-spawn, which ends; the note written before is still there.
old="$pid"
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID"
SMOKE_LATEST_INFO="$work/info-b"
export SMOKE_LATEST_INFO
"$bin/frostnotes" >"$work/launcher.log" 2>&1
for _ in $(seq 1 20); do
    if [ -s "$SMOKE_CHILD_PID" ]; then
        break
    fi
    sleep 0.25
done
if [ ! -s "$SMOKE_CHILD_PID" ]; then
    echo "smoke test: the launcher did not start a notesd of the latest build" >&2
    print_logs
    exit 1
fi
spawned="$(cat "$SMOKE_SPAWN_PID" 2>/dev/null || true)"
pid="$(cat "$SMOKE_CHILD_PID")"
if [ "$pid" = "$old" ] || ! wait_gone "$old"; then
    echo "smoke test: the old notesd $old still runs" >&2
    print_logs
    exit 1
fi
if [ -n "$spawned" ] && kill -0 "$spawned" 2>/dev/null; then
    echo "smoke test: flatpak-spawn $spawned outlived the launcher" >&2
    print_logs
    exit 1
fi
got="$(notesctl hello --build 2>/dev/null || true)"
if [ "$got" != "buildB" ]; then
    echo "smoke test: the replacement reports build '$got', not buildB" >&2
    print_logs
    exit 1
fi
got="$(notesctl cat smoke.md)"
if [ "$got" != "hello from the smoke test" ]; then
    echo "smoke test: after the update the note reads back '$got'" >&2
    print_logs
    exit 1
fi
sleep 1
if ! kill -0 "$pid" 2>/dev/null; then
    echo "smoke test: the replacement notesd $pid ended with flatpak-spawn" >&2
    print_logs
    exit 1
fi

# A notesd of the latest build is kept, the same process.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID"
"$bin/frostnotes" >"$work/launcher.log" 2>&1
if [ -e "$SMOKE_SPAWN_PID" ] || ! kill -0 "$pid" 2>/dev/null || ! notesctl hello >/dev/null; then
    echo "smoke test: the launcher did not keep the notesd of the latest build" >&2
    print_logs
    exit 1
fi

# An unknown latest build keeps the running notesd too.
SMOKE_LATEST_INFO="$work/no-info"
export SMOKE_LATEST_INFO
"$bin/frostnotes" >"$work/launcher.log" 2>&1
if [ -e "$SMOKE_SPAWN_PID" ] || ! kill -0 "$pid" 2>/dev/null || ! notesctl hello >/dev/null; then
    echo "smoke test: the launcher did not keep the notesd when the latest build was unknown" >&2
    print_logs
    exit 1
fi

# A latest-build query that leaves a detached process on its output, and
# prints no build: the launcher does not wait for it, keeps the notesd and
# runs the app, all within 20 s (the query's bound is 10 s).
SMOKE_LATEST_INFO="$work/info-b"
SMOKE_SPAWN_DETACH=1
export SMOKE_LATEST_INFO SMOKE_SPAWN_DETACH
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID"
start="$(date +%s)"
rc=0
"$bin/frostnotes" >"$work/launcher.log" 2>&1 || rc=$?
took=$(($(date +%s) - start))
unset SMOKE_SPAWN_DETACH
detached="$(cat "$SMOKE_DETACHED_PID" 2>/dev/null || true)"
if [ -z "$detached" ] || ! kill -0 "$detached" 2>/dev/null; then
    echo "smoke test: the latest-build query did not leave its detached process" >&2
    print_logs
    exit 1
fi
if [ "$rc" -ne 0 ] || [ "$took" -gt 20 ]; then
    echo "smoke test: the launcher exited $rc after $took s beside a detached process on the query's output; want 0 within 20 s" >&2
    print_logs
    exit 1
fi
if [ -e "$SMOKE_SPAWN_PID" ] || ! kill -0 "$pid" 2>/dev/null || ! notesctl hello >/dev/null; then
    echo "smoke test: the launcher did not keep the notesd beside a detached process on the query's output" >&2
    print_logs
    exit 1
fi
if ls "$state"/latest-build.* >/dev/null 2>&1; then
    echo "smoke test: the launcher left a latest-build file in $state" >&2
    exit 1
fi
kill -KILL "$detached" 2>/dev/null || true

if [ -e "$SMOKE_SPAWN_FAULT" ]; then
    echo "smoke test: the launcher called flatpak-spawn without --latest-version" >&2
    print_logs
    exit 1
fi

# The app's setting for background activity is the off file
# (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md). The
# request helper reads it itself, so the launcher starts the helper with no
# arguments; and `frostnotes --background`, the autostart entry's command
# line, starts no notesd while the file exists. The launcher starts the
# helper in the background, so wait for the fake helper's record, at most 5 s.
# The second `frostnotes --background` above, with notesd running, shows the
# other way: without the file it reaches notesd's lock.
off="$XDG_CONFIG_HOME/frostnotes/background-off"
asked() {
    # asked: one launch, and the request it starts must have no arguments.
    rm -f "$SMOKE_REQUEST_ARGS"
    "$bin/frostnotes" >"$work/launcher.log" 2>&1
    for _ in $(seq 1 50); do
        if [ -s "$SMOKE_REQUEST_ARGS" ]; then
            break
        fi
        sleep 0.1
    done
    args="$(cat "$SMOKE_REQUEST_ARGS" 2>/dev/null || true)"
    if [ -z "$args" ]; then
        echo "smoke test: the launcher did not ask the Background portal" >&2
        print_logs
        exit 1
    fi
    if [ "$args" != "args:" ]; then
        echo "smoke test: the launcher started the helper with '$args'; want no arguments" >&2
        print_logs
        exit 1
    fi
}

mkdir -p "$XDG_CONFIG_HOME/frostnotes"
rm -f "$off"
asked

# With the setting off, the autostart's command line starts no notesd, says so
# in notesd.log and exits 0; the notesd that runs keeps running.
printf 'off\n' >"$off"
rc=0
"$bin/frostnotes" --background >/dev/null 2>&1 || rc=$?
if [ "$rc" -ne 0 ]; then
    echo "smoke test: 'frostnotes --background' exited $rc with the setting off; want 0" >&2
    print_logs
    exit 1
fi
off_line="frostnotes: background activity is off in Settings; notesd does not start at login"
if ! tail -n 1 "$state/notesd.log" | grep -qxF "$off_line"; then
    echo "smoke test: notesd.log does not end with '$off_line' with the setting off" >&2
    print_logs
    exit 1
fi
if ! kill -0 "$pid" 2>/dev/null; then
    echo "smoke test: 'frostnotes --background' ended the notesd that runs" >&2
    print_logs
    exit 1
fi
rm -f "$off"

kill -TERM "$pid"
# Wait until it has gone, before the cleanup removes its directories.
wait_gone "$pid" || true
pid=
echo "smoke test passed"
