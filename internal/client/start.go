package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/transport"
)

// DaemonExecutable is the binary that should run `daemon` for the agenterm
// executable exe. The CLI bundled in agenterm.app (Contents/Helpers/agenterm,
// which Homebrew symlinks into PATH) defers to the app's own executable
// (Contents/MacOS/agenterm) when it exists and is executable: only a process
// of the bundle gets native notifications and their clicks. Any other exe
// is returned unchanged.
func DaemonExecutable(exe string) string {
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	app := appExeForHelper(real)
	if app == "" {
		return exe
	}
	if fi, err := os.Stat(app); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return exe
	}
	return app
}

// appExeForHelper maps "/A/X.app/Contents/Helpers/agenterm" to
// "/A/X.app/Contents/MacOS/agenterm"; "" when path is not a bundle helper.
func appExeForHelper(path string) string {
	const marker = ".app/Contents/Helpers/"
	i := strings.LastIndex(path, marker)
	if i < 0 || strings.Contains(path[i+len(marker):], "/") || i+len(marker) == len(path) {
		return ""
	}
	return path[:i+len(".app")] + "/Contents/MacOS/agenterm"
}

// StartDaemon launches `<exe> daemon` detached from the caller (new session,
// stdio to the daemon log) and waits until its socket accepts connections.
// exe is typically os.Executable() of the agenterm binary.
func StartDaemon(ctx context.Context, exe string, p paths.Paths) error {
	if err := p.Ensure(); err != nil {
		return err
	}
	logf, err := os.OpenFile(p.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "daemon")
	cmd.Dir = "/"
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }() // reap if we outlive it

	t := transport.Default()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		if transport.Alive(t, p.Socket()) {
			return nil
		}
		select {
		case err := <-exited:
			if transport.Alive(t, p.Socket()) {
				return nil // another starter won the race
			}
			return fmt.Errorf("daemon exited during startup (%v); see %s", err, p.LogFile())
		case <-ctx.Done():
			return fmt.Errorf("daemon did not come up: %w; see %s", ctx.Err(), p.LogFile())
		case <-tick.C:
		}
	}
}
