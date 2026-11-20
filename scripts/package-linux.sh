#!/usr/bin/env bash
# Builds the Linux app for this machine's architecture and packages it:
#   dist/panal-<v>-linux-<arch>.AppImage   (x86_64 / aarch64 inside)
#   dist/panal-<v>-linux-<arch>.deb
# Run on Linux with Go, Node, wails and the GTK/WebKit dev packages
# (libgtk-3-dev libwebkit2gtk-4.1-dev); release.yml does it on Ubuntu.
# The packages carry the app (panal-app) and the cgo-free CLI
# (panal), which the app starts the daemon with. They don't bundle GTK
# or WebKitGTK: the system's are used (libwebkit2gtk-4.1).
set -euo pipefail

die() { echo "package-linux: $*" >&2; exit 1; }
step() { echo "==> $*"; }

[[ $(uname -s) == Linux ]] || die "run on Linux"
VERSION=${1:?usage: scripts/package-linux.sh <version>}
ARCH=$(go env GOARCH)
case $ARCH in
amd64) AI_ARCH=x86_64 AI_SHA=ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0 ;;
arm64) AI_ARCH=aarch64 AI_SHA=f0837e7448a0c1e4e650a93bb3e85802546e60654ef287576f46c71c126a9158 ;;
*) die "unsupported arch $ARCH" ;;
esac
APPIMAGETOOL_VERSION=1.9.1
NFPM_VERSION=v2.48.0

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"
DIST=$ROOT/dist
WORK=$DIST/linux-$ARCH
BASE=panal-$VERSION-linux-$ARCH
COMMIT=$(git rev-parse --short HEAD)
PKG=github.com/elionaz/panal/internal/version
LDFLAGS="-s -w -X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT"
rm -rf "$WORK"
mkdir -p "$WORK"

step "cli: linux/$ARCH (CGO_ENABLED=0)"
CGO_ENABLED=0 go build -trimpath -ldflags "$LDFLAGS" -o "$WORK/panal" ./cmd/panal

step "app: wails build linux/$ARCH (webkit2gtk-4.1)"
(cd app && wails build -platform "linux/$ARCH" -clean -trimpath -m -skipbindings -tags webkit2_41 -ldflags "$LDFLAGS" -o panal-app)
cp app/build/bin/panal-app "$WORK/panal-app"

step "appimage"
APPDIR=$WORK/AppDir
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/512x512/apps"
install -m 0755 "$WORK/panal-app" "$WORK/panal" "$APPDIR/usr/bin/"
cp packaging/linux/panal.desktop "$APPDIR/usr/share/applications/"
cp packaging/linux/panal.desktop "$APPDIR/panal.desktop"
cp app/build/appicon.png "$APPDIR/usr/share/icons/hicolor/512x512/apps/panal.png"
cp app/build/appicon.png "$APPDIR/panal.png"
cat >"$APPDIR/AppRun" <<'APPRUN'
#!/bin/sh
HERE=$(dirname "$(readlink -f "$0")")
exec "$HERE/usr/bin/panal-app" "$@"
APPRUN
chmod 0755 "$APPDIR/AppRun"
TOOL=$WORK/appimagetool
curl -fsSL --retry 3 -o "$TOOL" \
	"https://github.com/AppImage/appimagetool/releases/download/$APPIMAGETOOL_VERSION/appimagetool-$AI_ARCH.AppImage"
echo "$AI_SHA  $TOOL" | sha256sum -c -
chmod +x "$TOOL"
# No FUSE on CI runners: the tool runs extracted.
APPIMAGE_EXTRACT_AND_RUN=1 ARCH=$AI_ARCH "$TOOL" --no-appstream "$APPDIR" "$DIST/$BASE.AppImage"

step "deb"
sed -e "s#@WORK@#$WORK#g" -e "s#@VERSION@#$VERSION#g" -e "s#@DEB_ARCH@#$ARCH#g" \
	packaging/linux/nfpm.yaml >"$WORK/nfpm.yaml"
go run "github.com/goreleaser/nfpm/v2/cmd/nfpm@$NFPM_VERSION" package \
	--config "$WORK/nfpm.yaml" --packager deb --target "$DIST/$BASE.deb"

step "check"
"$WORK/panal" --version
dpkg-deb --info "$DIST/$BASE.deb" | sed -n '1,20p'
ls -l "$DIST/$BASE.AppImage" "$DIST/$BASE.deb"
