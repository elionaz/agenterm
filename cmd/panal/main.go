// Command panal is the daemon and CLI client for panal.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/daemoncmd"
	"github.com/elionaz/panal/internal/notifycmd"
	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/project"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/version"
)

const usage = `panal - workspace daemon for coding agents

Usage:
  panal daemon [--detach]          run the daemon (foreground unless --detach)
  panal stop [--force]             stop the daemon; refuses while sessions are alive
                                      unless --force (which kills them)
  panal project add <dir> [--name N]
  panal project list
  panal project rm <id>
  panal project set <id|name> [--name N] [--color C|auto] [--icon E|none]
  panal profiles                   list agent profiles (catalog + profiles.yaml)
  panal profiles scan              re-detect installed agents and list them
  panal spawn [--cols C --rows R] [--worktree [--branch B] [--base X] [--trust-setup|--no-setup]
                | --in-worktree ID] <projectId|name> <profileId>
                                      --worktree: new git worktree + branch for the session
                                      (set up from the repository's .panal.yaml)
  panal diff [--stat] <sessionId|worktreeId>   changes (worktree: vs its base)
  panal worktrees <projectId|name> list panal worktrees (live / exited / orphan)
  panal worktree rm [--force] [--delete-branch] [--dry-run] <id>
  panal worktree merge <id>        merge its branch into the base (main folder must be
                                      clean and on the base branch)
  panal worktree pr [--no-open] <id>   push its branch and open the pull request page
                                      (GitHub / GitLab; prints the URL)
  panal worktree open [--finder] <id>
  panal ls                         list sessions (with activity line and timers)
  panal tail [-n N] <sessionId>    last N lines of output as plain text (default 8)
  panal attach <sessionId>         attach (detach with Ctrl-\)
  panal kill <sessionId>           kill a session (again to forget an exited one)
  panal rename <sessionId> <name>  name a session (--reset: back to the profile's name)
  panal resume <sessionId>|--all   respawn sessions suspended by a daemon restart
                                      (agents continue their conversation when they can)
  panal discard <sessionId>|--all  forget suspended sessions (and their scrollback)
  panal notify [--session ID] [--state S] [--kind K] [--message M] [--activity A]
                                      report state/attention (from agent hooks)
  panal notify --from-hook claude|codex   read a hook payload (stdin / last arg)
  panal ack <sessionId>            mark a session's attention as seen
  panal msg peers                  the other sessions of this project (inside a session)
  panal msg send <to> <text…>      message a session of this project (id, name or all)
  panal msg read                   your new messages from other sessions
  panal msg list [--pending]       message history
  panal wait <sessionId> [--timeout 30m]   until it is idle, waiting for input or exited
  panal instructions [<project>]   the standing instructions its sessions receive at start
                                      (inside a session: its project; none: the all-project ones)
  panal instructions list [<project>]   the instruction topics
  panal instructions show <topic>  one topic's text (for topics that load on demand)
  panal handoff [--worktree] [--keep] [--note FILE|-] <sessionId> <profile>
                                      continue a session's work with another agent (a summary as
                                      its first prompt); closes the session unless --keep
  panal handoff --into <targetId> <sessionId>   hand it to a live session instead
  panal handoff --print <sessionId>   the summary it would hand over
  panal browser [open|url|close|status|mcp] [--session ID]
                                      the session's own isolated Chrome (open: start it, print
                                      its DevTools URL; mcp: the browser MCP server bound to it)
  panal version                    print the version and commit (also --version)

Environment:
  PANAL_HOME     single dir overriding ~/.panal and ~/.config/panal
  PANAL_NOTIFY   off: no desktop notifications from the daemon (for test instances)
  PANAL_SESSION  default session for notify (set inside sessions)
  PANAL_SOCK     daemon socket (set inside sessions); ignored when PANAL_HOME is set
`

func main() {
	paths.ApplyLegacyEnv()
	warnLegacyName(os.Args[0], os.Args[1:])
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "panal:", err)
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
	case notifycmd.Name:
		return notifycmd.Run(rest, os.Stdin, os.Stdout, os.Stderr)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version", "--version":
		fmt.Println("panal", version.String())
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
		return cmdStop(ctx, rest)
	case "project":
		return cmdProject(ctx, c, rest)
	case "profiles":
		return cmdProfiles(ctx, c, rest)
	case "spawn":
		return cmdSpawn(ctx, c, rest)
	case "ls":
		return cmdLs(ctx, c)
	case "diff":
		return cmdDiff(ctx, c, rest)
	case "worktrees":
		if len(rest) != 1 {
			return errors.New("usage: panal worktrees <projectId|name>")
		}
		return cmdWorktrees(ctx, c, rest[0])
	case "worktree":
		return cmdWorktree(ctx, c, rest)
	case "tail":
		fs := flag.NewFlagSet("tail", flag.ContinueOnError)
		n := fs.Int("n", 8, "lines")
		if err := parseInterspersed(fs, rest); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: panal tail [-n N] <sessionId>")
		}
		lines, err := c.Tail(ctx, fs.Arg(0), *n)
		if err != nil {
			return err
		}
		for _, l := range lines {
			fmt.Println(l)
		}
		return nil
	case "attach":
		if len(rest) != 1 {
			return errors.New("usage: panal attach <sessionId>")
		}
		return attach(ctx, c, rest[0])
	case "ack":
		if len(rest) != 1 {
			return errors.New("usage: panal ack <sessionId>")
		}
		info, err := c.Ack(ctx, rest[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", info.ID, info.State)
		return nil
	case "rename":
		id, name, err := parseRename(rest)
		if err != nil {
			return err
		}
		info, err := c.Rename(ctx, id, name)
		if err != nil {
			return err
		}
		fmt.Printf("%s %s\n", info.ID, displayName(info))
		return nil
	case "resume", "discard":
		return cmdSuspended(ctx, c, cmd, rest)
	case "msg":
		return cmdMsg(ctx, c, rest)
	case "wait":
		return cmdWait(ctx, c, rest)
	case "instructions":
		return cmdInstructions(ctx, c, rest)
	case "browser":
		return cmdBrowser(ctx, c, rest)
	case "handoff":
		return cmdHandoff(ctx, c, rest)
	case "kill":
		if len(rest) != 1 {
			return errors.New("usage: panal kill <sessionId>")
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
	sock, fromSession, err := client.ResolveSocket()
	if err != nil {
		return nil, err
	}
	c := client.New(sock)
	// Inside a session (only $PANAL_SOCK) the daemon is the session's own:
	// never start another one.
	if autoStart && !fromSession {
		c.AutoStart = func(ctx context.Context) error {
			p, err := paths.Default()
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			// The CLI inside panal.app starts the daemon as the app, so
			// it gets native notifications and their clicks.
			return client.StartDaemon(ctx, client.DaemonExecutable(exe), p)
		}
	}
	return c, nil
}

func cmdStop(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	force := fs.Bool("force", false, "also kill live sessions")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: panal stop [--force]")
	}
	c, err := newClient(false)
	if err != nil {
		return err
	}
	// The daemon enforces the live-session check (and logs who asked).
	if err := c.Shutdown(ctx, *force); err != nil {
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
		return errors.New("usage: panal project add|list|rm|set")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("project add", flag.ContinueOnError)
		name := fs.String("name", "", "display name (defaults to folder name)")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: panal project add <dir> [--name N]")
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
			return errors.New("usage: panal project rm <id>")
		}
		return c.ProjectRemove(ctx, args[1])
	case "set":
		return cmdProjectSet(ctx, c, args[1:])
	}
	return fmt.Errorf("unknown project subcommand %q", args[0])
}

const projectSetUsage = "usage: panal project set <id|name> [--name N] [--color C|auto] [--icon E|none]"

// cmdProjectSet renames a project or changes its sidebar badge.
func cmdProjectSet(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("project set", flag.ContinueOnError)
	name := fs.String("name", "", "new display name")
	color := fs.String("color", "", "badge color: "+strings.Join(project.Colors, ", ")+", or auto")
	icon := fs.String("icon", "", "an emoji shown instead of the initials, or none")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(projectSetUsage)
	}
	params := protocol.ProjectUpdateParams{}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "name":
			params.Name = name
		case "color":
			v := autoNone(*color, "auto")
			params.Color = &v
		case "icon":
			v := autoNone(*icon, "none")
			params.Icon = &v
		}
	})
	if params.Name == nil && params.Color == nil && params.Icon == nil {
		return errors.New(projectSetUsage)
	}
	params.ID = fs.Arg(0) // an id or a name: the daemon resolves it
	p, err := c.UpdateProject(ctx, params)
	if err != nil {
		return err
	}
	fmt.Printf("%s\t%s\t%s\t%s\n", p.ID, p.Name, cmp.Or(p.Color, "auto"), cmp.Or(p.Icon, "-"))
	return nil
}

// autoNone maps the "back to automatic" keyword to "".
func autoNone(v, keyword string) string {
	if strings.EqualFold(strings.TrimSpace(v), keyword) {
		return ""
	}
	return v
}

func cmdProfiles(ctx context.Context, c *client.Client, args []string) error {
	var ps []profile.Profile
	var err error
	switch {
	case len(args) == 0 || args[0] == "list" || args[0] == "ls":
		ps, err = c.Profiles(ctx)
	case args[0] == "scan":
		ps, err = c.RescanProfiles(ctx)
	default:
		return fmt.Errorf("unknown profiles subcommand %q (want list or scan)", args[0])
	}
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tENABLED\tINSTALLED\tSOURCE\tSTATUS\tCMD")
	for _, p := range ps {
		installed := "-"
		switch {
		case p.Error != "":
			installed = "error: " + p.Error
		case p.Installed:
			installed = p.Path
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name, yesNo(p.Enabled), installed, p.Source, p.Status,
			strings.TrimSpace(p.Cmd+" "+strings.Join(p.Args, " ")))
	}
	return w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func cmdSpawn(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ContinueOnError)
	cols := fs.Uint("cols", 0, "initial columns (default: current terminal or 80)")
	rows := fs.Uint("rows", 0, "initial rows (default: current terminal or 24)")
	wt := fs.Bool("worktree", false, "run in a new git worktree (own branch)")
	branch := fs.String("branch", "", "branch for --worktree (default agent/<profile>-<id>)")
	base := fs.String("base", "", "base branch for --worktree (default: the project's current branch)")
	inWT := fs.String("in-worktree", "", "run in an existing panal worktree (id)")
	trustSetup := fs.Bool("trust-setup", false, "trust and run the repository's worktree setup (.panal.yaml)")
	noSetup := fs.Bool("no-setup", false, "create the worktree without running its setup")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: panal spawn [--worktree [--branch B] [--base X]] <projectId> <profileId>")
	}
	if !*wt && (*branch != "" || *base != "" || *trustSetup || *noSetup) {
		return errors.New("--branch, --base, --trust-setup and --no-setup need --worktree")
	}
	if *trustSetup && *noSetup {
		return errors.New("--trust-setup and --no-setup exclude each other")
	}
	mode := ""
	if *wt {
		mode = protocol.ModeWorktree
	}
	cc, rr := uint16(*cols), uint16(*rows)
	if cc == 0 || rr == 0 {
		if w, h, ok := termSize(); ok {
			cc, rr = w, h
		}
	}
	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: fs.Arg(0), ProfileID: fs.Arg(1), Cols: cc, Rows: rr,
		Mode: mode, Branch: *branch, Base: *base, WorktreeID: *inWT, TrustSetup: *trustSetup, SkipSetup: *noSetup})
	if err != nil {
		return err
	}
	if info.WorktreeID != "" {
		fmt.Printf("%s\t%s\t%s\n", info.ID, info.Branch, info.WorktreePath)
		if len(info.SuggestCopy) > 0 {
			fmt.Fprintf(os.Stderr, "note: git ignores %s, so the worktree doesn't have them; list them under worktree.copy in .panal.yaml to copy them into new worktrees\n",
				strings.Join(info.SuggestCopy, ", "))
		}
		return nil
	}
	fmt.Println(info.ID)
	// Only a note on stderr (stdout stays the id), for agents: best effort.
	if info.ProfileID == shellProfile {
		return nil
	}
	if ss, err := c.Sessions(ctx); err == nil {
		if note := sharedNote(folderSharers(ss, info.ProjectID, info.ID)); note != "" {
			fmt.Fprintln(os.Stderr, note)
		}
	}
	return nil
}

// cmdInstructions: the standing instructions set in the app (Settings →
// Instructions). With no subcommand, what a session of the project
// receives at start; list: the topics; show: one topic's text (what an
// agent runs for an "on demand" topic).
func cmdInstructions(ctx context.Context, c *client.Client, args []string) error {
	const usage = "usage: panal instructions [<project>] | list [<project>] | show <topic>"
	sub := ""
	if len(args) > 0 && (args[0] == "list" || args[0] == "show") {
		sub, args = args[0], args[1:]
	}
	if len(args) > 1 {
		return errors.New(usage)
	}
	if sub == "show" {
		if len(args) != 1 {
			return errors.New(usage)
		}
		t, err := c.ShowInstruction(ctx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("# %s\n\n%s\n", t.Title, t.Body)
		return nil
	}
	project := ""
	if len(args) == 1 {
		project = args[0]
	} else if id := os.Getenv("PANAL_SESSION"); id != "" {
		if ss, err := c.Sessions(ctx); err == nil {
			for _, s := range ss {
				if s.ID == id {
					project = s.ProjectID
				}
			}
		}
	}
	r, err := c.Instructions(ctx, project)
	if err != nil {
		return err
	}
	for _, e := range r.Errors {
		fmt.Fprintln(os.Stderr, "panal: skipped:", e)
	}
	if sub == "list" {
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TOPIC\tTITLE\tLOAD\tPROJECTS\tON\tBYTES")
		for _, t := range r.Topics {
			scope := "all"
			if len(t.Projects) > 0 {
				scope = strings.Join(t.Projects, ",")
			}
			on := "yes"
			if !t.Enabled {
				on = "no"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n", t.Slug, t.Title, t.Load, scope, on, len(t.Body))
		}
		fmt.Fprintf(w, "\n%s\n", r.Dir)
		return w.Flush()
	}
	if r.Composed == "" {
		fmt.Fprintln(os.Stderr, "no instructions (set them in panal: Settings → Instructions)")
		return nil
	}
	fmt.Println(r.Composed)
	return nil
}

func cmdLs(ctx context.Context, c *client.Client) error {
	ss, err := c.Sessions(ctx)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPROJECT\tPROFILE\tSTATE\tATTENTION\tPID\tSIZE\tAGE\tTURN\tGIT\tACTIVITY\tCWD")
	for _, s := range ss {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s%s\t%s\t%d\t%dx%d\t%s\t%s\t%s\t%s\t%s\n", s.ID, cmp.Or(s.Name, "-"), s.ProjectID, s.ProfileID, s.State, exitSuffix(s),
			attention(s), s.Pid, s.Cols, s.Rows, time.Since(s.CreatedAt).Round(time.Second), turn(s), gitCol(s.Branch, s.Git), activity(s), s.Cwd)
	}
	return w.Flush()
}

// parseRename reads `rename <sessionId> [name...]`: the remaining words
// are joined with spaces; no name (or --reset) clears it.
func parseRename(args []string) (id, name string, err error) {
	const use = "usage: panal rename <sessionId> <name>  (or --reset)"
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", "", errors.New(use)
	}
	id, words := args[0], args[1:]
	if len(words) == 1 && words[0] == "--reset" {
		return id, "", nil
	}
	if len(words) > 0 && words[0] == "--" {
		words = words[1:]
	}
	name = strings.TrimSpace(strings.Join(words, " "))
	if name == "" {
		return "", "", errors.New(use)
	}
	return id, name, nil
}

// displayName is the session's name, or its profile when it has none.
func displayName(s protocol.SessionInfo) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ProfileID
}

// cmdSuspended runs resume / discard on one suspended session or --all.
func cmdSuspended(ctx context.Context, c *client.Client, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	all := fs.Bool("all", false, "every suspended session")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if *all == (fs.NArg() == 1) || fs.NArg() > 1 {
		return fmt.Errorf("usage: panal %s <sessionId>|--all", cmd)
	}
	call := c.Resume
	if cmd == "discard" {
		call = c.Discard
	}
	res, err := call(ctx, fs.Arg(0), *all)
	if err != nil {
		return err
	}
	for _, s := range res.Sessions {
		if cmd == "discard" {
			fmt.Printf("%s discarded\n", s.ID)
		} else {
			fmt.Printf("%s %s pid %d\n", s.ID, s.State, s.Pid)
		}
	}
	for _, f := range res.Failed {
		fmt.Fprintf(os.Stderr, "%s: %s\n", f.SessionID, f.Error)
	}
	if *all && len(res.Sessions) == 0 && len(res.Failed) == 0 {
		fmt.Println("no suspended sessions")
	}
	if len(res.Failed) > 0 {
		return fmt.Errorf("%d sessions could not be %sd", len(res.Failed), cmd)
	}
	return nil
}

// turn renders the turn timer: "4m12s" (running), "=6m0s" (finished).
func turn(s protocol.SessionInfo) string {
	if s.TurnStartedAt == nil {
		return "-"
	}
	if s.TurnEndedAt != nil {
		return "=" + s.TurnEndedAt.Sub(*s.TurnStartedAt).Round(time.Second).String()
	}
	return time.Since(*s.TurnStartedAt).Round(time.Second).String()
}

// activity renders "[hook] Leyendo main.go", flagging stalls and progress.
func activity(s protocol.SessionInfo) string {
	out := "-"
	if s.Activity != "" {
		a := []rune(s.Activity)
		if len(a) > 50 {
			a = append(a[:49], '…')
		}
		out = "[" + s.ActivitySource + "] " + string(a)
	}
	if s.Progress != nil {
		out += fmt.Sprintf(" (%s %d%%)", s.Progress.State, s.Progress.Value)
	}
	if s.Stalled {
		out = "STALLED " + out
	}
	return out
}

// attention renders "permission*" (unread) or "-".
func attention(s protocol.SessionInfo) string {
	if s.AttentionKind == "" {
		return "-"
	}
	if s.Unread {
		return s.AttentionKind + "*"
	}
	return s.AttentionKind
}

func exitSuffix(s protocol.SessionInfo) string {
	if s.ExitCode != nil {
		return fmt.Sprintf("(%d)", *s.ExitCode)
	}
	if s.ResumeMode != "" { // suspended: how resume would restart it
		out := "[" + s.ResumeMode + "]"
		if s.ResumeErrorCode != "" {
			out += "!" + s.ResumeErrorCode
		}
		return out
	}
	return ""
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
