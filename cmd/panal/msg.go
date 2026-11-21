package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/protocol"
)

const msgUsage = `usage:
  panal msg peers [--session ID]       the other sessions of this project
  panal msg send <to> <text…>          to: a session id, a name (msg peers) or all;
                                          text "-" reads it from stdin
  panal msg read                       your new messages (marks them delivered)
  panal msg list [--pending] [--session ID | --project ID]   history`

func cmdMsg(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New(msgUsage)
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "peers":
		fs := flag.NewFlagSet("msg peers", flag.ContinueOnError)
		sid := fs.String("session", "", "outside a session: whose project")
		if err := parseInterspersed(fs, rest); err != nil {
			return err
		}
		r, err := c.MsgPeers(ctx, *sid)
		if err != nil {
			return err
		}
		fmt.Print(formatPeers(r))
		return nil
	case "send":
		fs := flag.NewFlagSet("msg send", flag.ContinueOnError)
		proj := fs.String("project", "", "outside a session: the project (when <to> is a name or all)")
		if err := parseInterspersed(fs, rest); err != nil {
			return err
		}
		if fs.NArg() < 2 {
			return errors.New(msgUsage)
		}
		body := strings.Join(fs.Args()[1:], " ")
		if body == "-" {
			b, err := io.ReadAll(io.LimitReader(os.Stdin, mailbox.MaxBody+1))
			if err != nil {
				return err
			}
			body = string(b)
		}
		ms, err := c.MsgSend(ctx, protocol.MsgSendParams{To: fs.Arg(0), Body: body, ProjectID: *proj})
		if err != nil {
			return err
		}
		for _, m := range ms {
			fmt.Printf("sent #%d to %s (%s)\n", m.ID, m.ToName, m.To)
		}
		return nil
	case "read":
		if len(rest) != 0 {
			return errors.New(msgUsage)
		}
		r, err := c.MsgTake(ctx, false)
		if err != nil {
			return err
		}
		switch {
		case !r.Enabled:
			fmt.Println("messages between sessions are off (panal Settings › Messages)")
		case len(r.Messages) == 0:
			fmt.Println("no new messages")
		default:
			fmt.Println(mailbox.Preamble)
			fmt.Println()
			fmt.Print(mailbox.ForAgent(r.Messages))
		}
		if r.Enabled && r.Held > 0 {
			fmt.Printf("%d more %s waiting for your user to deliver %s (this project's messages are manual)\n",
				r.Held, plural(r.Held, "message is", "messages are"), plural(r.Held, "it", "them"))
		}
		return nil
	case "list":
		fs := flag.NewFlagSet("msg list", flag.ContinueOnError)
		sid := fs.String("session", "", "messages of this session")
		proj := fs.String("project", "", "messages of this project")
		pending := fs.Bool("pending", false, "only those not delivered yet")
		if err := parseInterspersed(fs, rest); err != nil {
			return err
		}
		ms, err := c.MsgList(ctx, protocol.MsgListParams{SessionID: *sid, ProjectID: *proj, PendingOnly: *pending})
		if err != nil {
			return err
		}
		for _, m := range ms {
			fmt.Println(formatListLine(m))
		}
		return nil
	}
	return errors.New(msgUsage)
}

func formatPeers(r protocol.MsgPeers) string {
	var b strings.Builder
	fmt.Fprintf(&b, "project %s (%s)\n", r.ProjectName, r.ProjectID)
	if r.Self != nil {
		fmt.Fprintf(&b, "you: %s\n", peerLine(*r.Self))
	}
	if len(r.Peers) == 0 {
		b.WriteString("no other sessions\n")
	}
	for _, p := range r.Peers {
		fmt.Fprintf(&b, "  %s\n", peerLine(p))
	}
	if !r.Enabled {
		b.WriteString("messages between sessions are off (panal Settings › Messages)\n")
	}
	return b.String()
}

func peerLine(p protocol.MsgPeer) string {
	s := fmt.Sprintf("%s  %s [%s] %s", p.ID, p.Name, p.ProfileID, p.State)
	switch {
	case p.Mode == protocol.ModeWorktree && p.Branch != "":
		s += " · worktree " + p.Branch
	case p.SameFolder:
		s += " · same folder as you"
	}
	if p.Activity != "" {
		s += " · " + p.Activity
	}
	return s
}

func formatListLine(m mailbox.Message) string {
	st := "pending"
	if !m.Pending() {
		st = "delivered"
	}
	to := m.To
	if m.ToName != "" {
		to = m.ToName + " (" + m.To + ")"
	}
	return fmt.Sprintf("#%d %s  %s → %s  [%s]  %s", m.ID, m.At.Local().Format("01-02 15:04"), m.Sender(), to, st,
		strings.ReplaceAll(m.Body, "\n", " ⏎ "))
}

func cmdWait(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute, "give up after this long (exit status 2)")
	fs.Bool("idle", true, "wait until it is idle, waiting for input or exited (the default)")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: panal wait <sessionId> [--timeout 30m]")
	}
	id := fs.Arg(0)
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	st, err := c.WaitIdle(ctx, id)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "panal: %s is still busy after %s\n", id, *timeout)
			os.Exit(2)
		}
		return err
	}
	fmt.Printf("%s %s\n", id, st)
	return nil
}
