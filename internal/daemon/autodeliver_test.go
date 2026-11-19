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

	// Off (set so in the file; it is on by default): the message waits for Deliver.
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

// A heads-up from another session (nobody asked for it) does not wake an
// idle Claude session: it waits for its next turn. A message from the
// user still does.
func TestAutoDeliverOnlyReplies(t *testing.T) {
	defer daemon.SetAutoTiming(100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond)()
	cli := agentermCLI(t)
	c, p, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	if err := os.WriteFile(p.Settings(), []byte("ui:\n  language: en\nmessages:\n  enabled: true\n  auto_deliver: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
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
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "replies")
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
	if _, err := c.Notify(ctx, b.ID, "running"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Notify(ctx, b.ID, "idle"); err != nil {
		t.Fatal(err)
	}

	runIn(t, c, sh.ID, cli+" msg send "+b.ID+" 'heads-up' >/dev/null")
	out.waitNudges(t, 0, "a heads-up from another session")
	if l, err := c.MsgList(ctx, protocol.MsgListParams{SessionID: b.ID}); err != nil || len(l) != 1 || !l[0].Pending() {
		t.Fatalf("the heads-up should stay pending: %+v, %v", l, err)
	}

	if _, err := c.MsgSend(ctx, protocol.MsgSendParams{To: b.ID, Body: "from the user"}); err != nil {
		t.Fatal(err)
	}
	out.waitNudges(t, 1, "a message from the user")
}

// On by default; agents without message hooks get the "msg read" line only
// when they reported the end of their turn, and the session says why
// messages wait otherwise.
func TestAutoDeliverDefaultsAndOtherAgents(t *testing.T) {
	defer daemon.SetAutoTiming(100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond)()
	c, p, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	// No messages section in settings.yaml: the defaults apply.
	if err := os.WriteFile(p.Settings(), []byte("ui:\n  language: en\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := t.TempDir() + "/fakeagent"
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nstty -echo -isig\nexec cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	profs := testProfiles +
		"- id: fakeclaude\n  cmd: " + fake + "\n  status: hooks\n  setup_hooks: claude\n" +
		"- id: fakecodex\n  cmd: " + fake + "\n  idle_seconds: 0.3\n"
	if err := os.WriteFile(p.Profiles(), []byte(profs), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RescanProfiles(ctx); err != nil {
		t.Fatal(err)
	}
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "defaults")
	if err != nil {
		t.Fatal(err)
	}
	attach := func(id string) (*output, func()) {
		t.Helper()
		st, err := c.Attach(ctx, id, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
		return collect(t, st), func() { st.Close() }
	}
	hold := func(id string) string {
		t.Helper()
		return findSession(t, c, id).MsgHold
	}
	send := func(id, body string) {
		t.Helper()
		if _, err := c.MsgSend(ctx, protocol.MsgSendParams{To: id, Body: body}); err != nil {
			t.Fatal(err)
		}
	}

	// Claude, idle: a message from the user arrives by itself.
	cl, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "fakeclaude"})
	if err != nil {
		t.Fatal(err)
	}
	clOut, closeCl := attach(cl.ID)
	defer closeCl()
	c.Notify(ctx, cl.ID, "running")
	c.Notify(ctx, cl.ID, "idle")
	send(cl.ID, "hello")
	clOut.waitNudges(t, 1, "claude, default settings")
	// Told: the UIs say so (it stays pending until the agent takes them).
	if h := hold(cl.ID); h != "nudged" {
		t.Fatalf("hold after the nudge = %q, want nudged", h)
	}

	// Another agent that only went quiet: held, with the reason.
	cx, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "fakecodex"})
	if err != nil {
		t.Fatal(err)
	}
	cxOut, closeCx := attach(cx.ID)
	defer closeCx()
	deadline := time.Now().Add(5 * time.Second)
	for findSession(t, c, cx.ID).State != "idle" {
		if time.Now().After(deadline) {
			t.Fatal("the heuristic never went idle")
		}
		time.Sleep(50 * time.Millisecond)
	}
	send(cx.ID, "one")
	cxOut.waitNudges(t, 0, "other agent, quiet only")
	if h := hold(cx.ID); h != "no turn end" {
		t.Fatalf("hold = %q, want no turn end", h)
	}
	// It reports the end of its turn (like codex's notify): it is told to
	// read them.
	c.Notify(ctx, cx.ID, "running")
	c.Notify(ctx, cx.ID, "idle")
	send(cx.ID, "two")
	cxOut.waitNudges(t, 1, "other agent, turn end reported")
	cxOut.mu.Lock()
	told := bytes.Contains(cxOut.buf.Bytes(), []byte("agenterm msg read"))
	cxOut.mu.Unlock()
	if !told {
		t.Fatal("the other agent was not told to run agenterm msg read")
	}

	// Explicitly off in the file: nothing by itself, and it says so.
	if err := os.WriteFile(p.Settings(), []byte("ui:\n  language: en\nmessages:\n  auto_deliver: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.Notify(ctx, cl.ID, "running")
	c.Notify(ctx, cl.ID, "idle")
	send(cl.ID, "while off")
	clOut.waitNudges(t, 1, "off")
	if h := hold(cl.ID); h != "off" {
		t.Fatalf("hold while off = %q", h)
	}
}
