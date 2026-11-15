package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/transport"
)

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
