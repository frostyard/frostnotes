#!/usr/bin/env bash
# Provision the nsl `frostnotes` machine (Debian 13) with the Tauri, WebKitGTK,
# Node, Rust and Go toolchains the app build needs. The host is atomic and
# cannot install -dev packages; the engine itself builds and tests on the host.
#
# Run from the repository root on the host (idempotent; safe to re-run):
#   nsl create frostnotes --distro debian:13
#   nsl run -m frostnotes --root bash dev/nsl/provision.sh "$(id -un)"
#
# nsl's --cloud-init option is specified but deferred (nsl docs/specs/provisioning.md),
# so this script is the provisioning step. Downloads are version-pinned and
# checksum-verified (frostyard/core ADR-0023); bump a pin and its checksum together.
set -euo pipefail

NODE_VERSION=24.21.0
NODE_SHA256=fd8e59d5a511510f6a298afb548f18c7d2b1be404d8b4a27d94fbe49f56cb2d6
GO_VERSION=1.27.1
GO_SHA256=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445
RUSTUP_VERSION=1.29.1
RUSTUP_SHA256=dda7234360b7f578ca8b0ddcb80145646fa61a67c1720a5abc7051b35c9fcb71
RUST_TOOLCHAIN=1.99.0

TARGET_USER=${1:?usage: provision.sh USER (the nsl account that builds the app)}
id "$TARGET_USER" >/dev/null

STATE=/var/tmp/frostnotes
TMP=$(mktemp -d)
chmod 0755 "$TMP" # rustup-init runs as TARGET_USER
trap 'rm -rf "$TMP"' EXIT

fetch() { # url sha256 dest
	curl -fsSL --retry 3 -o "$3" "$1"
	echo "$2  $3" | sha256sum -c --quiet -
}

export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -y -q --no-install-recommends \
	build-essential ca-certificates curl file git make pkg-config wget xz-utils \
	libwebkit2gtk-4.1-dev libxdo-dev libssl-dev libayatana-appindicator3-dev \
	librsvg2-dev webkit2gtk-driver xvfb xauth

if [[ "$(/opt/node/bin/node --version 2>/dev/null)" != "v$NODE_VERSION" ]]; then
	fetch "https://nodejs.org/dist/v$NODE_VERSION/node-v$NODE_VERSION-linux-x64.tar.xz" \
		"$NODE_SHA256" "$TMP/node.tar.xz"
	rm -rf "/opt/node-v$NODE_VERSION"
	mkdir -p "/opt/node-v$NODE_VERSION"
	tar -xJf "$TMP/node.tar.xz" -C "/opt/node-v$NODE_VERSION" --strip-components=1
	ln -sfn "/opt/node-v$NODE_VERSION" /opt/node
fi
ln -sf /opt/node/bin/node /opt/node/bin/npm /opt/node/bin/npx /opt/node/bin/corepack /usr/local/bin/
# pnpm comes from corepack, pinned with its hash by app/package.json's packageManager.
corepack enable --install-directory /usr/local/bin

if [[ "$(/usr/local/go/bin/go env GOVERSION 2>/dev/null)" != "go$GO_VERSION" ]]; then
	fetch "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz" "$GO_SHA256" "$TMP/go.tar.gz"
	rm -rf /usr/local/go
	tar -xzf "$TMP/go.tar.gz" -C /usr/local
fi

# Rust lives in /opt so it never touches a home directory shared with the host.
install -d -o "$TARGET_USER" /opt/rustup /opt/cargo
if [[ ! -x /opt/cargo/bin/rustup ]]; then
	fetch "https://static.rust-lang.org/rustup/archive/$RUSTUP_VERSION/x86_64-unknown-linux-gnu/rustup-init" \
		"$RUSTUP_SHA256" "$TMP/rustup-init"
	chmod 0755 "$TMP/rustup-init"
	runuser -u "$TARGET_USER" -- env RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
		"$TMP/rustup-init" -y --no-modify-path --profile minimal --default-toolchain "$RUST_TOOLCHAIN"
fi
runuser -u "$TARGET_USER" -- env RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo \
	/opt/cargo/bin/rustup toolchain install "$RUST_TOOLCHAIN" --profile minimal

# Build output and package stores stay machine-local: virtiofs is slow for them
# and SQLite WAL must not live on /mnt/host (docs/adr/0009-development-environment-and-gates.md).
install -d -o "$TARGET_USER" "$STATE" "$STATE/target" "$STATE/pnpm-store" "$STATE/notesd"

cat >/etc/profile.d/frostnotes.sh <<'EOF'
# Written by frostnotes dev/nsl/provision.sh.
export RUSTUP_HOME=/opt/rustup CARGO_HOME=/opt/cargo
export PATH=/opt/cargo/bin:/opt/node/bin:/usr/local/go/bin:$PATH
export CARGO_TARGET_DIR=/var/tmp/frostnotes/target
export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
# Host file edits raise no inotify events inside nsl machines; watchers poll.
export NSL=1
EOF

echo "frostnotes machine provisioned: node $NODE_VERSION, go $GO_VERSION, rust $RUST_TOOLCHAIN"
