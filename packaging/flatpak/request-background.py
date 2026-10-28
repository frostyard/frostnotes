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

Whatever happens, the helper writes the outcome to
`$XDG_STATE_HOME/frostnotes/background.json` for the app to read, and
exits 0. A missing portal or a refusal changes nothing else for the app.

Each request reads the shell's counter of changes as `generation`, under
its lock and immediately before it reads the setting; the last request's is
the one in the outcome. A failure reads the counter when it writes. A
failure that cannot take the lock does not replace an answer whose
generation is at least that counter: the newer answer is the one the app
waits for. The app compares that number with the one its own change left,
so it can tell an answer about the new setting from an answer about an old
one (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md).
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


def written_generation():
    """The generation of the answer in the outcome file, or None.

    None means an answer the app cannot use: no file, one that does not
    parse, or one whose generation is no int.
    """
    try:
        with open(os.path.join(state_dir(), OUTCOME_NAME), encoding="utf-8") as handle:
            found = json.load(handle)
    except (OSError, ValueError):
        return None
    if not isinstance(found, dict):
        return None
    value = found.get("generation")
    if not isinstance(value, int) or isinstance(value, bool):
        return None
    return value


def write_outcome(outcome):
    """Write the outcome atomically: the app reads the file at any time."""
    directory = state_dir()
    path = os.path.join(directory, OUTCOME_NAME)
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
    finally:
        if temp is not None:
            try:
                os.unlink(temp)
            except OSError:
                pass


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
        # Without the lock the helpers could undo each other; the app still
        # gets an answer it can show, unless a newer answer is already there.
        fail(outcome, f"cannot lock {os.path.join(state_dir(), LOCK_NAME)}: {err}")
        # The counter, read now and not when the helper started: a helper
        # that did take the lock may have written its answer in between, and
        # an older failure must not hide it from the app (ADR-0033).
        outcome["generation"] = generation()
        newer = written_generation()
        if newer is not None and newer >= outcome["generation"]:
            log(
                f"keeps the newer answer of generation {newer} in "
                f"{os.path.join(state_dir(), OUTCOME_NAME)}"
            )
            return 0
        write_outcome(outcome)
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
        write_outcome(outcome)
        handle.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
