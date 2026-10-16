#!/usr/bin/env python3
"""Render the checked-in application PNGs from the SVG master.

Requires Python, PyGObject, GdkPixbuf 2.0 and its librsvg SVG loader.
Run from any directory: python3 scripts/render-icons.py
"""

from pathlib import Path

import gi

gi.require_version("GdkPixbuf", "2.0")
from gi.repository import GdkPixbuf


def main():
    icons = Path(__file__).resolve().parents[1] / "app/src-tauri/icons"
    source = (icons / "icon.svg").read_bytes()
    for size, filename in ((32, "32x32.png"), (128, "128x128.png"), (512, "icon.png")):
        loader = GdkPixbuf.PixbufLoader.new_with_type("svg")
        loader.set_size(size, size)
        loader.write(source)
        loader.close()
        pixbuf = loader.get_pixbuf()
        pixbuf.savev(str(icons / filename), "png", [], [])
        print(f"Rendered {filename}: {size} × {size}")


if __name__ == "__main__":
    main()
