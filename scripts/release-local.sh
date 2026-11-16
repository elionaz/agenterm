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
#   SPARKLE_PUBLIC_ED_KEY   Sparkle EdDSA public key (base64) for SUPublicEDKey.
#                           Empty: the app is built with its updater off.
#   SPARKLE_PRIVATE_ED_KEY  the matching private key (base64, as exported by
#                           Sparkle's `generate_keys -x`): signs the zip and
#                           enables dist/appcast.xml. Empty: no appcast.
#   SPARKLE_KEY_ROTATION    non-empty: allow a SPARKLE_PUBLIC_ED_KEY that is not
#                           the private key's pair (docs/development.md#in-app-updates-sparkle).
#
# Sparkle (scripts/fetch-sparkle.sh: pinned, SHA-256 verified, cached in
# .cache/sparkle/) is embedded at Contents/Frameworks/Sparkle.framework.
#
# Output (dist/):
#   agenterm-<v>-macos-universal.zip   agenterm.app (CLI at Contents/Helpers/agenterm)
#   appcast.xml                        Sparkle feed for that zip (with SPARKLE_PRIVATE_ED_KEY)
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

SPARKLE_PUBLIC_ED_KEY=${SPARKLE_PUBLIC_ED_KEY:-}
SPARKLE_PRIVATE_ED_KEY=${SPARKLE_PRIVATE_ED_KEY:-}
# An EdDSA (ed25519) key is 32 bytes; Sparkle's private-key export is the
# 32-byte seed or, from older versions, 64 or 96 bytes. Checked up front so
# a mangled secret fails the build instead of shipping an app that rejects
# every update.
b64len() { printf '%s' "$1" | base64 -D 2>/dev/null | wc -c | tr -d ' '; }
if [[ -n $SPARKLE_PUBLIC_ED_KEY && $(b64len "$SPARKLE_PUBLIC_ED_KEY") != 32 ]]; then
	die "SPARKLE_PUBLIC_ED_KEY is not a base64 ed25519 public key"
fi
if [[ -n $SPARKLE_PRIVATE_ED_KEY ]]; then
	case $(b64len "$SPARKLE_PRIVATE_ED_KEY") in 32 | 64 | 96) ;; *) die "SPARKLE_PRIVATE_ED_KEY is not a base64 ed25519 private key" ;; esac
fi

rm -rf "$DIST"
mkdir -p "$WORK"

step "sparkle: fetch"
SPARKLE_DIR=$(scripts/fetch-sparkle.sh)
[[ $SPARKLE_DIR != *" "* ]] || die "Sparkle path must not contain spaces: $SPARKLE_DIR"

step "app: wails build darwin/universal ($VERSION, $COMMIT)"
# -tags sparkle builds app/updater_sparkle_darwin.go. Sparkle is weak-linked
# (the app still starts if the framework is missing) and found through
# @rpath = Contents/Frameworks. Only this build gets these flags (not the
# CLI), and cgo does not allow them in #cgo lines.
(cd app && CGO_CFLAGS="$CGO_CFLAGS -F$SPARKLE_DIR" \
	CGO_LDFLAGS="$CGO_LDFLAGS -F$SPARKLE_DIR -Wl,-weak_framework,Sparkle -Wl,-rpath,@executable_path/../Frameworks" \
	wails build -platform darwin/universal -clean -trimpath -m -skipbindings -tags sparkle -ldflags "$LDFLAGS")
ditto app/build/bin/agenterm.app "$APP"
mkdir -p "$APP/Contents/Frameworks"
ditto "$SPARKLE_DIR/Sparkle.framework" "$APP/Contents/Frameworks/Sparkle.framework"

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
if [[ -n $SPARKLE_PUBLIC_ED_KEY ]]; then
	/usr/libexec/PlistBuddy -c "Set :SUPublicEDKey $SPARKLE_PUBLIC_ED_KEY" "$PLIST"
else
	echo "release: warning: SPARKLE_PUBLIC_ED_KEY not set: in-app updates are disabled in this build" >&2
	/usr/libexec/PlistBuddy -c "Delete :SUPublicEDKey" "$PLIST"
fi
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
# signsparkle [codesign options] <path>: Sparkle's nested code. Hardened
# runtime but none of the app's entitlements; Developer ID or ad-hoc.
signsparkle() {
	if [[ -n $SIGN_IDENTITY ]]; then
		codesign --force --options runtime --timestamp \
			${SIGN_KEYCHAIN:+--keychain "$SIGN_KEYCHAIN"} --sign "$SIGN_IDENTITY" "$@"
	else
		codesign --force --sign - "$@"
	fi
}

# Inside-out. Sparkle first, in the order of its documentation for
# Developer ID apps (https://sparkle-project.org/documentation/sandboxing/#code-signing):
# the XPC services (Downloader keeps its own entitlements), Autoupdate,
# Updater.app, then the framework itself. No --deep: each piece is signed
# explicitly.
FW=$APP/Contents/Frameworks/Sparkle.framework
signsparkle "$FW/Versions/B/XPCServices/Installer.xpc"
signsparkle --preserve-metadata=entitlements "$FW/Versions/B/XPCServices/Downloader.xpc"
signsparkle "$FW/Versions/B/Autoupdate"
signsparkle "$FW/Versions/B/Updater.app"
signsparkle "$FW"
# Then the CLI (also what goes in the darwin tarball), then the bundle,
# which signs the main executable (app + daemon) and seals the rest.
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

# The final (stapled) zip is what Sparkle downloads: sign it with the EdDSA
# key and describe it in appcast.xml.
APPCAST=$DIST/appcast.xml
if [[ -n $SPARKLE_PRIVATE_ED_KEY ]]; then
	step "appcast: EdDSA-sign the zip"
	KEYFILE=$(mktemp "${TMPDIR:-/tmp}/agenterm-ed.XXXXXX")
	KDIR=
	trap 'rm -f "$KEYFILE"; [[ -z $KDIR ]] || rm -rf "$KDIR"' EXIT
	chmod 600 "$KEYFILE"
	printf '%s' "$SPARKLE_PRIVATE_ED_KEY" >"$KEYFILE" # builtin: never in argv or the log
	ED_SIG=$("$SPARKLE_DIR/bin/sign_update" --ed-key-file "$KEYFILE" -p "$ZIP")
	"$SPARKLE_DIR/bin/sign_update" --verify --ed-key-file "$KEYFILE" "$ZIP" "$ED_SIG"
	rm -f "$KEYFILE"
	# The app checks updates against SUPublicEDKey: make sure it is the
	# private key's pair (a mismatch would make every update fail). Except
	# in a key rotation release, which is signed with the old key (what the
	# installed apps trust) and ships the new public key.
	if [[ -n ${SPARKLE_KEY_ROTATION:-} ]]; then
		echo "release: warning: SPARKLE_KEY_ROTATION set: SUPublicEDKey is not checked against the signing key" >&2
	elif [[ -n $SPARKLE_PUBLIC_ED_KEY ]]; then
		OSSL=
		for o in /opt/homebrew/opt/openssl@3/bin/openssl /usr/local/opt/openssl@3/bin/openssl openssl; do
			if [[ $("$o" version 2>/dev/null) == "OpenSSL 3"* ]]; then
				OSSL=$o
				break
			fi
		done
		if [[ -n $OSSL ]]; then
			KDIR=$(mktemp -d)
			{
				echo "-----BEGIN PUBLIC KEY-----"
				# SubjectPublicKeyInfo for ed25519 = fixed 12-byte prefix + the raw key.
				{ printf '\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00'; printf '%s' "$SPARKLE_PUBLIC_ED_KEY" | base64 -D; } | base64
				echo "-----END PUBLIC KEY-----"
			} >"$KDIR/pub.pem"
			printf '%s' "$ED_SIG" | base64 -D >"$KDIR/sig"
			"$OSSL" pkeyutl -verify -pubin -inkey "$KDIR/pub.pem" -rawin -in "$ZIP" -sigfile "$KDIR/sig" >/dev/null ||
				die "SPARKLE_PUBLIC_ED_KEY does not match SPARKLE_PRIVATE_ED_KEY"
			rm -rf "$KDIR"
			echo "SUPublicEDKey matches the signing key"
		else
			echo "release: warning: no OpenSSL 3: SUPublicEDKey not checked against the signing key" >&2
		fi
	else
		echo "release: warning: appcast signed, but this app has no SUPublicEDKey (in-app updates are off in it)" >&2
	fi
	step "appcast: $APPCAST"
	scripts/render-appcast.sh "$VERSION" "$ZIP" "$ED_SIG" "$MIN_MACOS" >"$APPCAST"
	xmllint --noout "$APPCAST"
else
	echo "release: warning: SPARKLE_PRIVATE_ED_KEY not set: no appcast.xml (existing installs will not be offered this update)" >&2
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
		echo "appcast=$([[ -f $APPCAST ]] && echo true || echo false)"
	} >>"$GITHUB_OUTPUT"
fi

lipo -info "$APP/Contents/MacOS/agenterm" "$APP/Contents/Helpers/agenterm"
LINKS=$(otool -L "$APP/Contents/MacOS/agenterm") # not piped into grep -q: SIGPIPE under pipefail
[[ $LINKS == *'@rpath/Sparkle.framework'* ]] || die "app binary does not link Sparkle"
"$APP/Contents/Helpers/agenterm" --version
echo "release: $VERSION done (signed=$SIGNED notarized=$NOTARIZE) -> $DIST"
