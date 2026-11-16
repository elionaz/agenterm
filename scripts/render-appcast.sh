#!/usr/bin/env bash
# Prints the Sparkle appcast (a single item: the latest release) for
# scripts/release-local.sh, which uploads it as the release asset
# appcast.xml, i.e. SUFeedURL
# https://github.com/<repo>/releases/latest/download/appcast.xml.
#
#   scripts/render-appcast.sh <version> <zip> <edSignature> [min macOS]
#
# Release notes: the "## [<version>]" section of CHANGELOG.md as simple
# HTML (headings, bullet lists, `code`, **bold**), plus a link to the GitHub
# release; without such a section, Sparkle shows the release page instead.
# GITHUB_REPOSITORY (owner/name) defaults to elionaz/agenterm.
set -euo pipefail

[[ $# -ge 3 && $# -le 4 ]] || { echo "usage: $0 <version> <zip> <edSignature> [min macOS]" >&2; exit 1; }
VERSION=${1#v}
ZIP=$2
SIG=$3
MIN_MACOS=${4:-13.0}
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "invalid version: $1" >&2; exit 1; }
[[ -f $ZIP ]] || { echo "no such file: $ZIP" >&2; exit 1; }
[[ $SIG =~ ^[A-Za-z0-9+/=]+$ ]] || { echo "invalid EdDSA signature" >&2; exit 1; }

REPO=${GITHUB_REPOSITORY:-elionaz/agenterm}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
LENGTH=$(stat -f%z "$ZIP")
URL="https://github.com/$REPO/releases/download/v$VERSION/$(basename "$ZIP")"
NOTES_URL="https://github.com/$REPO/releases/tag/v$VERSION"
PUBDATE=$(LC_ALL=C date -u "+%a, %d %b %Y %H:%M:%S +0000")

# CHANGELOG section -> HTML.
NOTES=$(awk -v v="$VERSION" '
	function esc(s) { gsub(/&/, "\\&amp;", s); gsub(/</, "\\&lt;", s); gsub(/>/, "\\&gt;", s); return s }
	function inl(s,   out) {
		s = esc(s)
		while (match(s, /`[^`]+`/)) { s = substr(s, 1, RSTART - 1) "<code>" substr(s, RSTART + 1, RLENGTH - 2) "</code>" substr(s, RSTART + RLENGTH) }
		while (match(s, /\*\*[^*]+\*\*/)) { s = substr(s, 1, RSTART - 1) "<b>" substr(s, RSTART + 2, RLENGTH - 4) "</b>" substr(s, RSTART + RLENGTH) }
		return s
	}
	function endp() { if (p) { printf "</p>\n"; p = 0 } }
	function endli() { if (li) { printf "</li>\n"; li = 0 } }
	function endul() { endli(); if (ul) { print "</ul>"; ul = 0 } }
	/^## \[/ { if (on) exit; on = index($0, "## [" v "]") == 1; next }
	!on { next }
	/^\[[^]]+\]: / { next }
	/^### / { endp(); endul(); print "<h3>" inl(substr($0, 5)) "</h3>"; next }
	/^[-*] / { endp(); if (!ul) { print "<ul>"; ul = 1 }; endli(); printf "<li>%s", inl(substr($0, 3)); li = 1; next }
	/^[ \t]+[^ \t]/ && li { sub(/^[ \t]+/, ""); printf " %s", inl($0); next }
	/^[ \t]*$/ { endp(); next }
	p { printf " %s", inl($0); next }
	{ endul(); printf "<p>%s", inl($0); p = 1 }
	END { endp(); endul() }
' "$ROOT/CHANGELOG.md")
NOTES=${NOTES//]]>/]]&gt;}

cat <<EOF
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>agenterm</title>
    <link>https://github.com/$REPO</link>
    <description>agenterm releases</description>
    <language>en</language>
    <item>
      <title>agenterm $VERSION</title>
      <pubDate>$PUBDATE</pubDate>
      <sparkle:version>$VERSION</sparkle:version>
      <sparkle:shortVersionString>$VERSION</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>$MIN_MACOS</sparkle:minimumSystemVersion>
      <sparkle:fullReleaseNotesLink>$NOTES_URL</sparkle:fullReleaseNotesLink>
EOF
if [[ -n $NOTES ]]; then
	printf '      <description><![CDATA[\n%s\n<p><a href="%s">Release on GitHub</a></p>\n      ]]></description>\n' "$NOTES" "$NOTES_URL"
else
	printf '      <sparkle:releaseNotesLink>%s</sparkle:releaseNotesLink>\n' "$NOTES_URL"
fi
cat <<EOF
      <enclosure url="$URL" length="$LENGTH" type="application/octet-stream" sparkle:edSignature="$SIG"/>
    </item>
  </channel>
</rss>
EOF
