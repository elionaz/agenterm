package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/protocol"
)

const handoffUsage = "usage: panal handoff [--worktree] [--keep] [--note FILE|-] <sessionId> <profile>\n" +
	"       panal handoff --into <sessionId> [--keep] [--note FILE|-] <sessionId>\n" +
	"       panal handoff --print <sessionId>"

// cmdHandoff continues a session's work with another agent (or the same
// one in a new worktree): it drafts the summary (or reads --note), starts
// the agent with it and closes the session unless --keep. --print only
// shows the draft.
func cmdHandoff(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	worktree := fs.Bool("worktree", false, "start it in a new worktree from the session's current commit")
	keep := fs.Bool("keep", false, "leave the original session running")
	noteFile := fs.String("note", "", "the summary to hand over (a file, or - for stdin); default: the drafted one")
	printOnly := fs.Bool("print", false, "print the drafted summary and stop")
	into := fs.String("into", "", "hand the work to this live session instead of starting one")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if *printOnly {
		if fs.NArg() != 1 {
			return errors.New(handoffUsage)
		}
		n, err := c.HandoffNote(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		fmt.Print(n.Text)
		return nil
	}
	if (*into == "" && fs.NArg() != 2) || (*into != "" && fs.NArg() != 1) {
		return errors.New(handoffUsage)
	}
	profile := ""
	if *into == "" {
		profile = fs.Arg(1)
	}
	var note string
	switch *noteFile {
	case "":
		n, err := c.HandoffNote(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		note = n.Text
	case "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		note = string(b)
	default:
		b, err := os.ReadFile(*noteFile)
		if err != nil {
			return err
		}
		note = string(b)
	}
	where := protocol.HandoffSame
	if *worktree {
		where = protocol.HandoffWorktree
	}
	info, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: fs.Arg(0), ProfileID: profile, TargetID: *into, Where: where, Note: note, Keep: *keep})
	if err != nil {
		return err
	}
	fmt.Println(info.ID)
	return nil
}
