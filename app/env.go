package main

import (
	"os"

	"github.com/elionaz/panal/internal/shellenv"
)

// prepareDaemonEnv adjusts this process's environment before it spawns the
// daemon, which (and every session) inherits it. Apps launched from Finder or
// the Dock get launchd's minimal environment: PATH lacks Homebrew, npm and
// friends (so `claude`/`codex` are not found) and LANG is unset (so shells
// mangle UTF-8). We borrow PATH from the user's login shell and fill in sane
// terminal defaults.
func prepareDaemonEnv() {
	if p := shellenv.LoginPath(); p != "" {
		os.Setenv("PATH", p)
	}
	// Sessions are rendered by xterm.js, whatever terminal launched the app.
	os.Setenv("TERM", "xterm-256color")
	os.Setenv("COLORTERM", "truecolor")
	os.Setenv("TERM_PROGRAM", "panal")
	if os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" && os.Getenv("LC_CTYPE") == "" {
		os.Setenv("LANG", "en_US.UTF-8")
	}
}
