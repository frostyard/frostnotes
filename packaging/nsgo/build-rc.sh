#!/usr/bin/env bash
# Build the preview Flatpak and complete Lodge app tree from one committed revision.
set -euo pipefail
root=$(git rev-parse --show-toplevel)
revision=$(git -C "$root" rev-parse HEAD)
out=$(realpath -m "${1:-$root/build/nsgo-rc}")
id=org.frostyard.Frostnotes.Preview
release=$(git -C "$root" show "$revision:cmd/frostnotes-nsgo/nsgo.yaml" | sed -n 's/^version: //p')
version="$release-rc.${revision:0:12}"
name="frostnotes-$version"
test "$(uname -sm)" = 'Linux x86_64'
mkdir -p "$out"
if [[ -e "$out/$name" ]]; then
    echo "Candidate already exists at $out/$name" >&2
    exit 1
fi
# Keep the source under the checkout so nsl sees it at the same path.
mkdir -p "$root/build"
stage=$(mktemp -d "$root/build/.nsgo-rc.XXXXXXXX")
result=$(mktemp -d "$out/.candidate.XXXXXXXX")
trap 'rm -rf -- "$stage" "$result"' EXIT
mkdir "$stage/src"
git -C "$root" archive "$revision" | tar -x -C "$stage/src"
go_root=$(go env GOROOT)
export PATH="$go_root/bin:$PATH" GOWORK=off
cd "$stage/src"
# Lodge's web app is the React app (make lodge-app; docs/adr/0050-frostnotes-has-no-desktop-window.md).
make lodge-app VERSION="$version" SOURCE_REVISION="$revision"
printf '%s\n' "$revision" > "$result/revision.txt"
tar -czf "$result/$name-lodge.tar.gz" -C build/lodge/apps notes

(cd cmd/frostnotes-nsgo && go tool nsgo build -target linux_x64 -version "$release" -channel none -installers=false -output "$stage/dist")
mv "$stage/dist/FrostNotes-$release-linux_x64" "$stage/native"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$stage/native/" ./cmd/notesd ./cmd/notesctl
cp LICENSE packaging/flatpak/{frostnotes.sh,request-background.py} "$result/revision.txt" "$stage/native/"
cp "packaging/nsgo/$id.json" "$stage/"
release_date=$(git -C "$root" show -s --format=%cs "$revision")
sed "s/@VERSION@/$version/; s/@DATE@/$release_date/" "packaging/nsgo/$id.metainfo.xml" > "$stage/native/$id.metainfo.xml"
cd "$stage"
if command -v flatpak-builder >/dev/null; then
    builder=(flatpak-builder)
else
    builder=(flatpak run "--filesystem=$stage" org.flatpak.Builder)
fi
"${builder[@]}" --disable-download --repo=repo app "$id.json"
flatpak build-bundle --runtime-repo=https://flathub.org/repo/flathub.flatpakrepo repo "$result/$name-x86_64.flatpak" "$id" preview
(cd "$result" && sha256sum -- *.flatpak *.tar.gz revision.txt > SHA256SUMS)
mv -T "$result" "$out/$name"
printf 'Candidate: %s\nSource: %s\n' "$out/$name" "$revision"
