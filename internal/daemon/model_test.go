package daemon_test

import (
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/protocol"
)

func TestSessionModel(t *testing.T) {
	c, p, stop := startDaemon(t)
	ctx := ctxT(t)
	writeProfiles(t, p.Profiles(), `
- id: modelled
  cmd: /bin/sh
  args: ["-c", "sleep 30", "--model", "test-model-1"]
`)
	setRestore(t, p, "always")
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	// From the command line.
	m, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "modelled"})
	if err != nil || m.Model != "test-model-1" {
		t.Fatalf("args model: %q %v", m.Model, err)
	}
	// From the hooks: one event per change.
	s, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	events := eventStream(t, c, "")
	for range 2 {
		if _, err := c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: s.ID, Model: "claude-opus-5-5"}); err != nil {
			t.Fatal(err)
		}
	}
	e := waitEvent(t, events, "session.model", func(e protocol.Event) bool { return e.Type == protocol.EventSessionModel })
	if e.SessionID != s.ID || e.Model != "claude-opus-5-5" {
		t.Fatalf("event %+v", e)
	}
	select {
	case e := <-events:
		if e.Type == protocol.EventSessionModel {
			t.Fatalf("second event for the same model: %+v", e)
		}
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: s.ID, Model: "bad id with spaces"}); err != nil {
		t.Fatal(err)
	}
	if got := findSession(t, c, s.ID).Model; got != "claude-opus-5-5" {
		t.Fatalf("model = %q", got)
	}

	// A restart keeps it.
	stop()
	c2, _ := restartDaemon(t, p)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := findSession(t, c2, s.ID).Model; got == "claude-opus-5-5" {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("after restart model = %q", got)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
