#!/usr/bin/env python3
"""The Flatpak metainfo's screenshots (make screenshots, run by run.sh): drive
the real Frostnotes UI (Vite on 127.0.0.1:5173, notesd on the made-up vault)
in WebKitGTK, the engine of the desktop app, through WebKitWebDriver, and save
1280x800 PNGs to the directory named by the first argument. Python's standard
library only: WebDriver is plain HTTP and JSON."""
import base64
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

OUT = sys.argv[1]
URL = "http://127.0.0.1:5173/"
D = "http://127.0.0.1:4445"
EL = "element-6066-11e4-a52e-4f735466cecf"


def req(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    r = urllib.request.Request(D + path, data=data, method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            return json.load(resp)["value"]
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path}: {e.read().decode()}") from None


def js(script, *args):
    return req("POST", S + "/execute/sync", {"script": script, "args": list(args)})


FIND = """
const [name, sel] = arguments;
for (const el of document.querySelectorAll(sel || 'button,[role=tab],[role=treeitem],a,select,input')) {
  const label = (el.getAttribute('aria-label') || el.textContent || '').trim();
  if (label === name && el.getClientRects().length) return el;
}
return null;
"""


def find(name, sel=None, timeout=20):
    end = time.time() + timeout
    while time.time() < end:
        el = js(FIND, name, sel)
        if el:
            return el[EL]
        time.sleep(0.2)
    raise RuntimeError(f"nothing named {name!r}")


def click(name, sel=None):
    req("POST", f"{S}/element/{find(name, sel)}/click", {})
    time.sleep(0.6)


def wait_text(text, timeout=20):
    end = time.time() + timeout
    while time.time() < end:
        if js("return document.body.innerText.includes(arguments[0])", text):
            return
        time.sleep(0.2)
    raise RuntimeError(f"no text {text!r}")


def shot(name):
    js("document.activeElement && document.activeElement.blur(); window.getSelection().removeAllRanges();")
    time.sleep(1.0)
    png = base64.b64decode(req("GET", S + "/screenshot"))
    with open(os.path.join(OUT, name), "wb") as f:
        f.write(png)
    print("saved", name, len(png), flush=True)


def start_session():
    for _ in range(50):
        try:
            req("GET", "/status")
            break
        except (OSError, RuntimeError):
            time.sleep(0.2)
    sid = req("POST", "/session", {"capabilities": {"alwaysMatch": {
        "browserName": "MiniBrowser",
        "webkitgtk:browserOptions": {
            "binary": "/usr/lib/x86_64-linux-gnu/webkit2gtk-4.1/MiniBrowser",
            "args": ["--automation"],
        },
    }}})["sessionId"]
    return f"/session/{sid}"


# The driver stops on every exit, and the session ends when it began.
drv = subprocess.Popen(["WebKitWebDriver", "--port=4445"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
S = None
try:
    S = start_session()
    # A viewport of 1280x800: size the window, then correct for the toolbar.
    req("POST", S + "/window/rect", {"x": 0, "y": 0, "width": 1280, "height": 800})
    for _ in range(3):
        w, h = js("return [window.innerWidth, window.innerHeight]")
        rect = req("GET", S + "/window/rect")
        req("POST", S + "/window/rect", {"width": rect["width"] + 1280 - w, "height": rect["height"] + 800 - h})
        time.sleep(0.5)
    print("viewport", js("return [window.innerWidth, window.innerHeight]"), flush=True)

    req("POST", S + "/url", {"url": URL})
    find("Projects", "[role=treeitem]", 60)

    # 1. The editor: a note with properties, tasks, a table and backlinks.
    click("Projects", "[role=treeitem]")
    click("Cabin Weekend", "[role=treeitem]")
    wait_text("Linked mentions")
    shot("editor.png")

    # 2. Search across the vault, with the recipe open.
    click("Recipes", "[role=treeitem]")
    click("Winter Squash Soup", "[role=treeitem]")
    wait_text("Roast two squash")
    click("Search", "[role=tab],button")
    inp = js("return document.querySelector('aside input')")[EL]
    req("POST", f"{S}/element/{inp}/value", {"text": "soup\ue007"})
    wait_text("results")
    shot("search.png")

    # 3. Every open task in the vault, beside the trip plan.
    click("Files", "[role=tab],button")
    click("Cabin Weekend", "[role=treeitem]")
    wait_text("Days")
    click("Tasks", "[role=tab],button")
    wait_text("Order seed garlic")
    shot("tasks.png")

    # 4. The dark theme, reading a note with its outline.
    click("Settings", "button")
    click("Appearance", "button,[role=tab]")
    sel = js("return [...document.querySelectorAll('select')].find(s => [...s.options].some(o => o.value.toLowerCase() === 'dark'))")
    js("""const s = arguments[0];
const o = [...s.options].find(o => o.value.toLowerCase() === 'dark');
Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, o.value);
s.dispatchEvent(new Event('change', {bubbles: true}));""", sel)
    time.sleep(0.5)
    req("POST", f"{S}/actions", {"actions": [{"type": "key", "id": "k", "actions": [
        {"type": "keyDown", "value": "\ue00c"}, {"type": "keyUp", "value": "\ue00c"}]}]})
    time.sleep(0.5)
    click("Reading", "[role=treeitem]")
    click("The Quiet Orchard", "[role=treeitem]")
    wait_text("apple orchard")
    click("Outline", "[role=tab],button")
    shot("dark.png")
finally:
    try:
        if S is not None:
            req("DELETE", S)
    finally:
        drv.terminate()
        drv.wait(timeout=10)
