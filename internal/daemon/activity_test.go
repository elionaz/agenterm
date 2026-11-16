package daemon_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/notifycmd"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
)

func TestHookActivityAndTimers(t *testing.T) {
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
	t.Setenv(paths.EnvHome, "")
	hook := func(payload string) {
		t.Helper()
		var out, errb bytes.Buffer
		if err := notifycmd.Run([]string{"--from-hook", "claude"}, strings.NewReader(payload), &out, &errb); err != nil || out.Len() != 0 {
			t.Fatalf("hook: err=%v stdout=%q stderr=%q", err, out.String(), errb.String())
		}
	}

	hook(`{"session_id":"abc","cwd":"/x","hook_event_name":"UserPromptSubmit","prompt":"hola"}`)
	e := waitEvent(t, evc, "thinking", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionActivity && e.Activity == "Thinking…"
	})
	if e.ActivitySource != "hook" || e.ActivityKind != "thinking" || e.TurnStartedAt == nil || e.TurnEndedAt != nil || e.LastEventAt == nil {
		t.Fatalf("activity event %+v", e)
	}
	// PreToolUse (doc sample): stdout stays empty — no permission decision.
	hook(`{"session_id":"abc123","transcript_path":"/t.jsonl","cwd":"/home/user/my-project","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"npm test","description":"Run test suite","timeout":120000,"run_in_background":false},"tool_use_id":"toolu_01ABC123"}`)
	waitEvent(t, evc, "bash activity", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionActivity && e.Activity == "Bash: npm test"
	})
	info := findSession(t, c, sess.ID)
	if info.Activity != "Bash: npm test" || info.ActivityKind != "bash" || info.ActivityTarget != "npm test" || info.ActivitySource != "hook" || info.ActivityAt == nil || info.State != "running" {
		t.Fatalf("session info %+v", info)
	}
	// Activity-only notify (no state).
	var out bytes.Buffer
	if err := notifycmd.Run([]string{"--activity", "Compilando"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, evc, "manual activity", func(e protocol.Event) bool { return isMine(e) && e.Activity == "Compilando" })

	hook(`{"session_id":"abc","cwd":"/x","hook_event_name":"Stop","last_assistant_message":"ok"}`)
	e = waitEvent(t, evc, "done", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionState && e.State == "idle"
	})
	if e.TurnStartedAt == nil || e.TurnEndedAt == nil || !e.TurnEndedAt.After(*e.TurnStartedAt) {
		t.Fatalf("turn timers on idle %+v", e)
	}
}

func TestOutputActivityProgressAndTail(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	writeProfiles(t, p.Profiles(), `
- id: build
  cmd: /bin/sh
  args: ["-c", "printf 'step 1\\n\\033[1mstep 2: compiling\\033[0m\\n⠋\\r⠙\\r'; printf '\\033]9;4;1;40\\007'; read a; printf '\\033]9;4;0\\007'; read b"]
`)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	evc := eventStream(t, c, "")
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "build"})
	if err != nil {
		t.Fatal(err)
	}
	isMine := func(e protocol.Event) bool { return e.SessionID == sess.ID }
	e := waitEvent(t, evc, "progress + output activity", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionActivity && e.Progress != nil && e.Activity == "step 2: compiling"
	})
	if e.ActivitySource != "output" || e.Progress.State != "normal" || e.Progress.Value != 40 || e.LastOutputAt == nil {
		t.Fatalf("activity event %+v", e)
	}
	time.Sleep(200 * time.Millisecond)
	info := findSession(t, c, sess.ID)
	if info.AttentionKind != "" || info.State == "waiting_input" {
		t.Fatalf("progress raised attention: %+v", info)
	}
	lines, err := c.Tail(ctx, sess.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"step 1", "step 2: compiling", "⠙"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("tail %q want %q", lines, want)
	}
	if lines, _ := c.Tail(ctx, sess.ID, 1); len(lines) != 1 {
		t.Fatalf("tail 1: %q", lines)
	}
	if _, err := c.Tail(ctx, "nope", 1); err == nil {
		t.Fatal("unknown session")
	}

	st, err := c.Attach(ctx, sess.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.Input([]byte("x\n"))
	waitEvent(t, evc, "progress cleared", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionActivity && e.Progress == nil
	})
}

func TestStallDetection(t *testing.T) {
	notes := make(chan daemon.Notification, 16)
	c, p, _ := startDaemonWith(t, func(n daemon.Notification) { notes <- n })
	ctx := ctxT(t)
	if err := os.WriteFile(p.Settings(), []byte("activity:\n  stall_seconds: 1\nui:\n  language: es\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	evc := eventStream(t, c, "") // no UI: the daemon notifies
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	isMine := func(e protocol.Event) bool { return e.SessionID == sess.ID }
	// Before any hook event there is no open turn: no stall.
	time.Sleep(1500 * time.Millisecond)
	if findSession(t, c, sess.ID).Stalled {
		t.Fatal("stalled without an open turn")
	}
	if _, err := c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: sess.ID, State: "running",
		Activity: "Reading go.mod", ActivityKind: "read", ActivityTarget: "go.mod"}); err != nil {
		t.Fatal(err)
	}
	e := waitEvent(t, evc, "stall", func(e protocol.Event) bool { return isMine(e) && e.StallRaised })
	if e.Type != protocol.EventSessionActivity || !e.Stalled || e.StalledAt == nil || e.State != "running" || e.AttentionKind != "" {
		t.Fatalf("stall event %+v", e)
	}
	select {
	case n := <-notes:
		if n.Kind != "stall" || n.Sound || !strings.HasPrefix(n.Body, "¿Trabado? Sin actividad hace") || !strings.HasSuffix(n.Body, " · Leyendo go.mod") {
			t.Fatalf("stall notification %+v", n)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no stall notification")
	}
	if info := findSession(t, c, sess.ID); !info.Stalled || info.Unread || info.AttentionKind != "" {
		t.Fatalf("stall must not be an attention: %+v", info)
	}
	// Any event clears it.
	if _, err := c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: sess.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, evc, "stall cleared", func(e protocol.Event) bool {
		return isMine(e) && e.Type == protocol.EventSessionActivity && !e.Stalled
	})
}

func TestSettingsSaveKeepsUnsentFieldsAndUnknownKeys(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	if err := os.WriteFile(p.Settings(), []byte("activity:\n  stall_seconds: 42\ntheme: dark\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An older client only knows about notify.
	var got settingsResult
	if err := c.Call(ctx, protocol.MethodSettingsSave, map[string]any{"notify": map[string]any{"enabled": true, "sound": false, "notifyOnDone": true}}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Activity.StallSeconds != 42 || got.Notify.Sound || !got.Notify.NotifyOnStall {
		t.Fatalf("saved %+v", got)
	}
	data, _ := os.ReadFile(p.Settings())
	if !strings.Contains(string(data), "theme: dark") || !strings.Contains(string(data), "stall_seconds: 42") {
		t.Fatalf("file:\n%s", data)
	}
}

type settingsResult struct {
	Notify struct {
		Sound, NotifyOnStall bool
	}
	Activity struct{ StallSeconds int }
}
