package session

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/status"
)

func spawn(t *testing.T, o Options) *Session {
	t.Helper()
	if o.ID == "" {
		o.ID = "test"
	}
	if o.Cwd == "" {
		o.Cwd = t.TempDir()
	}
	s, err := Spawn(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Kill(time.Second) })
	return s
}

func waitDone(t *testing.T, s *Session) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}
}

func TestSpawnEnvAndExitCode(t *testing.T) {
	s := spawn(t, Options{ID: "abc", Cmd: "/bin/sh", SockPath: "/tmp/x.sock",
		Args: []string{"-c", `echo "id=$AGENTERM_SESSION sock=$AGENTERM_SOCK foo=$FOO"; pwd; exit 7`},
		Env:  []string{"FOO=bar"}})
	waitDone(t, s)
	out := string(s.Scrollback())
	if !strings.Contains(out, "id=abc sock=/tmp/x.sock foo=bar") {
		t.Fatalf("env not injected: %q", out)
	}
	if !strings.Contains(out, s.Cwd) && !strings.Contains(out, "/private"+s.Cwd) {
		t.Fatalf("cwd not applied: %q (want %s)", out, s.Cwd)
	}
	code, exited := s.ExitCode()
	if !exited || code != 7 || s.State() != status.Exited {
		t.Fatalf("code=%d exited=%v state=%s", code, exited, s.State())
	}
	if _, err := s.Write([]byte("x")); err == nil {
		t.Fatal("write after exit should fail")
	}
}

func TestAttachLiveAndResize(t *testing.T) {
	s := spawn(t, Options{Cmd: "/bin/cat"})
	snap, sub := s.Attach()
	defer sub.Close()
	if len(snap) != 0 {
		t.Fatalf("unexpected snapshot %q", snap)
	}
	if err := s.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
	if c, r := s.Size(); c != 100 || r != 40 {
		t.Fatalf("size %dx%d", c, r)
	}
	s.Write([]byte("ping\n"))
	var got bytes.Buffer
	deadline := time.After(3 * time.Second)
	for strings.Count(got.String(), "ping") < 2 {
		select {
		case c := <-sub.C:
			got.Write(c)
		case <-deadline:
			t.Fatalf("timeout, got %q", got.String())
		}
	}
	s.Kill(time.Second)
	if _, ok := <-sub.C; ok {
		// drain remaining
		for range sub.C {
		}
	}
	if sub.Dropped() {
		t.Fatal("subscriber should not be marked dropped on exit")
	}
	// Attaching after exit still yields scrollback and a closed channel.
	snap, sub2 := s.Attach()
	if !bytes.Contains(snap, []byte("ping")) {
		t.Fatalf("scrollback missing: %q", snap)
	}
	if _, ok := <-sub2.C; ok {
		t.Fatal("expected closed channel")
	}
}

func TestSlowSubscriberDropped(t *testing.T) {
	s := spawn(t, Options{Cmd: "/bin/sh", Args: []string{"-c", "yes | head -c 2000000"}, RingSize: 1024})
	_, slow := s.Attach() // never read
	waitDone(t, s)
	for range slow.C {
	}
	if !slow.Dropped() {
		t.Fatal("slow subscriber should have been dropped")
	}
	if got := len(s.Scrollback()); got != 1024 {
		t.Fatalf("ring len = %d", got)
	}
}

func TestKillInteractiveShell(t *testing.T) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	s := spawn(t, Options{Cmd: sh, Args: []string{"-i"}})
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	s.Kill(2 * time.Second)
	if s.State() != status.Exited {
		t.Fatalf("state %s", s.State())
	}
	t.Logf("killed in %s", time.Since(start))
}
