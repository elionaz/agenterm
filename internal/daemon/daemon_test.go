package daemon_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
)

const testProfiles = `
- id: hello
  cmd: /bin/sh
  args: ["-c", "echo hello; sleep 0.2"]
- id: cat
  cmd: /bin/cat
  status: hooks
- id: shell
  cmd: /bin/sh
  idle_seconds: 0.2
`

// hermeticPath keeps detection off the login shell (and the user's rc files).
func hermeticPath() string { return "/usr/bin:/bin" }

// startDaemon runs a daemon in-process on a short temp dir (unix socket paths
// are limited to ~104 bytes on macOS, so t.TempDir() can be too long).
func startDaemon(t *testing.T) (*client.Client, paths.Paths, func()) {
	t.Helper()
	return startDaemonWith(t, func(daemon.Notification) {})
}

// startDaemonWith is startDaemon with a fallback notifier (tests must never
// post real desktop notifications).
func startDaemonWith(t *testing.T, notifier func(daemon.Notification)) (*client.Client, paths.Paths, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "agt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	if err := os.WriteFile(p.Profiles(), []byte(testProfiles), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := daemon.New(daemon.Config{
		Paths:      p,
		IdleAfter:  time.Hour, // keep the heuristic out of the way unless a profile overrides it
		KillGrace:  time.Second,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Exe:        "/opt/agenterm test/agenterm",
		Notifier:   notifier,
		SearchPath: hermeticPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	c := client.New(p.Socket())
	deadline := time.Now().Add(3 * time.Second)
	for c.Ping(context.Background()) != nil {
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var stopped bool
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	}
	t.Cleanup(stop)
	return c, p, stop
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// readUntil collects output frames until pred holds on the accumulated bytes
// or a terminal frame arrives.
func readUntil(t *testing.T, st *client.Stream, pred func(out []byte, f protocol.Frame) bool) []byte {
	t.Helper()
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		for {
			f, err := st.Recv()
			if err != nil {
				done <- err
				return
			}
			if f.Type == protocol.FrameOutput {
				out.Write(f.Payload)
			}
			if pred(out.Bytes(), f) {
				done <- nil
				return
			}
		}
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stream ended: %v (output %q)", err, out.String())
		}
	case <-time.After(5 * time.Second):
		st.Close()
		<-done
		t.Fatalf("timeout; output so far %q", out.String())
	}
	return out.Bytes()
}

func findSession(t *testing.T, c *client.Client, id string) protocol.SessionInfo {
	t.Helper()
	list, err := c.Sessions(ctxT(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not listed", id)
	return protocol.SessionInfo{}
}

func waitState(t *testing.T, c *client.Client, id, state string) protocol.SessionInfo {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := findSession(t, c, id)
		if s.State == state {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s: state %s, want %s", id, s.State, state)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDaemonEndToEnd(t *testing.T) {
	c, p, stop := startDaemon(t)
	ctx := ctxT(t)

	if fi, err := os.Stat(p.Socket()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("socket perms: %v %v", fi, err)
	}
	if _, err := os.Stat(p.PidFile()); err != nil {
		t.Fatalf("pid file: %v", err)
	}

	// Projects.
	projDir := t.TempDir()
	proj, err := c.ProjectAdd(ctx, projDir, "")
	if err != nil {
		t.Fatal(err)
	}
	projs, err := c.ProjectList(ctx)
	if err != nil || len(projs) != 1 || projs[0].ID != proj.ID {
		t.Fatalf("project list: %+v %v", projs, err)
	}
	profs, err := c.Profiles(ctx)
	if err != nil || len(profs) < 3 || profs[0].ID != "hello" || profs[1].ID != "cat" || profs[2].ID != "shell" {
		t.Fatalf("profiles: %+v %v", profs, err)
	}

	events, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	evc := make(chan protocol.Event, 64)
	go func() {
		for {
			ev, err := events.Next()
			if err != nil {
				close(evc)
				return
			}
			evc <- ev
		}
	}()

	// 1. Short-lived command: output, then exit 0.
	hello, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hello", Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	if hello.Cwd != projDir || hello.Cols != 100 {
		t.Fatalf("spawn info: %+v", hello)
	}
	st, err := c.Attach(ctx, hello.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var exitCode = -99
	out := readUntil(t, st, func(_ []byte, f protocol.Frame) bool {
		if f.Type == protocol.FrameExit {
			exitCode, _ = protocol.ParseExit(f.Payload)
			return true
		}
		return false
	})
	st.Close()
	if !bytes.Contains(out, []byte("hello")) || exitCode != 0 {
		t.Fatalf("hello: out=%q code=%d", out, exitCode)
	}
	if info := waitState(t, c, hello.ID, "exited"); info.ExitCode == nil || *info.ExitCode != 0 {
		t.Fatalf("hello exit: %+v", info)
	}

	// 2. Interactive cat: attach, type, read echo, detach, reattach.
	cat, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.Name, ProfileID: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	st, err = c.Attach(ctx, cat.ID, 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	if st.Info.Cols != 120 || st.Info.Rows != 40 {
		t.Fatalf("attach resize not applied: %+v", st.Info)
	}
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	if err := st.Input([]byte("ping-123\n")); err != nil {
		t.Fatal(err)
	}
	// tty echo + cat's own output => the token appears twice.
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Count(out, []byte("ping-123")) >= 2 })
	if err := st.Resize(90, 20); err != nil {
		t.Fatal(err)
	}
	st.Close() // detach

	if info := findSession(t, c, cat.ID); info.State == "exited" {
		t.Fatal("detach must not kill the session")
	}
	deadline := time.Now().Add(2 * time.Second)
	for info := findSession(t, c, cat.ID); info.Cols != 90; info = findSession(t, c, cat.ID) {
		if time.Now().After(deadline) {
			t.Fatalf("resize frame not applied: %+v", info)
		}
		time.Sleep(10 * time.Millisecond)
	}

	st, err = c.Attach(ctx, cat.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	replay := readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	if bytes.Count(replay, []byte("ping-123")) < 2 {
		t.Fatalf("scrollback missing on reattach: %q", replay)
	}
	st.Input([]byte("again\n"))
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Count(out, []byte("again")) >= 2 })

	// 3. notify changes state; output no longer overrides (hooks profile).
	if _, err := c.Notify(ctx, cat.ID, "waiting"); err != nil {
		t.Fatal(err)
	}
	st.Input([]byte("more\n"))
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Count(out, []byte("more")) >= 2 })
	if info := findSession(t, c, cat.ID); info.State != "waiting_input" {
		t.Fatalf("state after notify = %s", info.State)
	}
	if _, err := c.Notify(ctx, cat.ID, "bogus"); err == nil || !client.IsRemote(err) {
		t.Fatalf("bad state should be a remote error, got %v", err)
	}
	if _, err := c.Notify(ctx, "nope", "idle"); err == nil {
		t.Fatal("unknown session should fail")
	}

	// 4. kill -> exited; the attached stream gets an exit frame.
	killed, err := c.Kill(ctx, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if killed.State != "exited" || killed.ExitCode == nil {
		t.Fatalf("kill: %+v", killed)
	}
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameExit })
	st.Close()

	// Killing an exited session forgets it.
	if _, err := c.Kill(ctx, cat.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := c.Sessions(ctx)
	for _, s := range list {
		if s.ID == cat.ID {
			t.Fatal("exited session not removed")
		}
	}

	// Events observed along the way.
	want := map[string]bool{
		"spawned:" + cat.ID:                  false,
		"state:" + cat.ID + ":waiting_input": false,
		"state:" + cat.ID + ":exited":        false,
		"state:" + hello.ID + ":exited":      false,
		"removed:" + cat.ID:                  false,
	}
	timeout := time.After(3 * time.Second)
	for remaining := len(want); remaining > 0; {
		select {
		case ev, ok := <-evc:
			if !ok {
				t.Fatal("event stream closed early")
			}
			var key string
			switch ev.Type {
			case protocol.EventSessionSpawned:
				key = "spawned:" + ev.SessionID
			case protocol.EventSessionState:
				key = "state:" + ev.SessionID + ":" + ev.State
			case protocol.EventSessionRemoved:
				key = "removed:" + ev.SessionID
			}
			if seen, ok := want[key]; ok && !seen {
				want[key] = true
				remaining--
			}
		case <-timeout:
			t.Fatalf("missing events: %v", want)
		}
	}

	// 5. Shutdown kills remaining sessions and cleans up.
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, c, sh.ID, "idle") // heuristic with idle_seconds: 0.2
	stop()
	if _, err := os.Stat(p.Socket()); !os.IsNotExist(err) {
		t.Fatalf("socket not removed: %v", err)
	}
	if _, err := os.Stat(p.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pid file not removed: %v", err)
	}
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("daemon still answering")
	}
}

func TestDaemonRejectsSecondInstanceAndStaleSocket(t *testing.T) {
	_, p, stop := startDaemon(t)
	srv2, err := daemon.New(daemon.Config{Paths: p, SearchPath: hermeticPath, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv2.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("want already running, got %v", err)
	}
	stop()

	// A leftover socket file with nobody listening is stale and gets replaced.
	if err := os.WriteFile(p.Socket(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	srv3, _ := daemon.New(daemon.Config{Paths: p, SearchPath: hermeticPath, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv3.Run(ctx) }()
	c := client.New(p.Socket())
	deadline := time.Now().Add(3 * time.Second)
	for c.Ping(context.Background()) != nil {
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start over stale socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = cancel
	if _, err := os.Stat(filepath.Join(p.RuntimeDir, "d.sock")); !os.IsNotExist(err) {
		t.Fatal("socket not cleaned up")
	}
}

func TestUnknownMethodAndBadParams(t *testing.T) {
	c, _, _ := startDaemon(t)
	ctx := ctxT(t)
	if err := c.Call(ctx, "nope", nil, nil); err == nil || !client.IsRemote(err) {
		t.Fatalf("got %v", err)
	}
	if _, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: "x", ProfileID: "cat"}); err == nil {
		t.Fatal("spawn with unknown project should fail")
	}
	if _, err := c.Attach(ctx, "nope", 0, 0); err == nil {
		t.Fatal("attach unknown session should fail")
	}
}
