// Package notifycmd is the `notify` subcommand shared by the CLI and the
// desktop app binary, so agent hooks can call back whichever agenterm
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

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/status"
)

// Name is the subcommand (os.Args[1]).
const Name = "notify"

// Usage is the one-line synopsis.
const Usage = `agenterm notify [--session ID] [--state S] [--kind K] [--message M]
  agenterm notify --from-hook claude|codex [JSON]   (hook payload on stdin or as last arg)`

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
	id := fs.String("session", os.Getenv(protocol.EnvSession), "session id (default $AGENTERM_SESSION)")
	state := fs.String("state", "", "running | waiting_input | idle (aliases: waiting, done)")
	kind := fs.String("kind", "", "permission | question | done | error (implies --state when omitted)")
	msg := fs.String("message", "", "message shown in the notification")
	fromHook := fs.String("from-hook", "", "read an agent hook payload: claude (stdin) | codex (last argument)")
	if err := fs.Parse(args); err != nil {
		if *fromHook != "" || hasHookFlag(args) {
			return nil
		}
		return err
	}
	if *fromHook != "" {
		if err := runHook(*fromHook, *id, fs.Args(), stdin); err != nil {
			fmt.Fprintln(stderr, "agenterm notify:", err)
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
	if *id == "" || *state == "" {
		return errors.New("usage: " + Usage)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	info, err := send(ctx, protocol.NotifyParams{SessionID: *id, State: *state, Kind: string(k), Message: *msg})
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

func runHook(agent, id string, rest []string, stdin io.Reader) error {
	if id == "" {
		return nil // not an agenterm session: nothing to report
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
	if err != nil || !ok {
		return err
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	_, err = send(ctx, protocol.NotifyParams{SessionID: id, State: string(ev.State), Kind: string(ev.Kind), Message: ev.Message})
	return err
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

// send never starts a daemon: hooks must not spawn one.
func send(ctx context.Context, p protocol.NotifyParams) (protocol.SessionInfo, error) {
	sock, err := client.SocketFromEnv()
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	return client.New(sock).NotifyEvent(ctx, p)
}
