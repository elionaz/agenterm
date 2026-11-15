// Package daemoncmd is the `daemon` subcommand shared by every binary that can
// host the agenterm daemon (the CLI and the desktop app), so the .app does not
// depend on a separately installed CLI.
package daemoncmd

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/transport"
)

// Name is the subcommand that selects daemon mode (os.Args[1]).
const Name = "daemon"

// Run executes `daemon [--detach] [--idle D]`. In the foreground it blocks
// until SIGTERM/SIGINT or a shutdown request; with --detach it starts the
// current executable in daemon mode and returns once the socket is up.
func Run(args []string) error {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	detach := fs.Bool("detach", false, "start in the background and return once it is ready")
	idle := fs.Duration("idle", 0, "heuristic idle timeout (default 3s)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := paths.Default()
	if err != nil {
		return err
	}
	if *detach {
		if transport.Alive(transport.Default(), p.Socket()) {
			fmt.Println("daemon already running on", p.Socket())
			return nil
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if err := client.StartDaemon(context.Background(), exe, p); err != nil {
			return err
		}
		fmt.Println("daemon started on", p.Socket())
		return nil
	}

	// Record which signal stopped us so the log explains every shutdown.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigs)
	go func() {
		if sig, ok := <-sigs; ok {
			cancel(fmt.Errorf("received signal %s", sig))
		}
	}()
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)
	srv, err := daemon.New(daemon.Config{
		Paths:     p,
		IdleAfter: *idle,
		KillGrace: 2 * time.Second,
		Logger:    slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	if err != nil {
		return err
	}
	return srv.Run(ctx)
}
