// Package shellenv reads environment values from the user's login shell.
//
// Apps launched from Finder or the Dock get launchd's minimal environment
// (PATH without Homebrew, npm, ~/.local/bin…), so agent CLIs are not found.
// Both the desktop app (before it spawns the daemon) and the daemon (to
// detect installed agents) ask the login shell instead.
package shellenv

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const pathMarker = "__PANAL_PATH__"

// LoginPath asks the user's login shell for its PATH. It returns "" on any
// failure (or after a 3 s timeout).
func LoginPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// -i so rc files that only run interactively (common for PATH setup)
	// are sourced; markers isolate the value from any rc-file chatter.
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", `printf '%s%s%s' "`+pathMarker+`" "$PATH" "`+pathMarker+`"`)
	cmd.Stdin = nil
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	return extractMarked(out)
}

func extractMarked(out []byte) string {
	m := []byte(pathMarker)
	i := bytes.Index(out, m)
	if i < 0 {
		return ""
	}
	rest := out[i+len(m):]
	j := bytes.Index(rest, m)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(string(rest[:j]))
}

// JoinPaths concatenates PATH lists, dropping empty and duplicate entries
// (the first occurrence wins).
func JoinPaths(lists ...string) string {
	seen := make(map[string]bool)
	var out []string
	for _, l := range lists {
		for _, d := range filepath.SplitList(l) {
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			out = append(out, d)
		}
	}
	return strings.Join(out, string(filepath.ListSeparator))
}
