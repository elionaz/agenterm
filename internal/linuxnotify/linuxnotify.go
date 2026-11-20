// Package linuxnotify posts desktop notifications on Linux through the
// freedesktop notification service (org.freedesktop.Notifications, over
// the D-Bus session bus): GNOME, KDE, XFCE and most other desktops run
// one. Each notification has a key (agenterm: a session id); a newer one
// with the same key replaces it, and clicking it calls the click handler
// with that key. Pure Go (godbus), so the cgo-free CLI/daemon can use it.
package linuxnotify

import "errors"

// ErrUnavailable: no notification service (not Linux, no session bus).
var ErrUnavailable = errors.New("no desktop notification service")

// Note is one notification.
type Note struct {
	Key   string // replaces the previous notification with the same key
	Title string
	Body  string
	Sound bool
}
