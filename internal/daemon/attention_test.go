package daemon_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/notifycmd"
	"github.com/elionaz/agenterm/internal/protocol"
)

// eventStream subscribes and forwards events to a channel.
func eventStream(t *testing.T, c *client.Client, clientKind string) <-chan protocol.Event {
	t.Helper()
	ev, err := c.SubscribeAs(ctxT(t), clientKind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ev.Close() })
	ch := make(chan protocol.Event, 128)
	go func() {
		defer close(ch)
		for {
			e, err := ev.Next()
			if err != nil {
				return
			}
			ch <- e
		}
	}()
	return ch
}

func waitEvent(t *testing.T, ch <-chan protocol.Event, what string, pred func(protocol.Event) bool) protocol.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatalf("event stream closed waiting for %s", what)
			}
			if pred(e) {
				return e
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s", what)
		}
	}
}

func writeProfiles(t *testing.T, path, extra string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(testProfiles+extra), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalSignalsRaiseAttention(t *testing.T) {
	notes := make(chan daemon.Notification, 16)
	c, p, _ := startDaemonWith(t, func(n daemon.Notification) { notes <- n })
	ctx := ctxT(t)
	writeProfiles(t, p.Profiles(), `
- id: sig
  cmd: /bin/sh
  args: ["-c", "printf '\\033]9;4;1;50\\007'; printf '\\033]9;Need approval\\007'; read a; printf '\\a'; read b; printf '\\033]777;notify;Codex;Approve rm?\\007'; read c"]
`)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	evc := eventStream(t, c, "") // not a UI: the daemon notifies itself

	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	isMine := func(e protocol.Event) bool { return e.SessionID == sess.ID }

	// 1. OSC 9 (the 9;4 progress report before it is ignored).
	e := waitEvent(t, evc, "OSC 9 attention", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.Type != protocol.EventSessionState || e.State != "waiting_input" || e.AttentionKind != "question" ||
		e.AttentionMessage != "Need approval" || !e.Unread || e.ProjectName != "demo" || e.ProfileID != "sig" {
		t.Fatalf("bad event %+v", e)
	}
	select {
	case n := <-notes:
		if n.Title != "demo · sig" || n.Body != "Need approval" || !n.Sound {
			t.Fatalf("bad fallback notification %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no fallback notification without a UI subscriber")
	}
	if info := findSession(t, c, sess.ID); info.AttentionMessage != "Need approval" || !info.Unread || info.AttentionAt == nil {
		t.Fatalf("session info lacks attention: %+v", info)
	}

	// 2. Ack clears unread (attention-only event).
	if _, err := c.Ack(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	e = waitEvent(t, evc, "ack", func(e protocol.Event) bool { return isMine(e) && e.Type == protocol.EventSessionAttention })
	if e.Unread || e.AttentionKind != "question" || e.Raised {
		t.Fatalf("bad ack event %+v", e)
	}
	if info := findSession(t, c, sess.ID); info.Unread || info.State != "waiting_input" {
		t.Fatalf("after ack: %+v", info)
	}

	// 3. Typing answers the wait; the BEL that follows raises a new one.
	st, err := c.Attach(ctx, sess.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	// Automatic terminal replies (focus report, DA) are not the user.
	st.Input([]byte("\x1b[I\x1b[?1;2c"))
	time.Sleep(100 * time.Millisecond)
	if info := findSession(t, c, sess.ID); info.State != "waiting_input" {
		t.Fatalf("terminal reply ended the wait: %+v", info)
	}
	st.Input([]byte("x\n"))
	waitEvent(t, evc, "running after input", func(e protocol.Event) bool { return isMine(e) && e.State == "running" })
	e = waitEvent(t, evc, "BEL attention", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.State != "waiting_input" || e.AttentionMessage != "" || e.AttentionKind != "question" {
		t.Fatalf("bad BEL event %+v", e)
	}
	select {
	case n := <-notes:
		if n.Body != "Está esperando tu respuesta" {
			t.Fatalf("fallback body %q", n.Body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no fallback notification for BEL")
	}

	// 4. With a UI subscribed the daemon stays quiet (the app notifies).
	uic := eventStream(t, c, protocol.ClientUI)
	time.Sleep(50 * time.Millisecond)
	st.Input([]byte("y\n"))
	e = waitEvent(t, uic, "OSC 777 attention", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.AttentionMessage != "Codex: Approve rm?" {
		t.Fatalf("bad OSC 777 event %+v", e)
	}
	select {
	case n := <-notes:
		t.Fatalf("daemon notified although a UI is subscribed: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}

	// The bytes reach the terminal untouched.
	_, st2 := attachSnapshot(t, c, sess.ID)
	if !bytes.Contains(st2, []byte("\x1b]9;Need approval\x07")) || !bytes.Contains(st2, []byte("\a")) {
		t.Fatalf("signals stripped from output: %q", st2)
	}
}

func attachSnapshot(t *testing.T, c *client.Client, id string) (*client.Stream, []byte) {
	t.Helper()
	st, err := c.Attach(ctxT(t), id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
}

func TestNotifyKindsAndHookPayloads(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	evc := eventStream(t, c, protocol.ClientUI)
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	isMine := func(e protocol.Event) bool { return e.SessionID == sess.ID }

	t.Setenv(protocol.EnvSession, sess.ID)
	t.Setenv(protocol.EnvSock, p.Socket())
	run := func(stdin string, args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if err := notifycmd.Run(args, strings.NewReader(stdin), &out, &errb); err != nil {
			t.Fatalf("notify %v: %v (%s)", args, err, errb.String())
		}
		return out.String()
	}

	// --kind implies the state.
	run("", "--kind", "permission", "--message", "Allow Bash?")
	e := waitEvent(t, evc, "permission", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.State != "waiting_input" || e.AttentionKind != "permission" || e.AttentionMessage != "Allow Bash?" {
		t.Fatalf("bad event %+v", e)
	}

	// Claude hooks: stdin payloads, nothing on stdout.
	if out := run(`{"session_id":"abc","cwd":"/x","hook_event_name":"UserPromptSubmit","prompt":"hola"}`, "--from-hook", "claude"); out != "" {
		t.Fatalf("hook mode printed %q (UserPromptSubmit stdout becomes Claude context!)", out)
	}
	waitEvent(t, evc, "running", func(e protocol.Event) bool { return isMine(e) && e.State == "running" })
	run(`{"session_id":"abc","cwd":"/x","hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Hecho: 3 tests verdes"}`, "--from-hook", "claude")
	e = waitEvent(t, evc, "done", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.State != "idle" || e.AttentionKind != "done" || e.AttentionMessage != "Hecho: 3 tests verdes" {
		t.Fatalf("bad stop event %+v", e)
	}

	// Codex notify: JSON as last argument.
	run("", "--from-hook", "codex", `{"type":"agent-turn-complete","turn-id":"1","last-assistant-message":"Listo"}`)
	e = waitEvent(t, evc, "codex done", func(e protocol.Event) bool { return isMine(e) && e.Raised })
	if e.AttentionMessage != "Listo" || e.AttentionKind != "done" {
		t.Fatalf("bad codex event %+v", e)
	}

	// Hook mode never fails: bad JSON, unknown agent, no daemon.
	run("garbage", "--from-hook", "claude")
	run("{}", "--from-hook", "nope")
	t.Setenv(protocol.EnvSock, filepath.Join(t.TempDir(), "none.sock"))
	start := time.Now()
	run(`{"hook_event_name":"Stop"}`, "--from-hook", "claude")
	if time.Since(start) > 3*time.Second {
		t.Fatal("hook notify took too long without a daemon")
	}
	// The plain form still reports errors.
	if err := notifycmd.Run([]string{"--state", "bogus"}, strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("expected error")
	}
}

func TestClaudeHookInjection(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	script := filepath.Join(t.TempDir(), "fake-claude")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'ARG:%s\\n' \"$@\"\nsleep 5\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeProfiles(t, p.Profiles(), `
- id: fakeclaude
  cmd: `+script+`
  args: ["--model", "x"]
  status: hooks
  setup_hooks: claude
`)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "fakeclaude"})
	if err != nil {
		t.Fatal(err)
	}
	settingsFile := filepath.Join(p.HooksDir(), "claude-settings.json")
	st, err := c.Attach(ctx, sess.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	out := readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte("ARG:x")) })
	want := "ARG:--settings\r\nARG:" + settingsFile + "\r\nARG:--model\r\nARG:x"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("args %q, want %q", out, want)
	}
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"'/opt/agenterm test/agenterm' notify --from-hook claude"`) {
		t.Fatalf("settings file: %s", data)
	}
	if !strings.HasPrefix(settingsFile, p.RuntimeDir) {
		t.Fatal("hook files must live in the runtime dir")
	}
}
