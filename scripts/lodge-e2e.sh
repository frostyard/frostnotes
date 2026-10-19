#!/usr/bin/env bash
# Builds lodge's development build (lodge's docs/specs/lodge-http.md, The
# development build) at a pinned commit, for M4's evidence on loopback
# (docs/plans/0006-m4-notes-inside-lodge.md, make m4-evidence): the e2e
# binary, which signs every browser in as the e2e identity, lodgectl, and
# the shell. Run it in the nsl machine, which has Go and pnpm:
#
#   scripts/lodge-e2e.sh REF OUT
#
# REF is a full commit hash, which pins what is built; OUT gets src/ (the
# checkout), lodge-e2e, lodgectl and shell/ (the built shell). A build of
# the same REF is kept.
set -euo pipefail

ref=${1:?usage: lodge-e2e.sh REF OUT}
out=${2:?usage: lodge-e2e.sh REF OUT}
[[ $ref =~ ^[0-9a-f]{40}$ ]] || { echo "lodge-e2e.sh: REF must be a full commit hash, not $ref" >&2; exit 2; }

mkdir -p "$out"
out=$(cd "$out" && pwd)
if [[ -f $out/ref && "$(cat "$out/ref")" == "$ref" && -x $out/lodge-e2e && -f $out/shell/index.html ]]; then
	echo "lodge's development build at $ref is in $out"
	exit 0
fi
rm -rf "${out:?}"/*
mkdir -p "$out/src"
src=$out/src
# A checkout of its own: the fetched commit's hash is what pins it.
git -C "$src" init -q
git -C "$src" fetch -q --depth 1 https://github.com/frostyard/lodge "$ref"
git -C "$src" -c advice.detachedHead=false checkout -q FETCH_HEAD
[[ "$(git -C "$src" rev-parse HEAD)" == "$ref" ]] || { echo "lodge-e2e.sh: fetched $(git -C "$src" rev-parse HEAD), not $ref" >&2; exit 1; }

(cd "$src" && CGO_ENABLED=0 go build -trimpath -tags e2e -o "$out/lodge-e2e" ./cmd/lodge)
(cd "$src" && CGO_ENABLED=0 go build -trimpath -o "$out/lodgectl" ./cmd/lodgectl)
(cd "$src/shell" && pnpm install --frozen-lockfile --silent && pnpm run build)
cp -R "$src/shell/dist" "$out/shell"
echo "$ref" >"$out/ref"
echo "lodge's development build at $ref: $out"
