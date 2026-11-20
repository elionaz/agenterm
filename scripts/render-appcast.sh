#!/usr/bin/env bash
# Prints the Sparkle appcast (a single item: the latest release) for
# scripts/release-local.sh, which uploads it as the release asset
# appcast.xml, i.e. SUFeedURL
# https://github.com/<repo>/releases/latest/download/appcast.xml.
#
#   scripts/render-appcast.sh <version> <zip> <edSignature> [min macOS]
#
# Release notes: the "## [<version>]" section of CHANGELOG.md as simple
# HTML (headings, bullet lists, `code`, **bold**, *italics*, [links](https://…)), plus a link to the GitHub
# release; without such a section, Sparkle shows the release page instead.
# GITHUB_REPOSITORY (owner/name) defaults to elionaz/panal.
set -euo pipefail

[[ $# -ge 3 && $# -le 4 ]] || { echo "usage: $0 <version> <zip> <edSignature> [min macOS]" >&2; exit 1; }
VERSION=${1#v}
ZIP=$2
SIG=$3
MIN_MACOS=${4:-13.0}
[[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "invalid version: $1" >&2; exit 1; }
[[ -f $ZIP ]] || { echo "no such file: $ZIP" >&2; exit 1; }
[[ $SIG =~ ^[A-Za-z0-9+/=]+$ ]] || { echo "invalid EdDSA signature" >&2; exit 1; }

REPO=${GITHUB_REPOSITORY:-elionaz/panal}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
LENGTH=$(stat -f%z "$ZIP")
URL="https://github.com/$REPO/releases/download/v$VERSION/$(basename "$ZIP")"
NOTES_URL="https://github.com/$REPO/releases/tag/v$VERSION"
PUBDATE=$(LC_ALL=C date -u "+%a, %d %b %Y %H:%M:%S +0000")

# CHANGELOG section -> HTML.
NOTES=$(awk -v v="$VERSION" '
	function esc(s) { gsub(/&/, "\\&amp;", s); gsub(/</, "\\&lt;", s); gsub(/>/, "\\&gt;", s); return s }
	function inl(s,   n, i, url, txt, out) {
		s = esc(s)
		# Code spans first, kept aside so nothing inside them is formatted.
		n = 0
		while (match(s, /`[^`]+`/)) { code[++n] = "<code>" substr(s, RSTART + 1, RLENGTH - 2) "</code>"; s = substr(s, 1, RSTART - 1) "\001" n "\002" substr(s, RSTART + RLENGTH) }
		# [text](http…) links; other targets keep just the text.
		while (match(s, /\[[^]]+\]\([^)]+\)/)) {
			txt = substr(s, RSTART, RLENGTH); i = index(txt, "](")
			url = substr(txt, i + 2, length(txt) - i - 2); txt = substr(txt, 2, i - 2)
			gsub(/"/, "\\&quot;", url)
			out = (url ~ /^https?:\/\//) ? "<a href=\"" url "\">" txt "</a>" : txt
			s = substr(s, 1, RSTART - 1) out substr(s, RSTART + RLENGTH)
		}
		while (match(s, /\*\*[^*]+\*\*/)) { s = substr(s, 1, RSTART - 1) "<b>" substr(s, RSTART + 2, RLENGTH - 4) "</b>" substr(s, RSTART + RLENGTH) }
		while (match(s, /\*[^* ]([^*]*[^* ])?\*/)) { s = substr(s, 1, RSTART - 1) "<i>" substr(s, RSTART + 1, RLENGTH - 2) "</i>" substr(s, RSTART + RLENGTH) }
		for (i = 1; i <= n; i++) { while (match(s, "\001" i "\002")) s = substr(s, 1, RSTART - 1) code[i] substr(s, RSTART + RLENGTH) }
		return s
	}
	# A list item or paragraph is formatted whole, so **bold** or a link may
	# wrap across lines.
	function endp() { if (p) { printf "<p>%s</p>\n", inl(buf); p = 0 } }
	function endli() { if (li) { printf "<li>%s</li>\n", inl(buf); li = 0 } }
	function endul() { endli(); if (ul) { print "</ul>"; ul = 0 } }
	/^## \[/ { if (on) exit; on = index($0, "## [" v "]") == 1; next }
	!on { next }
	/^\[[^]]+\]: / { next }
	/^### / { endp(); endul(); print "<h3>" inl(substr($0, 5)) "</h3>"; next }
	/^[-*] / { endp(); if (!ul) { print "<ul>"; ul = 1 }; endli(); buf = substr($0, 3); li = 1; next }
	/^[ \t]+[^ \t]/ && li { sub(/^[ \t]+/, ""); buf = buf " " $0; next }
	/^[ \t]*$/ { endp(); next }
	p { buf = buf " " $0; next }
	{ endul(); buf = $0; p = 1 }
	END { endp(); endul() }
' "$ROOT/CHANGELOG.md")
NOTES=${NOTES//]]>/]]&gt;}

cat <<EOF
<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>panal</title>
    <link>https://github.com/$REPO</link>
    <description>panal releases</description>
    <language>en</language>
    <item>
      <title>panal $VERSION</title>
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
