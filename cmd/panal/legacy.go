package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// legacyName is the CLI's name before the rename to panal. Installs keep an
// `agenterm` link to this binary for a while so scripts and agents that
// still call it work.
const legacyName = "agenterm"

// warnLegacyName prints a one-line deprecation notice when the binary runs
// as `agenterm`. Commands that hooks and agents call on their own (notify,
// msg) and the daemon stay quiet: nobody reads their stderr, or it would
// end up in an agent's context.
func warnLegacyName(argv0 string, args []string) {
	if filepath.Base(argv0) != legacyName {
		return
	}
	if len(args) > 0 {
		switch args[0] {
		case "notify", "msg", "daemon", "wait":
			return
		}
	}
	fmt.Fprintln(os.Stderr, "panal: `agenterm` is now `panal`; the old name will go away in a future release")
}
