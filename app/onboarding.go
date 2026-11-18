package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/macnotify"
	"github.com/elionaz/agenterm/internal/settings"
)

// The welcome screen (shown while there are no projects) reports what a new
// install needs: notification permission and the CLI. Agents come from the
// profile list; their hooks need no setup (they are injected per session).

// OnboardingInfo is what the welcome screen shows.
type OnboardingInfo struct {
	// Notifications: authorized | denied | not-determined | unknown, or
	// off (AGENTERM_NOTIFY=off) and unsupported (no .app bundle).
	Notifications string `json:"notifications"`
	// CLIPath is the `agenterm` found on the login shell's PATH ("" if none).
	CLIPath string `json:"cliPath"`
	// BundledCLI is the CLI inside this .app ("" outside a bundle).
	BundledCLI string `json:"bundledCli"`
}

// Onboarding reports notification permission and whether the CLI is
// installed.
func (a *App) Onboarding() OnboardingInfo {
	info := OnboardingInfo{Notifications: notificationState()}
	if p, err := exec.LookPath("agenterm"); err == nil {
		info.CLIPath = p
	}
	info.BundledCLI = bundledCLI()
	return info
}

// RequestNotifications asks macOS for permission (the system prompt appears
// only while the user has not decided) and returns the new state.
func (a *App) RequestNotifications() string {
	if s := notificationState(); s == "off" || s == "unsupported" || s == "denied" {
		return s
	}
	if a.wailsCtx != nil {
		_, _ = runtime.RequestNotificationAuthorization(a.wailsCtx)
	}
	return a.NotificationStatus().State
}

// NotificationInfo is Settings → Notifications' permission line.
type NotificationInfo struct {
	// State: as OnboardingInfo.Notifications.
	State string `json:"state"`
	// Detail: why the app can't show notifications right now ("" when it
	// can), in English (for the curious; the UI explains State).
	Detail string `json:"detail,omitempty"`
}

// NotificationStatus reports macOS's permission for agenterm, re-reading it
// (a change in System Settings applies without a relaunch).
func (a *App) NotificationStatus() NotificationInfo {
	if s := notificationState0(); s != "" {
		return NotificationInfo{State: s}
	}
	st, detail := a.desk.Refresh()
	return NotificationInfo{State: st.String(), Detail: detail}
}

// TestNotification shows a notification now (title and body come from the
// UI, in its language), whatever window is in front. It reports why
// nothing could be shown.
// It looks like an approval request (the kind that plays a sound when
// notify.sound is on).
func (a *App) TestNotification(title, body string) error {
	st, _ := settings.Load(a.paths.Settings())
	if err := a.desk.Post(note{SessionID: "test", Kind: "question", Title: title, Body: body,
		Sound: st.Notify.WantsSound("question"), Osascript: st.Notify.UsesOsascript()}); err != nil {
		return fmt.Errorf("notifications can't be shown: %w", err)
	}
	return nil
}

// OpenNotificationSettings opens System Settings → Notifications on
// agenterm's entry (for a permission the user denied).
func (a *App) OpenNotificationSettings() error {
	url := "x-apple.systempreferences:com.apple.Notifications-Settings.extension"
	if id := bundleIdentifier(); id != "" {
		url += "?id=" + id
	}
	return exec.Command("open", url).Run()
}

func notificationState() string {
	if s := notificationState0(); s != "" {
		return s
	}
	return macnotify.AuthStatus(2 * time.Second).String()
}

// notificationState0 is the state that needs no query to macOS ("" when it
// does: the permission).
func notificationState0() string {
	if desktopnotify.Disabled() {
		return "off"
	}
	if !macnotify.HasBundle() {
		return "unsupported"
	}
	return ""
}

// bundledCLI is Contents/Helpers/agenterm next to this executable.
func bundledCLI() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	bundle := macnotify.BundlePath(exe)
	if bundle == "" {
		return ""
	}
	p := filepath.Join(bundle, "Contents", "Helpers", "agenterm")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
