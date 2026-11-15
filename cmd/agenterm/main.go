// Command agenterm is the daemon and CLI client for agenterm.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemoncmd"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
)

const usage = `agenterm - workspace daemon for coding agents

Usage:
  agenterm daemon [--detach]          run the daemon (foreground unless --detach)
  agenterm stop                       stop the daemon (kills all sessions)
  agenterm project add <dir> [--name N]
  agenterm project list
  agenterm project rm <id>
  agenterm profiles                   list agent profiles
  agenterm spawn [--cols C --rows R] <projectId|name> <profileId>
  agenterm ls                         list sessions
  agenterm attach <sessionId>         attach (detach with Ctrl-\)
  agenterm kill <sessionId>           kill a session (again to forget an exited one)
  agenterm notify [--session ID] --state running|waiting_input|idle

Environment:
  AGENTERM_HOME     single dir overriding ~/.agenterm and ~/.config/agenterm
  AGENTERM_SESSION  default session for notify (set inside sessions)
  AGENTERM_SOCK     daemon socket (set inside sessions)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agenterm:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return errors.New("missing command")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case daemoncmd.Name:
		return daemoncmd.Run(rest)
	case "notify":
		return cmdNotify(rest)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}

	c, err := newClient(true)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	switch cmd {
	case "stop":
		return cmdStop(ctx)
	case "project":
		return cmdProject(ctx, c, rest)
	case "profiles":
		return cmdProfiles(ctx, c)
	case "spawn":
		return cmdSpawn(ctx, c, rest)
	case "ls":
		return cmdLs(ctx, c)
	case "attach":
		if len(rest) != 1 {
			return errors.New("usage: agenterm attach <sessionId>")
		}
		return attach(ctx, c, rest[0])
	case "kill":
		if len(rest) != 1 {
			return errors.New("usage: agenterm kill <sessionId>")
		}
		info, err := c.Kill(ctx, rest[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s %s%s\n", info.ID, info.State, exitSuffix(info))
		return nil
	}
	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

// newClient builds a client; with autoStart the daemon is launched on demand.
func newClient(autoStart bool) (*client.Client, error) {
	sock, err := client.SocketFromEnv()
	if err != nil {
		return nil, err
	}
	c := client.New(sock)
	if autoStart && os.Getenv(protocol.EnvSock) == "" {
		c.AutoStart = func(ctx context.Context) error {
			p, err := paths.Default()
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			return client.StartDaemon(ctx, exe, p)
		}
	}
	return c, nil
}

func cmdStop(ctx context.Context) error {
	c, err := newClient(false)
	if err != nil {
		return err
	}
	if err := c.Shutdown(ctx); err != nil {
		return err
	}
	// Wait for the socket to go away so callers can rely on a clean state.
	deadline := time.Now().Add(10 * time.Second)
	for c.Ping(ctx) == nil {
		if time.Now().After(deadline) {
			return errors.New("daemon still running after shutdown request")
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("daemon stopped")
	return nil
}

func cmdProject(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: agenterm project add|list|rm")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("project add", flag.ContinueOnError)
		name := fs.String("name", "", "display name (defaults to folder name)")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: agenterm project add <dir> [--name N]")
		}
		// Resolve relative paths here: the daemon's cwd is not the caller's.
		dir, err := filepath.Abs(fs.Arg(0))
		if err != nil {
			return err
		}
		p, err := c.ProjectAdd(ctx, dir, *name)
		if err != nil {
			return err
		}
		fmt.Printf("%s\t%s\t%s\n", p.ID, p.Name, p.Path)
		return nil
	case "list", "ls":
		ps, err := c.ProjectList(ctx)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tPATH\tADDED")
		for _, p := range ps {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Path, p.AddedAt.Local().Format(time.DateTime))
		}
		return w.Flush()
	case "rm", "remove":
		if len(args) != 2 {
			return errors.New("usage: agenterm project rm <id>")
		}
		return c.ProjectRemove(ctx, args[1])
	}
	return fmt.Errorf("unknown project subcommand %q", args[0])
}

func cmdProfiles(ctx context.Context, c *client.Client) error {
	ps, err := c.Profiles(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCMD\tSTATUS")
	for _, p := range ps {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Cmd, p.Status)
	}
	return w.Flush()
}

func cmdSpawn(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	cols := fs.Uint("cols", 0, "initial columns (default: current terminal or 80)")
	rows := fs.Uint("rows", 0, "initial rows (default: current terminal or 24)")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: agenterm spawn <projectId> <profileId>")
	}
	cc, rr := uint16(*cols), uint16(*rows)
	if cc == 0 || rr == 0 {
		if w, h, ok := termSize(); ok {
			cc, rr = w, h
		}
	}
	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: fs.Arg(0), ProfileID: fs.Arg(1), Cols: cc, Rows: rr})
	if err != nil {
		return err
	}
	fmt.Println(info.ID)
	return nil
}

func cmdLs(ctx context.Context, c *client.Client) error {
	ss, err := c.Sessions(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPROJECT\tPROFILE\tSTATE\tPID\tSIZE\tAGE\tCWD")
	for _, s := range ss {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s%s\t%d\t%dx%d\t%s\t%s\n", s.ID, s.ProjectID, s.ProfileID, s.State, exitSuffix(s),
			s.Pid, s.Cols, s.Rows, time.Since(s.CreatedAt).Round(time.Second), s.Cwd)
	}
	return w.Flush()
}

func exitSuffix(s protocol.SessionInfo) string {
	if s.ExitCode != nil {
		return fmt.Sprintf("(%d)", *s.ExitCode)
	}
	return ""
}

func cmdNotify(args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	id := fs.String("session", os.Getenv(protocol.EnvSession), "session id (default $AGENTERM_SESSION)")
	state := fs.String("state", "", "running | waiting_input | idle (aliases: waiting, done)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *state == "" {
		return errors.New("usage: agenterm notify [--session ID] --state STATE")
	}
	// Hooks must never spawn a daemon, and must not hang an agent.
	c, err := newClient(false)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	info, err := c.Notify(ctx, *id, *state)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", info.ID, info.State)
	return nil
}

// parseInterspersed lets flags appear after positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	return fs.Parse(append([]string{"--"}, pos...))
}
