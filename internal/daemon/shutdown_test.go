package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// startLoggedDaemon is startDaemon with the log captured; done yields Run's
// result.
func startLoggedDaemon(t *testing.T) (*client.Client, paths.Paths, *syncBuffer, <-chan error) {
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
	logs := &syncBuffer{}
	srv, err := daemon.New(daemon.Config{
		Paths:      p,
		IdleAfter:  time.Hour,
		KillGrace:  time.Second,
		Logger:     slog.New(slog.NewTextHandler(logs, nil)),
		Exe:        "/opt/agenterm test/agenterm",
		Notifier:   func(daemon.Notification) {},
		SearchPath: hermeticPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		done <- srv.Run(ctx)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	c := client.New(p.Socket())
	deadline := time.Now().Add(3 * time.Second)
	for c.Ping(context.Background()) != nil {
		if time.Now().After(deadline) {
			t.Fatal("daemon did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c, p, logs, done
}

func TestShutdownRefusesLiveSessionsUnlessForced(t *testing.T) {
	c, _, logs, done := startLoggedDaemon(t)
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "cat"})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Shutdown(ctx, false)
	if err == nil || !client.IsRemote(err) {
		t.Fatalf("shutdown without force: %v", err)
	}
	want := "1 live session: " + sess.ID + " (cat); use --force to stop them"
	if err.Error() != want {
		t.Fatalf("error %q, want %q", err, want)
	}
	code, data, _ := apierr.Code(err)
	raw, _ := data.(json.RawMessage)
	if code != apierr.LiveSessions || !strings.Contains(string(raw), `"count":1`) ||
		!strings.Contains(string(raw), `"profileId":"cat"`) {
		t.Fatalf("code %q data %s", code, raw)
	}
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("daemon went down after a refused shutdown: %v", err)
	}
	if s := findSession(t, c, sess.ID); s.State == "exited" {
		t.Fatal("session killed by a refused shutdown")
	}
	pid := "peer_pid=" + strconv.Itoa(os.Getpid())
	if l := logs.String(); !strings.Contains(l, "daemon.shutdown refused") || !strings.Contains(l, pid) || !strings.Contains(l, "force=false") {
		t.Fatalf("refusal not logged with the peer:\n%s", l)
	}

	if err := c.Shutdown(ctx, true); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forced shutdown did not stop the daemon")
	}
	l := logs.String()
	i := strings.Index(l, "daemon shutting down")
	if i < 0 {
		t.Fatalf("no shutdown line:\n%s", l)
	}
	line, _, _ := strings.Cut(l[i:], "\n")
	for _, w := range []string{`reason="daemon.shutdown RPC"`, pid, "peer_cmd=", "force=true"} {
		if !strings.Contains(line, w) {
			t.Errorf("shutdown line lacks %q: %s", w, line)
		}
	}
}

func TestShutdownIgnoresExitedSessions(t *testing.T) {
	c, _, _, done := startLoggedDaemon(t)
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, c, sess.ID, "exited")
	if err := c.Shutdown(ctx, false); err != nil {
		t.Fatalf("only exited sessions: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Sessions must resolve their own daemon: AGENTERM_HOME (which clients
// prefer) is replaced by the daemon's, even if the daemon inherited another.
func TestSessionEnvNamesItsDaemon(t *testing.T) {
	t.Setenv(paths.EnvHome, "/nonexistent/other-home")
	c, p, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	writeProfiles(t, p.Profiles(), `
- id: env
  cmd: /bin/sh
  args: ["-c", "echo \"HOME=$AGENTERM_HOME SOCK=$AGENTERM_SOCK END\"; sleep 5"]
`)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "env"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, sess.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	out := readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte("END")) })
	want := "HOME=" + p.RuntimeDir + " SOCK=" + p.Socket() + " END"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("session env %q, want %q", out, want)
	}
}
