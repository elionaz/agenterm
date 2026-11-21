package client

import (
	"context"
	"fmt"

	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/status"
)

// WaitRemoved is the state WaitIdle reports for a session that was removed.
const WaitRemoved = "removed"

// WaitDone reports whether state ends a wait: the session finished its
// turn (idle), waits for its user, or exited.
func WaitDone(state string) bool {
	switch status.State(state) {
	case status.Idle, status.WaitingInput, status.Exited:
		return true
	}
	return false
}

// WaitIdle blocks until session id finishes its turn, waits for its user,
// exits or is removed, and returns that state (WaitRemoved for the last).
// It gives up with ctx's error when ctx ends first.
func (c *Client) WaitIdle(ctx context.Context, id string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Subscribe first, then look: a change in between is not missed.
	ev, err := c.Subscribe(ctx)
	if err != nil {
		return "", err
	}
	defer ev.Close()
	go func() {
		<-ctx.Done()
		ev.Close()
	}()
	ss, err := c.Sessions(ctx)
	if err != nil {
		return "", err
	}
	found := false
	for _, s := range ss {
		if s.ID == id {
			found = true
			if WaitDone(s.State) {
				return s.State, nil
			}
		}
	}
	if !found {
		return "", fmt.Errorf("session not found: %s", id)
	}
	for {
		e, err := ev.Next()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", err
		}
		if e.SessionID != id {
			continue
		}
		if e.Type == protocol.EventSessionRemoved {
			return WaitRemoved, nil
		}
		if e.Type == protocol.EventSessionState && WaitDone(e.State) {
			return e.State, nil
		}
	}
}
