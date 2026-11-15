package main

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

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
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
