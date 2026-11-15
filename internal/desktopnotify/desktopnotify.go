// Package desktopnotify holds what the daemon and the desktop app share
// about attention notifications: their wording and the osascript fallback.
package desktopnotify

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
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

// Body is the attention message, or a Spanish fallback per kind.
func Body(kind, message string) string {
	if message != "" {
		return message
	}
	switch kind {
	case "permission":
		return "Necesita tu aprobación"
	case "question":
		return "Está esperando tu respuesta"
	case "done":
		return "Terminó"
	case "error":
		return "Terminó con un error"
	case "stall":
		return "¿Trabado? Sin actividad"
	}
	return "Necesita tu atención"
}

// Osascript posts a notification through AppleScript. macOS attributes it
// to Script Editor, and clicking it does not open agenterm, so it is only a
// fallback. Best effort and bounded in time; a no-op outside macOS.
func Osascript(title, body string, sound bool) error {
	if runtime.GOOS != "darwin" {
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

// StallBody describes a stalled turn: "¿Trabado? Sin actividad hace 5m ·
// <activity>". The quiet period counts from the last output or event.
func StallBody(activity string, lastOutput, lastEvent, stalledAt *time.Time) string {
	msg := "¿Trabado? Sin actividad"
	if stalledAt != nil {
		var last time.Time
		for _, t := range []*time.Time{lastOutput, lastEvent} {
			if t != nil && t.After(last) {
				last = *t
			}
		}
		if !last.IsZero() {
			msg += " hace " + ShortDuration(stalledAt.Sub(last))
		}
	}
	if activity != "" {
		msg += " · " + activity
	}
	return msg
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
