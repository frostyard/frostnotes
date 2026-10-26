#!/usr/bin/env python3
"""Ask the Background portal to run `frostnotes --background` at login.

`gdbus call` does not work for this: it exits as soon as the portal
returns the request's handle, and the portal closes a request when its
caller leaves the bus, before the user answers. This helper subscribes to
the request's Response signal first, makes the call, and stays on the bus
in a GLib main loop until the Response arrives or 10 minutes pass.

It always exits 0 and logs what happened to stderr: a missing portal or a
refusal changes nothing for the app (docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
"""

import os
import sys

TIMEOUT_SECONDS = 600
REASON = "Keep your notes in sync without a window"
COMMANDLINE = ["frostnotes", "--background"]

PORTAL = "org.freedesktop.portal.Desktop"
PORTAL_PATH = "/org/freedesktop/portal/desktop"


def log(message):
    print(f"frostnotes-request-background: {message}", file=sys.stderr, flush=True)


def main():
    try:
        from gi.repository import Gio, GLib
    except (ImportError, ValueError) as err:
        log(f"no PyGObject: {err}")
        return 0

    try:
        bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    except GLib.Error as err:
        log(f"no session bus: {err.message}")
        return 0

    loop = GLib.MainLoop()
    outcome = {}

    def on_response(_conn, _sender, _path, _iface, _signal, params, _data):
        code, results = params.unpack()
        outcome["code"] = code
        outcome["results"] = results
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
        "autostart": GLib.Variant("b", True),
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
        log(f"RequestBackground failed: {err.message}")
        return 0

    handle = reply.unpack()[0]
    if handle != predicted:
        log(f"the portal chose {handle}, not {predicted}; subscribing to it")
        subscribe(handle)

    def on_timeout():
        log(f"no answer in {TIMEOUT_SECONDS} s")
        loop.quit()
        return False

    GLib.timeout_add_seconds(TIMEOUT_SECONDS, on_timeout)
    loop.run()

    if "code" in outcome:
        results = {k: v for k, v in outcome["results"].items()}
        log(f"response {outcome['code']} (0 is granted): {results}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
