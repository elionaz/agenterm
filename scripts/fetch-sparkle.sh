#!/usr/bin/env bash
# Downloads the pinned Sparkle 2 distribution into .cache/sparkle/ (git
# ignored), checks its SHA-256 and prints the directory holding
# Sparkle.framework and bin/{sign_update,generate_appcast,generate_keys}.
#
#   SPARKLE_DIR=$(scripts/fetch-sparkle.sh)
#
# To upgrade: pick a release from https://github.com/sparkle-project/Sparkle/releases
# and copy the sha256 GitHub shows for Sparkle-<version>.tar.xz:
#   gh api repos/sparkle-project/Sparkle/releases/tags/<version> \
#     --jq '.assets[] | select(.name|endswith(".tar.xz")) | .digest'
set -euo pipefail

SPARKLE_VERSION=2.10.1
SPARKLE_SHA256=1a856d2a9edb8e628de88121d7be04cf0e2167f8641650b78fca00b14244cf27

die() { echo "fetch-sparkle: $*" >&2; exit 1; }

ROOT=$(cd "$(dirname "$0")/.." && pwd)
CACHE=$ROOT/.cache/sparkle
TARBALL=$CACHE/Sparkle-$SPARKLE_VERSION.tar.xz
DIR=$CACHE/$SPARKLE_VERSION
mkdir -p "$CACHE"

verify() { [[ $(shasum -a 256 "$1" | cut -d' ' -f1) == "$SPARKLE_SHA256" ]]; }

if [[ ! -f $TARBALL ]] || ! verify "$TARBALL"; then
	echo "fetch-sparkle: downloading Sparkle $SPARKLE_VERSION" >&2
	curl -fsSL --retry 3 -o "$TARBALL.part" \
		"https://github.com/sparkle-project/Sparkle/releases/download/$SPARKLE_VERSION/Sparkle-$SPARKLE_VERSION.tar.xz"
	verify "$TARBALL.part" || { rm -f "$TARBALL.part"; die "SHA-256 mismatch for Sparkle-$SPARKLE_VERSION.tar.xz"; }
	mv "$TARBALL.part" "$TARBALL"
fi

# Extracted afresh from the verified tarball on every run: what gets bundled
# is always exactly the pinned release.
rm -rf "$DIR"
mkdir -p "$DIR"
tar -xJf "$TARBALL" -C "$DIR"
[[ -d $DIR/Sparkle.framework && -x $DIR/bin/sign_update ]] || die "unexpected archive layout in $TARBALL"
codesign --verify --deep --strict "$DIR/Sparkle.framework" || die "Sparkle.framework signature is broken"
echo "$DIR"
