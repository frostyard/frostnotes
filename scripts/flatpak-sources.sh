#!/bin/sh
# Regenerate the Flatpak's offline sources (make flatpak-sources;
# docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md):
#
#   packaging/flatpak/cargo-sources.json   the app shell's crates, from app/src-tauri/Cargo.lock
#   packaging/flatpak/node-sources.json    the app's pnpm store, from app/pnpm-lock.yaml
#   packaging/flatpak/go-sources.json      notesd's Go modules, from go.mod and go.sum
#
# Run it after any change to those 3 lockfiles; CI regenerates the files and
# fails when they differ. It needs the network, git, Go and Python 3 on the
# host, and no Node, pnpm or Rust. The generators are flatpak-builder-tools at
# the commit below plus a patch (packaging/flatpak/patches), run with a
# virtual environment in build/ that holds their hashed dependencies
# (packaging/flatpak/generator-requirements.txt).
set -eu

# flatpak-builder-tools, pinned to a commit (frostyard/core ADR-0023).
TOOLS_COMMIT=9d86d336a39b9af38b86f114b6ce5820fe260f91
TOOLS_URL=https://github.com/flatpak/flatpak-builder-tools.git
# A patch to the node generator: the `#path:` fix of git-hosted packages.
PATCH=packaging/flatpak/patches/pnpm-git-path.patch
# The generators' dependencies: exact versions with hashes.
REQUIREMENTS=packaging/flatpak/generator-requirements.txt

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
work=build/flatpak-sources
out=packaging/flatpak
mkdir -p "$work"

if [ ! -d "$work/tools/.git" ]; then
    git clone --quiet --no-checkout "$TOOLS_URL" "$work/tools"
fi
if ! git -C "$work/tools" cat-file -e "$TOOLS_COMMIT^{commit}" 2>/dev/null; then
    git -C "$work/tools" fetch --quiet origin "$TOOLS_COMMIT"
fi
git -C "$work/tools" checkout --quiet --force --detach "$TOOLS_COMMIT"
if [ "$(git -C "$work/tools" rev-parse HEAD)" != "$TOOLS_COMMIT" ]; then
    echo "flatpak-sources: flatpak-builder-tools is not at $TOOLS_COMMIT" >&2
    exit 1
fi

# The generator writes a pnpm store of the files of a whole git-hosted
# tarball, where pnpm adds only the subdirectory a `#path:` names
# (the lodge packages). The patch fixes that; the checkout above is clean.
git -C "$work/tools" apply "$root/$PATCH"

if [ ! -x "$work/venv/bin/python" ]; then
    python3 -m venv "$work/venv"
fi
"$work/venv/bin/python" -m pip install --quiet --disable-pip-version-check --require-hashes -r "$REQUIREMENTS"

# The generators cache what they fetch; keep that out of the user's home.
export XDG_CACHE_HOME="$root/$work/cache"

"$work/venv/bin/python" "$work/tools/cargo/flatpak-cargo-generator.py" \
    app/src-tauri/Cargo.lock -o "$out/cargo-sources.json"

# pnpm 12's lockfile uses store version v11 (see the generator's README).
PYTHONPATH="$work/tools/node" "$work/venv/bin/python" -m flatpak_node_generator --no-requests-cache --pnpm-store-version v11 \
    -o "$out/node-sources.json" pnpm app/pnpm-lock.yaml

go run ./tools/flatpakgo >"$out/go-sources.json.tmp"
mv "$out/go-sources.json.tmp" "$out/go-sources.json"
