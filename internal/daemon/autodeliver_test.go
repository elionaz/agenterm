package daemon_test

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/protocol"
)

// output collects a session's terminal output in the background.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func collect(t *testing.T, st *client.Stream) *output {
	o := &output{}
	go func() {
		for {
			f, err := st.Recv()
			if err != nil {
				return
			}
			if f.Type == protocol.FrameOutput {
				o.mu.Lock()
				o.buf.Write(f.Payload)
				o.mu.Unlock()
			}
		}
	}()
	return o
}

// nudges counts the "[agenterm] There are new messages…" lines typed.
func (o *output) nudges() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Count(o.buf.Bytes(), []byte("[agenterm]"))
}

// waitNudges waits for n nudges; with stay, also checks no more come.
func (o *output) waitNudges(t *testing.T, n int, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for o.nudges() < n && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond) // anything extra shows up meanwhile
	if got := o.nudges(); got != n {
		t.Fatalf("%s: %d nudges, want %d\n%s", what, got, n, o.buf.String())
	}
}

func TestAutoDeliver(t *testing.T) {
	defer daemon.SetAutoTiming(100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond)()
	c, p, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	setAuto := func(on bool) {
		t.Helper()
		v := "false"
		if on {
			v = "true"
		}
		if err := os.WriteFile(p.Settings(), []byte("ui:\n  language: en\nmessages:\n  enabled: true\n  auto_deliver: "+v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setAuto(false)
	// A stand-in for Claude: takes the --settings agenterm passes, echoes
	// nothing itself, prints what it is typed (one line per nudge).
	fake := t.TempDir() + "/fakeclaude"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nstty -echo -isig\nexec cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	profs := testProfiles + "- id: fakeclaude\n  cmd: " + fake + "\n  status: hooks\n  setup_hooks: claude\n"
	if err := os.WriteFile(p.Profiles(), []byte(profs), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RescanProfiles(ctx); err != nil {
		t.Fatal(err)
	}
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "auto")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "fakeclaude"})
	if err != nil {
		t.Fatal(err)
	}
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, b.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	out := collect(t, st)
	idle := func() {
		t.Helper()
		if _, err := c.Notify(ctx, b.ID, "running"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Notify(ctx, b.ID, "idle"); err != nil {
			t.Fatal(err)
		}
	}
	send := func(body string) {
		t.Helper()
		if _, err := c.MsgSend(ctx, protocol.MsgSendParams{To: b.ID, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	idle()

	// Off (the default): the message waits for Deliver.
	send("one")
	out.waitNudges(t, 0, "off")

	// On: an idle Claude session is nudged, once.
	setAuto(true)
	idle()
	out.waitNudges(t, 1, "on, after going idle")
	send("two")
	out.waitNudges(t, 2, "on, new message while idle")

	// Not while it works, nor waiting for an answer.
	if _, err := c.Notify(ctx, b.ID, "running"); err != nil {
		t.Fatal(err)
	}
	send("three")
	out.waitNudges(t, 2, "running")
	if _, err := c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: b.ID, State: "waiting_input", Kind: "permission"}); err != nil {
		t.Fatal(err)
	}
	out.waitNudges(t, 2, "permission prompt")

	// Never over unsent input: the user typed without Enter, or added a line
	// (Shift+Enter sends LF). Clearing it (Ctrl-C) lifts the hold by itself.
	if err := st.Input([]byte("half a prompt")); err != nil {
		t.Fatal(err)
	}
	idle()
	send("four")
	out.waitNudges(t, 2, "draft")
	if err := st.Input([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	out.waitNudges(t, 2, "draft with a new line")
	if err := st.Input([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	out.waitNudges(t, 3, "draft cleared")

	// Agents waking each other up: at most AutoChainMax nudges to a session
	// without the user acting on it. A message from the user ("four") began
	// its chain: nudge 3 is its first. Typing into another session of the
	// project does not reset it.
	runIn(t, c, sh.ID, "true")
	for i := 1; i <= daemon.AutoChainMax+1; i++ {
		idle()
		want := 3 + min(i, daemon.AutoChainMax-1)
		out.waitNudges(t, want, "chain")
	}
	// The user acts on it (a prompt submitted): a new chain.
	if err := st.Input([]byte("go on\r")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(350 * time.Millisecond) // past the quiet time
	idle()
	out.waitNudges(t, 3+daemon.AutoChainMax, "chain after the user acted")
}
