// Package notifycmd is the `notify` subcommand shared by the CLI and the
// desktop app binary, so agent hooks can call back whichever panal
// executable runs the daemon.
package notifycmd

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
	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/status"
)

// Name is the subcommand (os.Args[1]).
const Name = "notify"

// Usage is the one-line synopsis.
const Usage = `panal notify [--session ID] [--sock PATH] [--state S] [--kind K] [--message M] [--activity A]
  panal notify [--sock PATH] --from-hook claude|codex|gemini|opencode|cursor [JSON]   (hook payload on stdin or as last arg)`

const (
	timeout     = 3 * time.Second
	hookTimeout = 2 * time.Second
	maxPayload  = 1 << 20
)

// Run executes `notify`. With --from-hook it prints nothing and never fails
// (errors go to stderr, exit status stays 0) so it can never disturb or
// block the agent that called it.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("session", os.Getenv(protocol.EnvSession), "session id (default $PANAL_SESSION)")
	state := fs.String("state", "", "running | waiting_input | idle (aliases: waiting, done)")
	kind := fs.String("kind", "", "permission | question | done | error (implies --state when omitted)")
	msg := fs.String("message", "", "message shown in the notification")
	activity := fs.String("activity", "", "what the agent is doing (activity line); alone it only updates that")
	fromHook := fs.String("from-hook", "", "read an agent hook payload: claude, gemini, opencode, cursor (stdin) | codex (last argument)")
	sock := fs.String("sock", "", "daemon socket (default: $PANAL_HOME's, else $PANAL_SOCK)")
	if err := fs.Parse(args); err != nil {
		if *fromHook != "" || hasHookFlag(args) {
			return nil
		}
		return err
	}
	if *fromHook != "" {
		// Gemini CLI takes a hook's stderr as its output when stdout is
		// empty, and shows it: keep quiet there.
		if err := runHook(*fromHook, *id, *sock, fs.Args(), stdin, stdout); err != nil && *fromHook != hooks.AgentGemini {
			fmt.Fprintln(stderr, "panal notify:", err)
		}
		return nil
	}

	k, err := status.ParseKind(*kind)
	if err != nil {
		return err
	}
	if *state == "" {
		*state = stateForKind(k)
	}
	if *id == "" || (*state == "" && *activity == "") {
		return errors.New("usage: " + Usage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	info, err := send(ctx, *sock, protocol.NotifyParams{SessionID: *id, State: *state, Kind: string(k), Message: *msg, Activity: *activity})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s %s\n", info.ID, info.State)
	return nil
}

func hasHookFlag(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--from-hook") || strings.HasPrefix(a, "-from-hook") {
			return true
		}
	}
	return false
}

func stateForKind(k status.Kind) string {
	switch k {
	case status.KindPermission, status.KindQuestion:
		return string(status.WaitingInput)
	case status.KindDone, status.KindError:
		return string(status.Idle)
	}
	return ""
}

func runHook(agent, id, sock string, rest []string, stdin io.Reader, stdout io.Writer) error {
	if id == "" {
		return nil // not a panal session: nothing to report
	}
	deadline := time.Now().Add(hookTimeout)
	var payload []byte
	if len(rest) > 0 {
		payload = []byte(rest[len(rest)-1]) // codex: JSON is the last argv
	} else {
		var err error
		if payload, err = readBounded(stdin, time.Until(deadline)); err != nil {
			return err
		}
	}
	ev, ok, err := hooks.Parse(agent, payload)
	if err != nil {
		return err
	}
	// The agent's own session id lets a restarted daemon resume the
	// conversation; events without a state (SessionStart) carry only that.
	ref, transcript := hooks.SessionRef(agent, payload)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	// The instructions and messages from other sessions, for agents whose
	// hooks can add to the conversation (inboxHookFor). The daemon knows
	// this session from the connection. Messages are only marked
	// delivered once printed (msg.ack): a hook that times out or fails
	// leaves them pending.
	var inboxErr error
	if h, delivers := inboxHookFor(agent, payload); delivers {
		inboxErr = deliverInbox(ctx, sock, h, stdout)
		if h.stop && inboxErr == errContinued {
			ok, inboxErr = false, nil // the turn goes on: not idle
		}
	}
	model := hooks.Model(agent, payload)
	if !ok && ref == "" && model == "" {
		return inboxErr
	}
	p := protocol.NotifyParams{SessionID: id}
	if ok {
		p = protocol.NotifyParams{SessionID: id, State: string(ev.State), Kind: string(ev.Kind), Message: ev.Message,
			Activity: ev.Activity, ActivityKind: ev.ActivityKind, ActivityTarget: ev.ActivityTarget, Turn: ev.Turn}
	}
	if ref != "" {
		p.Agent, p.AgentSessionID, p.TranscriptPath = agent, ref, transcript
	}
	p.Model = model
	if _, err = send(ctx, sock, p); err != nil {
		return err
	}
	return inboxErr
}

// readBounded reads stdin until EOF, at most maxPayload bytes and d time.
func readBounded(r io.Reader, d time.Duration) ([]byte, error) {
	type result struct {
		b   []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(io.LimitReader(r, maxPayload))
		ch <- result{b, err}
	}()
	select {
	case res := <-ch:
		return res.b, res.err
	case <-time.After(d):
		return nil, errors.New("timed out reading hook payload")
	}
}

// send never starts a daemon: hooks must not spawn one. An explicit sock
// (what generated hooks pass) wins over the environment.
func send(ctx context.Context, sock string, p protocol.NotifyParams) (protocol.SessionInfo, error) {
	c, err := hookClient(sock)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	return c.NotifyEvent(ctx, p)
}

func hookClient(sock string) (*client.Client, error) {
	if sock == "" {
		var err error
		if sock, err = client.SocketFromEnv(); err != nil {
			return nil, err
		}
	}
	return client.New(sock), nil
}

// errContinued: the Stop hook blocked, the turn goes on.
var errContinued = errors.New("turn continued")

// deliverInbox prints the session's pending messages for hook h and
// acknowledges them. It returns errContinued when a Stop was blocked.
func deliverInbox(ctx context.Context, sock string, h inboxHook, stdout io.Writer) error {
	if h.hold {
		return nil
	}
	c, err := hookClient(sock)
	if err != nil {
		return err
	}
	r, err := c.MsgTakePeek(ctx, h.start)
	if err != nil {
		return err
	}
	out, ids := hookOutput(h, r)
	if out == nil {
		return nil
	}
	if _, err := stdout.Write(out); err != nil {
		return err
	}
	if len(ids) > 0 {
		if err := c.MsgAck(ctx, ids); err != nil {
			return err
		}
	}
	if h.stop {
		return errContinued
	}
	return nil
}
