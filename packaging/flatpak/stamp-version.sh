#!/bin/sh
# Stamps a release version into the Flatpak's build files, on a v* tag run.
#
# The Flatpak's notesd reports the version it was built with: `notesd -version`
# prints it, and rpc.hello sends it, so a copy installed from the OCI image
# names the release it holds. Software centres show the metainfo's newest
# release, which is how the release reaches a user's installed app. So the tag's
# version goes into the ldflags of the manifest, and with the UTC date into the
# metainfo's <releases>. The committed files keep "flatpak" and 0.0.0: a build
# of main names itself flatpak, and no release is listed before one exists.
#
# Usage: stamp-version.sh VERSION COMMIT [DIR]
#   VERSION  the tag without its v, as MAJOR.MINOR.PATCH (0.2.0)
#   COMMIT   the tag's commit, 7 to 40 lowercase hex digits
#   DIR      holds org.frostyard.Frostnotes.json and
#            org.frostyard.Frostnotes.metainfo.xml; the script's own
#            directory when omitted.
#
# The files are edited in place, and the script stops at the first doubt: a
# version it has already stamped, a manifest without the text it replaces, or
# a metainfo without exactly one <releases>.
set -eu

die() {
	printf 'stamp-version.sh: %s\n' "$*" >&2
	exit 1
}

[ "$#" -ge 2 ] && [ "$#" -le 3 ] || die 'usage: stamp-version.sh VERSION COMMIT [DIR]'

version=$1
commit=$2
dir=${3:-$(dirname "$0")}

printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' ||
	die "version '$version' is not MAJOR.MINOR.PATCH"
printf '%s\n' "$commit" | grep -Eq '^[0-9a-f]{7,40}$' ||
	die "commit '$commit' is not 7 to 40 lowercase hex digits"

manifest=$dir/org.frostyard.Frostnotes.json
metainfo=$dir/org.frostyard.Frostnotes.metainfo.xml
[ -f "$manifest" ] || die "no manifest at $manifest"
[ -f "$metainfo" ] || die "no metainfo at $metainfo"

# Every check comes before the first edit, so a run refused by a check changes
# nothing. A failure during the edits (an I/O error) can leave the manifest
# stamped and the metainfo not: start again from fresh copies. In CI the
# failed step stops the job, so nothing is built or published.
old='-X main.version=flatpak'
new="-X main.version=$version -X main.commit=$commit"
found=$(grep -oF -- "$old" "$manifest" | wc -l)
[ "$found" -eq 1 ] || die "$manifest holds '$old' $found times, want exactly once"
found=$(grep -oF '<releases>' "$metainfo" | wc -l)
[ "$found" -eq 1 ] || die "$metainfo holds <releases> $found times, want exactly once"
if grep -Fq "version=\"$version\"" "$metainfo"; then
	die "$metainfo already lists version $version"
fi

# The manifest: name the version and the commit in notesd's ldflags, replacing
# the word that a main build reports.
sed -i "s/-X main\.version=flatpak/-X main.version=$version -X main.commit=$commit/" "$manifest"
grep -Fq -- "$new" "$manifest" || die "$manifest did not take the stamp"
if grep -Fq -- "$old" "$manifest"; then
	die "$manifest still holds '$old'"
fi

# The metainfo: the new release goes first, so it is the newest that
# appstreamcli and software centres show.
today=$(date -u +%Y-%m-%d)
release="<release version=\"$version\" date=\"$today\"/>"
# Keep the indentation of the release lines that are there; a metainfo with
# none gets two more spaces than its <releases>.
indent=$(sed -n 's/^\([[:space:]]*\)<release[[:space:]].*$/\1/p' "$metainfo" | head -n 1)
if [ -z "$indent" ]; then
	base=$(sed -n 's/^\([[:space:]]*\)<releases>.*$/\1/p' "$metainfo")
	indent=$base'  '
fi
sed -i "/<releases>/a\\
$indent$release" "$metainfo"
grep -Fq -- "$release" "$metainfo" || die "$metainfo did not take the release"

printf 'stamped %s (%s) into %s\n' "$version" "$commit" "$dir"
