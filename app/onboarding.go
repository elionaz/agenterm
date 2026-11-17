package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/macnotify"
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
	return notificationState()
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
	if desktopnotify.Disabled() {
		return "off"
	}
	if !macnotify.HasBundle() {
		return "unsupported"
	}
	return macnotify.AuthStatus(2 * time.Second).String()
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
