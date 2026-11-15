package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// prepareDaemonEnv adjusts this process's environment before it spawns the
// daemon, which (and every session) inherits it. Apps launched from Finder or
// the Dock get launchd's minimal environment: PATH lacks Homebrew, npm and
// friends (so `claude`/`codex` are not found) and LANG is unset (so shells
// mangle UTF-8). We borrow PATH from the user's login shell and fill in sane
// terminal defaults.
func prepareDaemonEnv() {
	if p := loginShellPath(); p != "" {
		os.Setenv("PATH", p)
	}
	// Sessions are rendered by xterm.js, whatever terminal launched the app.
	os.Setenv("TERM", "xterm-256color")
	os.Setenv("COLORTERM", "truecolor")
	os.Setenv("TERM_PROGRAM", "agenterm")
	if os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" && os.Getenv("LC_CTYPE") == "" {
		os.Setenv("LANG", "en_US.UTF-8")
	}
}

const pathMarker = "__AGENTERM_PATH__"

// loginShellPath asks the user's login shell for its PATH. It returns "" on
// any failure, leaving the current PATH untouched.
func loginShellPath() string {
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
