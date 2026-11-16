package daemoncmd

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/macnotify"
	"github.com/elionaz/agenterm/internal/paths"
)

// notifier is the daemon's fallback notifier (used while no UI client is
// subscribed). Inside the .app bundle it posts native notifications, which
// macOS attributes to agenterm (name and icon) and whose clicks open the app
// on that session. When that is not possible (no bundle, not authorized) it
// drops them, or uses osascript if notify.fallback says so.
// AGENTERM_NOTIFY=off turns it off entirely.
type notifier struct {
	log       *slog.Logger
	off       bool   // AGENTERM_NOTIFY=off
	bundle    string // the .app containing the daemon executable; "" if none
	native    bool   // bundle with a bundle identifier
	clicks    bool   // our delegate is installed (clicks reach onClick)
	askedOnce sync.Once

	mu         sync.Mutex
	lastReason string // last reason logged for a dropped notification
}

func newNotifier(log *slog.Logger, exe string) *notifier {
	n := &notifier{log: log, bundle: macnotify.BundlePath(exe), off: desktopnotify.Disabled()}
	if n.off {
		log.Info("notifications disabled", "env", desktopnotify.EnvNotify+"="+os.Getenv(desktopnotify.EnvNotify))
		return n
	}
	n.native = n.bundle != "" && macnotify.HasBundle()
	if n.native {
		n.clicks = macnotify.SetClickHandler(n.onClick)
	}
	log.Info("notifications", "native", n.native, "bundle", n.bundle, "click_handler", n.clicks)
	return n
}

// Notify never blocks the daemon for long (native posting and osascript are
// both time-bounded) and never panics it.
func (n *notifier) Notify(note daemon.Notification) {
	defer func() {
		if r := recover(); r != nil {
			n.log.Error("notification panicked", "panic", r)
		}
	}()
	if n.off {
		return
	}
	reason := "no app bundle (daemon not running as agenterm.app)"
	if n.native {
		st := macnotify.AuthStatus(2 * time.Second)
		if st == macnotify.StatusNotDetermined {
			// First time ever: ask (the prompt may wait on the user); until
			// then the notification goes to the fallback.
			n.askedOnce.Do(func() {
				go func() {
					ok, err := macnotify.RequestAuth(5 * time.Minute)
					n.log.Info("notification authorization", "granted", ok, "err", err)
				}()
			})
		}
		if st == macnotify.StatusAuthorized {
			err := macnotify.Post(macnotify.Note{
				ID:        macnotify.NotificationID(note.SessionID),
				Title:     note.Title,
				Body:      note.Body,
				SessionID: note.SessionID,
				Sound:     note.Sound,
			})
			if err == nil {
				n.log.Info("notification posted", "notifier", "native", "session", note.SessionID, "kind", note.Kind)
				return
			}
			reason = "native notification failed: " + err.Error()
		} else {
			reason = "native notifications not authorized (" + st.String() + ")"
		}
	}
	if !note.Osascript {
		n.dropped(reason)
		return
	}
	err := desktopnotify.Osascript(note.Title, note.Body, note.Sound)
	n.log.Info("notification posted", "notifier", "osascript", "reason", reason, "session", note.SessionID, "kind", note.Kind, "err", err)
}

// dropped logs, once per distinct reason, that notifications are not shown.
func (n *notifier) dropped(reason string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if reason == n.lastReason {
		return
	}
	n.lastReason = reason
	n.log.Warn("notifications not shown; set notify.fallback: osascript in settings.yaml to use AppleScript (shows as Script Editor)",
		"reason", reason)
}

// onClick opens the app on the clicked session. The daemon gets the click
// because it posted the notification; agenterm://session/<id> reaches the
// app whether it is running (URL event) or not (launched with it). The
// daemon's AGENTERM_HOME, if any, goes along for a cold launch so the app
// talks to this daemon.
func (n *notifier) onClick(sessionID string) {
	n.log.Info("notification clicked", "session", sessionID)
	args := []string{"-a", n.bundle}
	if h := os.Getenv(paths.EnvHome); h != "" {
		args = append(args, "--env", paths.EnvHome+"="+h)
	}
	args = append(args, macnotify.SessionURL(sessionID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "open", args...).CombinedOutput(); err != nil {
		n.log.Warn("open app for notification", "err", err, "out", string(out))
	}
}
