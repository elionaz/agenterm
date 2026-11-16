#!/usr/bin/env bash
# Prints the Homebrew cask for a release (packaging/homebrew/agenterm.rb.tmpl).
#
#   scripts/render-cask.sh <version> <sha256 of the macos-universal zip> [signed=true|false]
#
# Unsigned (ad-hoc) builds get a caveat explaining how to get past Gatekeeper.
set -euo pipefail

[[ $# -ge 2 && $# -le 3 ]] || { echo "usage: $0 <version> <sha256> [true|false]" >&2; exit 1; }
VERSION=${1#v}
SHA=$2
SIGNED=${3:-true}
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "invalid version: $1" >&2; exit 1; }
[[ $SHA =~ ^[0-9a-f]{64}$ ]] || { echo "invalid sha256: $SHA" >&2; exit 1; }

CAVEATS=""
if [[ $SIGNED != true ]]; then
	CAVEATS='
  caveats <<~EOS
    This build of agenterm is not signed with a Developer ID nor notarized,
    so Gatekeeper will refuse to open it. After installing, run:
      xattr -dr com.apple.quarantine #{appdir}/agenterm.app
  EOS
'
fi

TMPL=$(cd "$(dirname "$0")/.." && pwd)/packaging/homebrew/agenterm.rb.tmpl
while IFS= read -r line || [[ -n $line ]]; do
	line=${line//@VERSION@/$VERSION}
	line=${line//@SHA256@/$SHA}
	if [[ $line == @CAVEATS@* ]]; then
		printf '%s' "$CAVEATS"
		line=${line#@CAVEATS@}
	fi
	printf '%s\n' "$line"
done <"$TMPL"
