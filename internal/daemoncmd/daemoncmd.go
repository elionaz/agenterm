// Package daemoncmd is the `daemon` subcommand shared by every binary that can
// host the panal daemon (the CLI and the desktop app), so the .app does not
// depend on a separately installed CLI.
package daemoncmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/macnotify"
	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/peercred"
	"github.com/elionaz/panal/internal/sigsender"
	"github.com/elionaz/panal/internal/transport"
)

// Name is the subcommand that selects daemon mode (os.Args[1]).
const Name = "daemon"

// Run executes `daemon [--detach] [--idle D]`. In the foreground it blocks
// until SIGTERM/SIGINT or a shutdown request; with --detach it starts the
// current executable in daemon mode and returns once the socket is up.
//
// The CLI bundled in panal.app (Contents/Helpers/panal) runs the
// daemon as the app's executable instead (client.DaemonExecutable): with
// --detach it starts that one, in the foreground it execs it in place
// (same pid, args and environment), so the daemon belongs to the bundle and
// gets native notifications and their clicks.
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
		if err := client.StartDaemon(context.Background(), client.DaemonExecutable(exe), p); err != nil {
			return err
		}
		fmt.Println("daemon started on", p.Socket())
		return nil
	}
	if exe, err := os.Executable(); err == nil {
		if app := client.DaemonExecutable(exe); app != exe {
			// Only returns on failure: then run here, without native
			// notifications.
			err := syscall.Exec(app, append([]string{app, Name}, args...), os.Environ())
			fmt.Fprintf(os.Stderr, "panal daemon: exec %s: %v; running the CLI's daemon\n", app, err)
		}
	}

	// Record which signal stopped us — and who sent it — so the log explains
	// every shutdown.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	watchTermination(logger, cancel)
	// Move ~/.agenterm and ~/.config/agenterm (from before the rename) to
	// their panal names, unless an old daemon still uses them.
	moved, err := paths.MigrateLegacy(func(sock string) bool { return transport.Alive(transport.Default(), sock) })
	for _, m := range moved {
		logger.Info("moved pre-rename directory", "move", m)
	}
	if err != nil {
		logger.Warn("move pre-rename directories", "err", err)
	}
	if len(moved) > 0 {
		if p, err = paths.Default(); err != nil {
			return err
		}
	}
	exe, _ := os.Executable()
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	n := newNotifier(logger, exe)
	srv, err := daemon.New(daemon.Config{
		Paths:     p,
		IdleAfter: *idle,
		KillGrace: 2 * time.Second,
		Logger:    logger,
		Exe:       exe,
		Notifier:  n.Notify,
	})
	if err != nil {
		return err
	}
	if !n.clicks {
		return srv.Run(ctx)
	}
	// Notification clicks are delivered on the main dispatch queue: the main
	// thread serves the Cocoa run loop while the daemon runs elsewhere.
	macnotify.RunMainLoop(func() { err = srv.Run(ctx) })
	return err
}

// watchTermination cancels ctx on SIGTERM/SIGINT. Where supported it also
// logs the sending process (pid, command line, parent), so an unexpected
// shutdown can be traced back to whoever sent the signal.
func watchTermination(logger *slog.Logger, cancel context.CancelCauseFunc) {
	if evs, ok, err := sigsender.Notify(syscall.SIGTERM, syscall.SIGINT); ok {
		go func() {
			ev := <-evs
			attrs := []any{"signal", ev.Signal.String(), "sender_pid", ev.PID, "sender_uid", ev.UID}
			reason := fmt.Sprintf("received signal %s from pid %d", ev.Signal, ev.PID)
			if ev.PID == 0 {
				attrs = append(attrs, "sender_cmd", "(sender not reported by the kernel)")
				reason = fmt.Sprintf("received signal %s (sender unknown)", ev.Signal)
			} else if p, err := peercred.Lookup(ev.PID); err == nil {
				attrs = append(attrs, "sender_cmd", clip(p.Command), "sender_ppid", p.PPID)
				reason += " (" + clip(p.Command) + ")"
				if pp, err := peercred.Lookup(p.PPID); err == nil {
					attrs = append(attrs, "sender_parent_cmd", clip(pp.Command))
				}
			} else {
				attrs = append(attrs, "sender_cmd", "(exited before lookup)")
			}
			logger.Warn("termination signal received", attrs...)
			cancel(errors.New(reason))
		}()
		return
	} else if err != nil {
		logger.Warn("signal sender tracking unavailable", "err", err)
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		if sig, ok := <-sigs; ok {
			cancel(fmt.Errorf("received signal %s", sig))
		}
	}()
}

func clip(s string) string {
	const max = 300
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}
