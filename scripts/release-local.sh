#!/usr/bin/env bash
# Builds every release artifact into dist/. Used by .github/workflows/release.yml
# and for local dry runs:
#
#   scripts/release-local.sh 0.1.0          # ad-hoc signed, not notarized
#
# Optional environment (the workflow sets these from secrets):
#   SIGN_IDENTITY     codesign identity (name or SHA-1), e.g.
#                     "Developer ID Application: Jane Doe (TEAMID)". Empty: ad-hoc.
#   SIGN_KEYCHAIN     keychain holding that identity (default: search list).
#   NOTARY_KEY_PATH   App Store Connect API key (.p8) } all three, plus
#   NOTARY_KEY_ID     its key id                      } SIGN_IDENTITY, enable
#   NOTARY_ISSUER_ID  its issuer id                   } notarization + stapling.
#
# Output (dist/):
#   agenterm-<v>-macos-universal.zip   agenterm.app (CLI at Contents/Helpers/agenterm)
#   agenterm-<v>-macos-universal.dmg   the same app + /Applications link
#   agenterm-<v>-darwin-universal.tar.gz, agenterm-<v>-linux-{amd64,arm64}.tar.gz   CLI
#   checksums.txt                      sha256 of all of the above
set -euo pipefail

die() { echo "release: $*" >&2; exit 1; }
step() { echo "==> $*"; }

[[ $# -eq 1 ]] || die "usage: $0 <version>   (e.g. 0.1.0 or v0.1.0)"
VERSION=${1#v}
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || die "invalid version: $1"
[[ $(uname -s) == Darwin ]] || die "must run on macOS"
for t in wails lipo codesign ditto hdiutil shasum /usr/libexec/PlistBuddy; do
	command -v "$t" >/dev/null || die "missing tool: $t"
done

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
COMMIT=$(git rev-parse --short=7 HEAD)
DIST=$ROOT/dist
WORK=$DIST/work
APP=$WORK/agenterm.app
ENTITLEMENTS=$ROOT/app/build/darwin/entitlements.plist
MIN_MACOS=13.0
BASE=agenterm-$VERSION

SIGN_IDENTITY=${SIGN_IDENTITY:-}
NOTARIZE=false
if [[ -n $SIGN_IDENTITY && -n ${NOTARY_KEY_PATH:-} && -n ${NOTARY_KEY_ID:-} && -n ${NOTARY_ISSUER_ID:-} ]]; then
	NOTARIZE=true
fi

PKG=github.com/elionaz/agenterm/internal/version
LDFLAGS="-s -w -X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT"
# One deployment target for Go, cgo and Info.plist (wails appends 10.13 only
# when -mmacosx-version-min is absent).
export MACOSX_DEPLOYMENT_TARGET=$MIN_MACOS
export CGO_CFLAGS="${CGO_CFLAGS:-} -mmacosx-version-min=$MIN_MACOS"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -mmacosx-version-min=$MIN_MACOS"

rm -rf "$DIST"
mkdir -p "$WORK"

step "app: wails build darwin/universal ($VERSION, $COMMIT)"
(cd app && wails build -platform darwin/universal -clean -trimpath -m -skipbindings -ldflags "$LDFLAGS")
ditto app/build/bin/agenterm.app "$APP"

step "cli: darwin arm64 + amd64 (cgo) -> universal"
for arch in arm64 amd64; do
	CGO_ENABLED=1 GOOS=darwin GOARCH=$arch \
		go build -trimpath -ldflags "$LDFLAGS" -o "$WORK/agenterm-darwin-$arch" ./cmd/agenterm
done
lipo -create -output "$WORK/agenterm" "$WORK/agenterm-darwin-arm64" "$WORK/agenterm-darwin-amd64"
rm "$WORK/agenterm-darwin-arm64" "$WORK/agenterm-darwin-amd64"

step "cli: linux amd64 + arm64 (CGO_ENABLED=0)"
for arch in amd64 arm64; do
	mkdir -p "$WORK/linux-$arch"
	CGO_ENABLED=0 GOOS=linux GOARCH=$arch \
		go build -trimpath -ldflags "$LDFLAGS" -o "$WORK/linux-$arch/agenterm" ./cmd/agenterm
done

step "app: Info.plist version $VERSION, bundle the CLI"
PLIST=$APP/Contents/Info.plist
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$PLIST"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $VERSION" "$PLIST"
/usr/libexec/PlistBuddy -c "Set :LSMinimumSystemVersion $MIN_MACOS" "$PLIST"
mkdir -p "$APP/Contents/Helpers"

# sign <path>: Developer ID with hardened runtime, or ad-hoc.
sign() {
	if [[ -n $SIGN_IDENTITY ]]; then
		codesign --force --options runtime --timestamp --entitlements "$ENTITLEMENTS" \
			${SIGN_KEYCHAIN:+--keychain "$SIGN_KEYCHAIN"} --sign "$SIGN_IDENTITY" "$1"
	else
		codesign --force --sign - "$1"
	fi
}

if [[ -n $SIGN_IDENTITY ]]; then
	step "sign: $SIGN_IDENTITY (hardened runtime)"
else
	step "sign: ad-hoc (no SIGN_IDENTITY)"
fi
# Inside-out: the CLI first (also what goes in the darwin tarball), then the
# bundle, which signs the main executable (app + daemon) and seals the rest.
sign "$WORK/agenterm"
cp "$WORK/agenterm" "$APP/Contents/Helpers/agenterm"
sign "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
codesign --verify --strict "$WORK/agenterm"

# notarize <file>: submit and wait; on rejection print the log and fail.
notarize() {
	local out id status
	out=$(xcrun notarytool submit "$1" --key "$NOTARY_KEY_PATH" --key-id "$NOTARY_KEY_ID" \
		--issuer "$NOTARY_ISSUER_ID" --wait --timeout 30m --output-format json) || true
	echo "$out"
	id=$(jq -r '.id // empty' <<<"$out")
	status=$(jq -r '.status // empty' <<<"$out")
	if [[ $status != Accepted ]]; then
		[[ -n $id ]] && xcrun notarytool log "$id" --key "$NOTARY_KEY_PATH" \
			--key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER_ID" || true
		die "notarization of $(basename "$1") failed (status: ${status:-unknown})"
	fi
}

ZIP=$DIST/$BASE-macos-universal.zip
DMG=$DIST/$BASE-macos-universal.dmg

# No xattrs/resource forks in the zip: as ._* entries they would become stray
# files inside the bundle (breaking its seal) when extracted with plain unzip.
zipapp() { ditto -c -k --norsrc --noextattr --noqtn --keepParent "$APP" "$ZIP"; }

step "package: zip"
zipapp
if $NOTARIZE; then
	step "notarize: app"
	notarize "$ZIP"
	xcrun stapler staple "$APP"
	xcrun stapler validate "$APP"
	rm "$ZIP"
	zipapp
fi

step "package: dmg"
STAGE=$WORK/dmg
mkdir -p "$STAGE"
ditto "$APP" "$STAGE/agenterm.app"
ln -s /Applications "$STAGE/Applications"
hdiutil create -quiet -volname "agenterm $VERSION" -srcfolder "$STAGE" -fs HFS+ \
	-format UDZO -ov "$DMG"
if [[ -n $SIGN_IDENTITY ]]; then
	codesign --force --timestamp ${SIGN_KEYCHAIN:+--keychain "$SIGN_KEYCHAIN"} \
		--sign "$SIGN_IDENTITY" "$DMG"
fi
if $NOTARIZE; then
	step "notarize: dmg"
	notarize "$DMG"
	xcrun stapler staple "$DMG"
	xcrun stapler validate "$DMG"
fi

step "package: cli tarballs"
tarball() { # <name> <dir containing agenterm>
	local d=$WORK/tar/$1
	mkdir -p "$d"
	cp "$2/agenterm" LICENSE README.md "$d/"
	COPYFILE_DISABLE=1 tar --no-mac-metadata -C "$WORK/tar" -czf "$DIST/$1.tar.gz" "$1"
}
tarball "$BASE-darwin-universal" "$WORK"
tarball "$BASE-linux-amd64" "$WORK/linux-amd64"
tarball "$BASE-linux-arm64" "$WORK/linux-arm64"

step "checksums"
(cd "$DIST" && shasum -a 256 ./*.zip ./*.dmg ./*.tar.gz | sed 's# \./# #' >checksums.txt)
cat "$DIST/checksums.txt"

SIGNED=false
[[ -n $SIGN_IDENTITY ]] && SIGNED=true
ZIP_SHA=$(shasum -a 256 "$ZIP" | cut -d' ' -f1)
if [[ -n ${GITHUB_OUTPUT:-} ]]; then
	{
		echo "version=$VERSION"
		echo "signed=$SIGNED"
		echo "notarized=$NOTARIZE"
		echo "zip_sha256=$ZIP_SHA"
	} >>"$GITHUB_OUTPUT"
fi

lipo -info "$APP/Contents/MacOS/agenterm" "$APP/Contents/Helpers/agenterm"
"$APP/Contents/Helpers/agenterm" --version
echo "release: $VERSION done (signed=$SIGNED notarized=$NOTARIZE) -> $DIST"
