// Package macnotify posts native macOS notifications (UNUserNotificationCenter)
// for both the desktop app and the daemon the app starts, so they show as
// panal with its icon. It needs the process to belong to an .app bundle
// (a bundle identifier); elsewhere every call is a no-op or ErrUnavailable
// and callers fall back to osascript.
//
// The daemon, which has no Cocoa event loop, can also receive clicks: see
// SetClickHandler and RunMainLoop.
package macnotify

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// ErrUnavailable means native notifications cannot be used here (not macOS,
// no cgo, or no bundle identifier).
var ErrUnavailable = errors.New("native notifications unavailable")

// Note is one notification. ID replaces an earlier notification with the
// same ID (one per session); SessionID goes in userInfo["sessionId"].
type Note struct {
	ID, Title, Body, SessionID string
	Sound                      bool
}

// Status is the notification authorization state.
type Status int

const (
	StatusUnknown       Status = iota // query failed or timed out
	StatusNotDetermined               // never asked
	StatusDenied
	StatusAuthorized // authorized, provisional or ephemeral
)

func (s Status) String() string {
	switch s {
	case StatusNotDetermined:
		return "not-determined"
	case StatusDenied:
		return "denied"
	case StatusAuthorized:
		return "authorized"
	}
	return "unknown"
}

// NotificationID is the notification identifier used for a session, shared
// by app and daemon so either can replace or withdraw the other's.
func NotificationID(sessionID string) string { return "panal-" + sessionID }

// BundlePath returns the .app bundle that contains exe
// ("/A/panal.app/Contents/MacOS/panal" -> "/A/panal.app"), or ""
// when exe is not inside one.
func BundlePath(exe string) string {
	const marker = ".app/Contents/MacOS/"
	i := strings.LastIndex(exe, marker)
	if i < 0 || strings.Contains(exe[i+len(marker):], "/") {
		return ""
	}
	return exe[:i+len(".app")]
}

// URLScheme is the app's URL scheme (CFBundleURLTypes in Info.plist).
const URLScheme = "panal"

var sessionIDRe = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// SessionURL is panal://session/<id>: opening it selects that session in
// the app (launching it if needed).
func SessionURL(sessionID string) string {
	return URLScheme + "://session/" + url.PathEscape(sessionID)
}

// ParseSessionURL extracts the session id from panal://session/<id>.
func ParseSessionURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != URLScheme || u.Host != "session" {
		return "", false
	}
	id := strings.Trim(u.Path, "/")
	if !sessionIDRe.MatchString(id) {
		return "", false
	}
	return id, true
}
