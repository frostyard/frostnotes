#!/bin/sh
# Check a built .deb against frostyard/core ADR-0011 and ADR-0058: the
# control fields, a payload of /usr only, root ownership, no group- or
# world-writable entry, and the expected files. make deb-check runs it.
#
#   packaging/deb-check.sh FILE PACKAGE VERSION
set -eu

usage="usage: packaging/deb-check.sh FILE PACKAGE VERSION"
deb=${1:?$usage}
pkg=${2:?$usage}
version=${3:?$usage}

fail() {
  echo "deb-check: $*" >&2
  exit 1
}

[ -f "$deb" ] || fail "$deb is missing"

field() { dpkg-deb -f "$deb" "$1"; }
[ "$(field Package)" = "$pkg" ] || fail "Package is $(field Package), want $pkg"
[ "$(field Version)" = "$version" ] || fail "Version is $(field Version), want $version"
[ "$(field Architecture)" = amd64 ] || fail "Architecture is $(field Architecture), want amd64"

listing=$(mktemp)
trap 'rm -f "$listing"' EXIT
dpkg-deb -c "$deb" > "$listing"
cat "$listing"

# Each line: mode owner size date time path [-> target].
while read -r mode owner _ _ _ path _; do
  case $path in
    ./usr/ | ./usr/*) ;;
    *) fail "$path is outside ./usr/" ;;
  esac
  [ "$owner" = root/root ] || fail "$path is owned by $owner, want root/root"
  case $mode in
    ?????w????* | ????????w?*) fail "$path is group- or world-writable ($mode)" ;;
  esac
done < "$listing"

has() { awk -v p="$1" '$6 == p { found = 1 } END { exit !found }' "$listing"; }
for p in \
  ./usr/bin/notesd \
  ./usr/bin/notesctl \
  ./usr/lib/systemd/system/frostnotes.service \
  ./usr/lib/systemd/system/frostnotes-lodge.socket \
  ./usr/lib/sysusers.d/frostnotes.conf \
  ./usr/share/lodge/apps/notes/app.json \
  ./usr/share/lodge/apps/notes/web/index.html; do
  has "$p" || fail "$p is missing"
done

manifest=$(dpkg-deb --fsys-tarfile "$deb" | tar -xOf - ./usr/share/lodge/apps/notes/app.json)
case $manifest in
  *"\"version\": \"$version\""*) ;;
  *) fail "app.json does not carry version $version" ;;
esac

echo "deb-check: $deb is ok"
