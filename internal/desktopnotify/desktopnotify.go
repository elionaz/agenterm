// Package desktopnotify holds what the daemon and the desktop app share
// about attention notifications: their wording and the osascript fallback.
package desktopnotify

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/elionaz/agenterm/internal/i18n"
	"github.com/elionaz/agenterm/internal/protocol"
)

// Title is "<project> · <profile>" (either part may be missing).
func Title(projectName, profileID string) string {
	switch {
	case projectName != "" && profileID != "":
		return projectName + " · " + profileID
	case projectName != "":
		return projectName
	case profileID != "":
		return profileID
	}
	return "agenterm"
}

// Body is the attention message (kept verbatim: it comes from the agent),
// or a fallback per kind in lang.
func Body(lang i18n.Lang, kind, message string) string {
	if message != "" {
		return message
	}
	switch kind {
	case "permission":
		return i18n.T(lang, i18n.NotifyPermission)
	case "question":
		return i18n.T(lang, i18n.NotifyQuestion)
	case "done":
		return i18n.T(lang, i18n.NotifyDone)
	case "error":
		return i18n.T(lang, i18n.NotifyError)
	case "stall":
		return i18n.T(lang, i18n.NotifyStall)
	}
	return i18n.T(lang, i18n.NotifyAttention)
}

// EnvNotify is the kill switch for desktop notifications: AGENTERM_NOTIFY=off
// disables every notification the app and the daemon would post (native and
// osascript), whatever settings.yaml says. Meant for test and demo
// instances, next to AGENTERM_HOME.
const EnvNotify = "AGENTERM_NOTIFY"

// Disabled reports whether AGENTERM_NOTIFY turns desktop notifications off
// ("off", "0", "false" or "no", any case).
func Disabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvNotify))) {
	case "off", "0", "false", "no":
		return true
	}
	return false
}

// Osascript posts a notification through AppleScript. macOS attributes it
// to Script Editor, and clicking it opens Script Editor rather than
// agenterm, so it is only an opt-in fallback (notify.fallback: osascript).
// Best effort and bounded in time; a no-op outside macOS and when Disabled.
func Osascript(title, body string, sound bool) error {
	if runtime.GOOS != "darwin" || Disabled() {
		return nil
	}
	script := "display notification " + appleScriptString(body) + " with title " + appleScriptString(title)
	if sound {
		script += ` sound name "Glass"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "osascript", "-e", script).Run()
}

func appleScriptString(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 {
			return ' '
		}
		return r
	}, s)
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// StallBody describes a stalled turn: "Stalled? No activity for 5m ·
// <activity>". The quiet period counts from the last output or event.
func StallBody(lang i18n.Lang, activity string, lastOutput, lastEvent, stalledAt *time.Time) string {
	msg := i18n.T(lang, i18n.NotifyStall)
	if stalledAt != nil {
		var last time.Time
		for _, t := range []*time.Time{lastOutput, lastEvent} {
			if t != nil && t.After(last) {
				last = *t
			}
		}
		if !last.IsZero() {
			msg += " " + i18n.T(lang, i18n.NotifyStallAgo, ShortDuration(stalledAt.Sub(last)))
		}
	}
	if activity != "" {
		msg += " · " + activity
	}
	return msg
}

// StallEventBody is StallBody for a session.activity event, with its
// activity rendered in lang (hook activity) or verbatim (output).
func StallEventBody(lang i18n.Lang, ev protocol.Event) string {
	return StallBody(lang, ActivityText(lang, ev.Live), ev.LastOutputAt, ev.LastEventAt, ev.StalledAt)
}

// ActivityText is a session's activity line in lang.
func ActivityText(lang i18n.Lang, l protocol.Live) string {
	if l.ActivityKind != "" {
		return i18n.ActivityText(lang, l.ActivityKind, l.ActivityTarget)
	}
	return l.Activity
}

// ShortDuration renders 45s, 5m, 1h 20m.
func ShortDuration(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	switch {
	case s < 60:
		return strconv.Itoa(max(s, 0)) + "s"
	case s < 3600:
		return strconv.Itoa(s/60) + "m"
	}
	h, m := s/3600, (s%3600)/60
	if m == 0 {
		return strconv.Itoa(h) + "h"
	}
	return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
}
