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
# The fake flatpak-spawn fails, and records the fault, when the launcher calls
# it without --latest-version.
#
# FROSTNOTES_BIN_DIR is where frostnotes, notesd and notesctl are; the
# default is the Flatpak's /app/bin. The test cleans up after itself.

set -eu

bin="${FROSTNOTES_BIN_DIR:-/app/bin}"

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
mkdir -p "$work/run" "$work/home" "$work/data" "$work/vault" "$work/bin" "$work/libexec" "$state"
chmod 0700 "$work/run"
export XDG_RUNTIME_DIR="$work/run" HOME="$work/home" XDG_DATA_HOME="$work/data"
export XDG_STATE_HOME="$work/state"
export FLATPAK_ID=org.frostyard.Frostnotes
# No session bus: the Secret Service is absent, as in the build sandbox.
export DBUS_SESSION_BUS_ADDRESS=unix:path="$work/no-bus"
unset FROSTNOTES_SOCKET FROSTNOTES_DATA_DIR FROSTNOTES_FLATPAK_INFO

# A fake flatpak-spawn, first on PATH. It drops --latest-version (the launcher
# must pass it: without the flag, the real portal would run the window's own,
# perhaps old, build), and runs the command with FROSTNOTES_FLATPAK_INFO
# pointing at the "latest" deployment's info file, which the test controls.
# For `frostnotes` it records its own PID and its child's, as the portal's
# spawn does in a sandbox of its own, and waits for the child. The app and the
# Background helper exit at once.
cat >"$work/bin/flatpak-spawn" <<'EOF'
#!/bin/sh
if [ "${1:-}" != "--latest-version" ]; then
    echo "fake flatpak-spawn: called without --latest-version: $*" | tee -a "$SMOKE_SPAWN_FAULT" >&2
    exit 2
fi
shift
export FROSTNOTES_FLATPAK_INFO="$SMOKE_LATEST_INFO"
if [ "${1:-}" = "frostnotes" ]; then
    echo "$$" >"$SMOKE_SPAWN_PID"
    "$@" &
    echo "$!" >"$SMOKE_CHILD_PID"
    wait "$!"
else
    exec "$@"
fi
EOF
chmod +x "$work/bin/flatpak-spawn"
printf '#!/bin/sh\nexit 0\n' >"$work/libexec/frostnotes-app"
printf '#!/bin/sh\nexit 0\n' >"$work/libexec/frostnotes-request-background"
chmod +x "$work/libexec/frostnotes-app" "$work/libexec/frostnotes-request-background"
SMOKE_SPAWN_PID="$work/spawn.pid"
SMOKE_CHILD_PID="$work/notesd.pid"
SMOKE_SPAWN_FAULT="$work/spawn.fault"
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

if [ -e "$SMOKE_SPAWN_FAULT" ]; then
    echo "smoke test: the launcher called flatpak-spawn without --latest-version" >&2
    print_logs
    exit 1
fi

kill -TERM "$pid"
# Wait until it has gone, before the cleanup removes its directories.
wait_gone "$pid" || true
pid=
echo "smoke test passed"
