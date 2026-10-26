#!/usr/bin/env python3
"""Check a built .deb (frostyard/core ADR-0011, ADR-0058).

    packaging/deb-check.py DEB PACKAGE VERSION [--exec PATH]... [--file PATH]...
                           [--manifest PATH]

It fails unless:
  - the control fields are Package PACKAGE, Version VERSION, Architecture
    amd64 and Depends systemd, and the control archive holds only the
    regular files control and md5sums (so no maintainer scripts, of any
    member type);
  - every data member, read by its exact name, is a directory or a regular
    file whose path is /usr or under /usr (no absolute name, no "." or ".."
    component). No package needs a link, a hard link or a device, so none
    is allowed, and no link can lead out of /usr;
  - every member is owned by uid 0 and gid 0, and none is group- or
    world-writable;
  - each --exec PATH is a regular file with mode 0755 and a nonzero size;
  - each --file PATH is a regular file with a nonzero size;
  - the --manifest PATH is a JSON object whose top-level "version" is VERSION.

It prints the archive's listing, so that CI logs show it. Python's standard
library and dpkg-deb are all it needs.
"""

import argparse
import io
import json
import stat
import subprocess
import sys
import tarfile

# The control members nFPM writes for a package with no /etc files and no
# scripts; anything else (a preinst, a postinst, a trigger, a conffiles list)
# is refused, whatever its member type, because a sysext runs no script.
CONTROL_MEMBERS = {"control", "md5sums"}


def dpkg_deb(*args: str) -> bytes:
    return subprocess.run(["dpkg-deb", *args], check=True, capture_output=True).stdout


def member_path(name: str) -> str | None:
    """Return the absolute install path of an archive member, or None when
    its name is absolute or has a "." or ".." component past the leading "./"."""
    if name.startswith("/"):
        return None
    parts = name.split("/")
    if parts and parts[0] == ".":
        parts = parts[1:]
    if parts and parts[-1] == "":
        parts = parts[:-1]  # a directory's trailing slash
    if any(p in ("", ".", "..") for p in parts):
        return None
    return "/" + "/".join(parts)


def under_usr(path: str) -> bool:
    return path == "/usr" or path.startswith("/usr/")


def check(deb: str, package: str, version: str, execs: list[str], files: list[str], manifest: str | None) -> list[str]:
    errors = []

    fields = {}
    for field in ("Package", "Version", "Architecture", "Depends"):
        fields[field] = dpkg_deb("-f", deb, field).decode().strip()
    want = {"Package": package, "Version": version, "Architecture": "amd64", "Depends": "systemd"}
    for field, value in want.items():
        if fields[field] != value:
            errors.append(f"{field} is {fields[field]!r}, want {value!r}")

    with tarfile.open(fileobj=io.BytesIO(dpkg_deb("--ctrl-tarfile", deb))) as ctrl:
        seen = set()
        for m in ctrl.getmembers():
            path = member_path(m.name)
            if path == "/" and m.isdir():
                continue  # the archive's own "./"
            name = path[1:] if path else None
            if name not in CONTROL_MEMBERS or not m.isreg():
                errors.append(f"control member {m.name!r} is not allowed (only the files control and md5sums; no maintainer scripts)")
            else:
                seen.add(name)
        if "control" not in seen:
            errors.append("the control archive has no control file")

    entries = {}
    with tarfile.open(fileobj=io.BytesIO(dpkg_deb("--fsys-tarfile", deb))) as data:
        for m in data.getmembers():
            link = f" -> {m.linkname}" if m.issym() else ""
            print(f"{stat.filemode(m.mode | _type_bits(m))} {m.uid}/{m.gid} {m.size:>9} {m.name}{link}")
            path = member_path(m.name)
            if path is None or not under_usr(path):
                errors.append(f"member {m.name!r} is not under /usr")
                continue
            if not (m.isdir() or m.isreg()):
                errors.append(f"{path} is not a directory or a regular file (no links)")
            if m.uid != 0 or m.gid != 0:
                errors.append(f"{path} is owned by {m.uid}/{m.gid}, want 0/0")
            if m.mode & 0o022:
                errors.append(f"{path} is group- or world-writable ({oct(m.mode)})")
            if path in entries:
                errors.append(f"{path} is in the archive twice")
            entries[path] = m

        for path in execs:
            m = entries.get(path)
            if m is None or not m.isreg():
                errors.append(f"{path} is missing or not a regular file")
            elif m.mode & 0o7777 != 0o755 or m.size == 0:
                errors.append(f"{path} is not a nonempty file with mode 0755 ({oct(m.mode)}, {m.size} bytes)")
        for path in files:
            m = entries.get(path)
            if m is None or not m.isreg():
                errors.append(f"{path} is missing or not a regular file")
            elif m.size == 0:
                errors.append(f"{path} is empty")
        if manifest is not None:
            m = entries.get(manifest)
            if m is None or not m.isreg():
                errors.append(f"{manifest} is missing or not a regular file")
            else:
                try:
                    doc = json.loads(data.extractfile(m).read())
                except ValueError as e:
                    errors.append(f"{manifest} is not JSON: {e}")
                else:
                    got = doc.get("version") if isinstance(doc, dict) else None
                    if got != version:
                        errors.append(f"{manifest} has version {got!r}, want {version!r}")
    return errors


def _type_bits(m: tarfile.TarInfo) -> int:
    if m.isdir():
        return 0o040000
    if m.issym():
        return 0o120000
    if m.isreg():
        return 0o100000
    return 0


def main() -> int:
    p = argparse.ArgumentParser(description="Check a built .deb (frostyard/core ADR-0011, ADR-0058).")
    p.add_argument("deb")
    p.add_argument("package")
    p.add_argument("version")
    p.add_argument("--exec", dest="execs", action="append", default=[], metavar="PATH")
    p.add_argument("--file", dest="files", action="append", default=[], metavar="PATH")
    p.add_argument("--manifest", metavar="PATH")
    a = p.parse_args()
    try:
        errors = check(a.deb, a.package, a.version, a.execs, a.files, a.manifest)
    except (OSError, subprocess.CalledProcessError, tarfile.TarError) as e:
        detail = getattr(e, "stderr", b"") or b""
        print(f"deb-check: {a.deb}: {e} {detail.decode(errors='replace').strip()}", file=sys.stderr)
        return 1
    for e in errors:
        print(f"deb-check: {a.deb}: {e}", file=sys.stderr)
    if errors:
        return 1
    print(f"deb-check: {a.deb} is ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
