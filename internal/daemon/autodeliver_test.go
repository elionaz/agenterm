package daemon_test

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/protocol"
)

// fakeAgent is an agent that prints back what it is sent: cat, with the
// terminal's echo off (else a nudge shows twice) and a marker once it is
// (waitReady), so nothing is typed before stty has run.
const fakeAgent = "#!/bin/sh\nstty -echo -isig\necho fake-ready\nexec cat\n"

// output collects a session's terminal output in the background.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func collect(t *testing.T, st *client.Stream, replay []byte) *output {
	o := &output{}
	o.buf.Write(replay)
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

// waitReady waits for fakeAgent's marker: its echo is off from then on.
func (o *output) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		o.mu.Lock()
		ok := bytes.Contains(o.buf.Bytes(), []byte("fake-ready"))
		o.mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the fake agent never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// nudges counts the "[panal] There are new messages…" lines typed.
func (o *output) nudges() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return bytes.Count(o.buf.Bytes(), []byte("[panal]"))
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
	// A stand-in for Claude: takes the --settings panal passes, echoes
	// nothing itself, prints what it is typed (one line per nudge).
	fake := t.TempDir() + "/fakeclaude"
	if err := os.WriteFile(fake, []byte(fakeAgent), 0o755); err != nil {
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
	replay := readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	out := collect(t, st, replay)
	out.waitReady(t)
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
// Automatic projects deliver any message (a heads-up too); manual ones hold
// the sessions' messages until the user delivers them; switching to
// automatic lets the held ones out.
func TestMessagesModes(t *testing.T) {
	defer daemon.SetAutoTiming(100*time.Millisecond, 300*time.Millisecond, 200*time.Millisecond)()
	cli := panalCLI(t)
	c, p, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	if err := os.WriteFile(p.Settings(), []byte("ui:\n  language: en\nmessages:\n  enabled: true\n  auto_deliver: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := t.TempDir() + "/fakeclaude"
	if err := os.WriteFile(fake, []byte(fakeAgent), 0o755); err != nil {
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
	replay := readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	out := collect(t, st, replay)
	out.waitReady(t)
	if _, err := c.Notify(ctx, b.ID, "running"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Notify(ctx, b.ID, "idle"); err != nil {
		t.Fatal(err)
	}

	evc := eventStream(t, c, protocol.ClientUI)
	send := func(text string) {
		t.Helper()
		runIn(t, c, sh.ID, cli+" msg send "+b.ID+" '"+text+"' >/dev/null")
	}
	setMode := func(m string) {
		t.Helper()
		if _, err := c.UpdateProject(ctx, protocol.ProjectUpdateParams{ID: proj.ID, Messages: &m}); err != nil {
			t.Fatal(err)
		}
	}
	idle := func() {
		t.Helper()
		if _, err := c.Notify(ctx, b.ID, "running"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Notify(ctx, b.ID, "idle"); err != nil {
			t.Fatal(err)
		}
	}

	// Automatic: a heads-up from another session wakes it, and the UIs see it.
	send("heads-up")
	e := waitEvent(t, evc, "message.sent", func(e protocol.Event) bool { return e.Type == protocol.EventMessageSent && e.MsgTo == b.ID })
	if e.MsgFrom != sh.ID || e.MsgExcerpt != "heads-up" || e.MsgHeld || e.ProjectID != proj.ID {
		t.Fatalf("sent event = %+v", e)
	}
	out.waitNudges(t, 1, "a heads-up in an automatic project")
	idle()

	// Manual: held, nothing typed, the hook gets nothing.
	setMode("manual")
	send("held one")
	e = waitEvent(t, evc, "held message.sent", func(e protocol.Event) bool { return e.Type == protocol.EventMessageSent && e.MsgExcerpt == "held one" })
	if !e.MsgHeld {
		t.Fatalf("not held: %+v", e)
	}
	out.waitNudges(t, 1, "a message in a manual project")
	if !held(t, c, b.ID, "held one") {
		t.Fatal("the message is not held")
	}
	if s := findSession(t, c, b.ID); s.MsgPending != 2 || s.MsgHold != "held" {
		t.Fatalf("session = pending %d hold %q", s.MsgPending, s.MsgHold)
	}
	// The user's own message is never held.
	if l, _ := c.MsgSend(ctx, protocol.MsgSendParams{To: b.ID, Body: "from the user"}); len(l) != 1 || l[0].Held {
		t.Fatalf("user message = %+v", l)
	}
	// Deliver releases and nudges.
	if _, err := c.MsgDeliver(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	out.waitNudges(t, 2, "Deliver in a manual project")
	if held(t, c, b.ID, "held one") {
		t.Fatal("still held after Deliver")
	}
	idle()

	// Held, then the project goes automatic: it goes out by itself.
	send("waiting")
	waitEvent(t, evc, "third send", func(e protocol.Event) bool { return e.Type == protocol.EventMessageSent && e.MsgExcerpt == "waiting" })
	out.waitNudges(t, 2, "still manual")
	setMode("auto")
	out.waitNudges(t, 3, "switched to automatic")
	if held(t, c, b.ID, "waiting") {
		t.Fatal("still held after switching to automatic")
	}

	// Release of a whole project.
	setMode("manual")
	send("one more")
	waitEvent(t, evc, "fourth send", func(e protocol.Event) bool { return e.Type == protocol.EventMessageSent && e.MsgExcerpt == "one more" })
	if !held(t, c, b.ID, "one more") {
		t.Fatal("not held")
	}
	if n, err := c.MsgRelease(ctx, proj.ID); err != nil || n != 1 {
		t.Fatalf("release = %d, %v", n, err)
	}
	if held(t, c, b.ID, "one more") {
		t.Fatal("still held after release")
	}
}

// held reports whether the message with body to session id is held.
func held(t *testing.T, c *client.Client, id, body string) bool {
	t.Helper()
	l, err := c.MsgList(ctxT(t), protocol.MsgListParams{SessionID: id})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range l {
		if m.Body == body {
			return m.Held
		}
	}
	t.Fatalf("no message %q in %+v", body, l)
	return false
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
	if err := os.WriteFile(fake, []byte(fakeAgent), 0o755); err != nil {
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
		replay := readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
		o := collect(t, st, replay)
		o.waitReady(t)
		return o, func() { st.Close() }
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
	told := bytes.Contains(cxOut.buf.Bytes(), []byte("panal msg read"))
	cxOut.mu.Unlock()
	if !told {
		t.Fatal("the other agent was not told to run panal msg read")
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
