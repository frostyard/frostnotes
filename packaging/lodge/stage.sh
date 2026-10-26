#!/bin/sh
# Stage notesd as lodge's notes app under DESTDIR, in the /usr layout
# (frostyard/core ADR-0058; docs/specs/lodge-app.md, What notesd installs):
# notesd and notesctl in usr/bin, the manifest with VERSION, the web build
# when BUILD/web holds one, the units and the sysusers.d entry. Nothing
# goes outside usr/. make lodge-package runs it.
#
#   packaging/lodge/stage.sh DESTDIR VERSION [BUILD]
#
# BUILD (default build) holds notesd and notesctl (make build) and web/
# (make web-build). DESTDIR may be a lodge root that other apps and lodge
# itself share, so this removes nothing of theirs: only the app's own
# directory is replaced, so a web build's stale files go, and every other
# file it stages is overwritten in place. DESTDIR must not be empty or /.
set -eu

usage="usage: packaging/lodge/stage.sh DESTDIR VERSION [BUILD]"
dest=${1:?$usage}
version=${2:?$usage}
build=${3:-build}
here=$(cd "$(dirname "$0")" && pwd)
pkg=$(dirname "$here")

if [ "$(realpath -m -- "$dest")" = / ]; then
  echo "stage.sh: refusing to stage into /; install from a staged root instead" >&2
  exit 1
fi
case $version in
  *[!A-Za-z0-9._+-]*) echo "stage.sh: version $version has characters a manifest cannot carry" >&2; exit 1 ;;
esac

app=$dest/usr/share/lodge/apps/notes
rm -rf -- "$app"
install -Dm755 "$build/notesd" "$dest/usr/bin/notesd"
install -Dm755 "$build/notesctl" "$dest/usr/bin/notesctl"
install -d "$app"
sed "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" "$here/app.json" > "$app/app.json"
if [ -d "$build/web" ]; then cp -R "$build/web" "$app/web"; fi
install -Dm644 "$pkg/systemd/frostnotes-lodge.socket" "$dest/usr/lib/systemd/system/frostnotes-lodge.socket"
install -Dm644 "$pkg/systemd/frostnotes.service" "$dest/usr/lib/systemd/system/frostnotes.service"
install -Dm644 "$pkg/sysusers.d/frostnotes.conf" "$dest/usr/lib/sysusers.d/frostnotes.conf"
