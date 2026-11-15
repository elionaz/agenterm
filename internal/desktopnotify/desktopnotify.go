// Package desktopnotify holds what the daemon and the desktop app share
// about attention notifications: their wording and the osascript fallback.
package desktopnotify

import (
	"context"
	"os/exec"
	"runtime"
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
