#!/usr/bin/env python3
"""Ask the Background portal to run `frostnotes --background` at login.

`gdbus call` does not work for this: it exits as soon as the portal
returns the request's handle, and the portal closes a request when its
caller leaves the bus, before the user answers. This helper subscribes to
the request's Response signal first, makes the call, and stays on the bus
in a GLib main loop until the Response arrives or 10 minutes pass.

The helper reads the app's setting itself: while
`${XDG_CONFIG_HOME:-~/.config}/frostnotes/background-off` exists, the
setting "Keep syncing when no window is open" is off, and the request asks
for no autostart, so the portal removes the autostart entry
(docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md). It
takes an exclusive lock on
`${XDG_STATE_HOME:-~/.local/state}/frostnotes/background.lock` before it
reads the setting, and holds the lock until it has written its answer, so
two helpers never undo each other. A setting that changes while it waits
for the portal makes it ask again, at most 3 times in all: the answer that
the app reads is the last request's.

Whatever happens, the helper writes its outcome to
`$XDG_STATE_HOME/frostnotes/` for the app to read, and exits 0: an answer
to `background.json`, and a failure that could not take the lock to
`background-failure-<generation>.json`. A missing portal or a refusal
changes nothing else for the app.

Each request reads the shell's counter of changes as `generation`, under
its lock and immediately before it reads the setting; the last request's is
the one in the outcome. A failure reads the counter when it writes. A
failure that cannot take the lock never writes `background.json`, so it
cannot replace a newer answer: it writes `background-failure-<generation>.json`
beside the answer, and removes the failures of lower generations. The app
shows the newest of the two, and an answer wins a tie. A helper that did
take the lock removes, after it wrote its answer, the failures up to that
answer's generation. The app compares those numbers with the one its own
change left, so it can tell an answer about the new setting from an answer
about an old one (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md).
"""

import fcntl
import json
import os
import sys
import tempfile
import time

TIMEOUT_SECONDS = 600
MAX_REQUESTS = 3
REASON = "Keep your notes in sync without a window"
COMMANDLINE = ["frostnotes", "--background"]

PORTAL = "org.freedesktop.portal.Desktop"
PORTAL_PATH = "/org/freedesktop/portal/desktop"

OUTCOME_NAME = "background.json"
LOCK_NAME = "background.lock"
GEN_NAME = "background-gen"
FAILURE_PREFIX = "background-failure-"
FAILURE_SUFFIX = ".json"


def log(message):
    print(f"frostnotes-request-background: {message}", file=sys.stderr, flush=True)


def state_dir():
    """The app's state directory, as the launcher's `state`."""
    base = os.environ.get("XDG_STATE_HOME")
    if not base:
        base = os.path.join(os.path.expanduser("~"), ".local", "state")
    return os.path.join(base, "frostnotes")


def generation():
    """The shell's counter of changes; a missing or bad file counts 0."""
    try:
        with open(os.path.join(state_dir(), GEN_NAME), encoding="utf-8") as handle:
            return int(handle.read().strip())
    except (OSError, ValueError):
        return 0


def off_file():
    """The app's setting: it exists while "Keep syncing" is off."""
    base = os.environ.get("XDG_CONFIG_HOME")
    if not base:
        base = os.path.join(os.path.expanduser("~"), ".config")
    return os.path.join(base, "frostnotes", "background-off")


def autostart_wanted():
    """The portal should start notesd at login while the setting is on."""
    return not os.path.exists(off_file())


def lock():
    """Serialize the helpers, and hold the lock until the answer is written.

    A helper that waits for the portal holds the lock, and the next one
    waits for it: so the last setting wins, and the answer belongs to the
    setting that the last request asked about.
    """
    directory = state_dir()
    os.makedirs(directory, exist_ok=True)
    handle = open(os.path.join(directory, LOCK_NAME), "a", encoding="utf-8")
    fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
    return handle


def failure_name(gen):
    """The name of the failure of generation gen."""
    return f"{FAILURE_PREFIX}{gen}{FAILURE_SUFFIX}"


def failure_generation(name):
    """The generation of a failure's name, or None for any other name.

    Only the exact form `background-failure-<digits>.json` counts.
    """
    if not name.startswith(FAILURE_PREFIX) or not name.endswith(FAILURE_SUFFIX):
        return None
    digits = name[len(FAILURE_PREFIX) : -len(FAILURE_SUFFIX)]
    if not digits.isascii() or not digits.isdigit():
        return None
    return int(digits)


def remove_failures(up_to):
    """Remove the failures of the generations up to up_to, best effort.

    Neither a failure that no longer exists nor one that cannot be removed
    changes what the app must know.
    """
    try:
        names = os.listdir(state_dir())
    except OSError:
        return
    for name in names:
        gen = failure_generation(name)
        if gen is None or gen > up_to:
            continue
        try:
            os.unlink(os.path.join(state_dir(), name))
        except OSError:
            pass


def write_outcome(outcome, name=OUTCOME_NAME):
    """Write the outcome atomically, and answer whether it is there.

    The app reads the file at any time.
    """
    directory = state_dir()
    path = os.path.join(directory, name)
    temp = None
    try:
        os.makedirs(directory, exist_ok=True)
        fd, temp = tempfile.mkstemp(dir=directory, prefix=".background-", suffix=".tmp")
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            json.dump(outcome, handle)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp, path)
        temp = None
    except OSError as err:
        log(f"cannot write {path}: {err}")
        return False
    finally:
        if temp is not None:
            try:
                os.unlink(temp)
            except OSError:
                pass
    return True


def fail(outcome, message):
    """Note a failure, for the log and for the app."""
    log(message)
    outcome["error"] = message


def flag(results, key):
    """One boolean of the portal's results; None when it is absent."""
    value = results.get(key)
    if value is None:
        return None
    unpack = getattr(value, "unpack", None)
    if unpack is not None:
        value = unpack()
    return bool(value)


def request(autostart, generation_value):
    """Ask the portal once, and answer the outcome of that request."""
    outcome = {
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "generation": generation_value,
        "autostartRequested": autostart,
        "response": None,
        "background": None,
        "autostart": None,
        "error": None,
    }
    ask(outcome, autostart)
    return outcome


def ask(outcome, autostart):
    """Ask the portal once and fill outcome with its answer."""
    try:
        from gi.repository import Gio, GLib
    except (ImportError, ValueError) as err:
        fail(outcome, f"no PyGObject: {err}")
        return

    try:
        bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    except GLib.Error as err:
        fail(outcome, f"no session bus: {err.message}")
        return

    loop = GLib.MainLoop()
    answer = {}

    def on_response(_conn, _sender, _path, _iface, _signal, params, _data):
        code, results = params.unpack()
        answer["code"] = code
        answer["results"] = results
        loop.quit()

    def subscribe(request_path):
        return bus.signal_subscribe(
            PORTAL,
            "org.freedesktop.portal.Request",
            "Response",
            request_path,
            None,
            Gio.DBusSignalFlags.NONE,
            on_response,
            None,
        )

    # The request's path is predictable: subscribe before the call, so a
    # fast Response cannot be missed (the portals documentation's advice).
    token = f"frostnotes{os.getpid()}"
    sender = bus.get_unique_name().lstrip(":").replace(".", "_")
    predicted = f"{PORTAL_PATH}/request/{sender}/{token}"
    subscribe(predicted)

    options = {
        "handle_token": GLib.Variant("s", token),
        "reason": GLib.Variant("s", REASON),
        "autostart": GLib.Variant("b", autostart),
        "commandline": GLib.Variant("as", COMMANDLINE),
    }
    try:
        reply = bus.call_sync(
            PORTAL,
            PORTAL_PATH,
            "org.freedesktop.portal.Background",
            "RequestBackground",
            GLib.Variant("(sa{sv})", ("", options)),
            GLib.VariantType("(o)"),
            Gio.DBusCallFlags.NONE,
            -1,
            None,
        )
    except GLib.Error as err:
        fail(outcome, f"RequestBackground failed: {err.message}")
        return

    handle = reply.unpack()[0]
    if handle != predicted:
        log(f"the portal chose {handle}, not {predicted}; subscribing to it")
        subscribe(handle)

    def on_timeout():
        loop.quit()
        return False

    GLib.timeout_add_seconds(TIMEOUT_SECONDS, on_timeout)
    loop.run()

    if "code" not in answer:
        fail(outcome, f"no answer in {TIMEOUT_SECONDS} s")
        return
    results = dict(answer["results"])
    log(f"response {answer['code']} (0 is granted): {results}")
    outcome["response"] = int(answer["code"])
    outcome["background"] = flag(results, "background")
    outcome["autostart"] = flag(results, "autostart")


def main():
    # A placeholder, for the paths that cannot ask: the setting is read
    # only under the lock, and each failure reads the counter when it writes.
    outcome = {
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "generation": 0,
        "autostartRequested": True,
        "response": None,
        "background": None,
        "autostart": None,
        "error": None,
    }
    try:
        handle = lock()
    except OSError as err:
        # Without the lock the helpers could undo each other. This helper
        # never writes background.json, so it cannot replace a newer
        # answer: it writes its own failure beside the answer, and the app
        # shows whichever is newer (ADR-0033).
        fail(outcome, f"cannot lock {os.path.join(state_dir(), LOCK_NAME)}: {err}")
        # The counter, read now and not when the helper started.
        outcome["generation"] = generation()
        gen = outcome["generation"]
        if write_outcome(outcome, failure_name(gen)):
            # Only now: when the write failed, the older failures hold the
            # last message the app has, and they must stay.
            remove_failures(gen - 1)
        return 0
    try:
        for _ in range(MAX_REQUESTS):
            # The counter, read under the lock just before the setting: the
            # app compares it with the number its own change left (ADR-0033).
            gen = generation()
            autostart = autostart_wanted()
            outcome = request(autostart, gen)
            if autostart_wanted() == autostart:
                break
    except Exception as err:  # the helper always exits 0
        fail(outcome, f"unexpected failure: {err}")
        outcome["generation"] = generation()
    finally:
        if write_outcome(outcome):
            # An answer wins over a failure of the same or a lower
            # generation, so those failures are never needed again.
            remove_failures(outcome["generation"])
        handle.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
