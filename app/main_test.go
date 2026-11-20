package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/elionaz/panal/internal/desktopnotify"
)

func TestMain(m *testing.M) {
	// Last line of defence: if something still runs this test binary as
	// `app.test daemon`, exit instead of rerunning the suite (and spawning
	// more copies of itself).
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		fmt.Fprintln(os.Stderr, "app.test: refusing to run as a daemon")
		os.Exit(2)
	}
	noSelfLaunch = true
	os.Setenv(desktopnotify.EnvNotify, "off") // tests never post notifications
	os.Exit(m.Run())
}
