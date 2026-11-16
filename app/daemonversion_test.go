package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/version"
)

func TestDaemonMismatch(t *testing.T) {
	for _, c := range []struct {
		app, daemon string
		want        bool
	}{
		{"0.2.0", "0.2.0", false},
		{"0.2.0", "", true}, // 0.1.x daemons do not report a version
		{"0.2.0", "0.1.1", true},
		{"0.2.1", "0.2.0", true},
		{"0.10.0", "0.9.0", true},
		{"0.2.0", "0.2.0-rc.1", true},
		{"0.2.0", "0.3.0", false},      // newer daemon: never downgrade it
		{"0.2.0-rc.1", "0.2.0", false}, // ditto
		{"0.2.0", "garbage", true},     // unknown: offer the restart
		{"dev", "0.1.0", false},        // development builds never ask
		{"dev", "dev", false},
	} {
		if got := daemonMismatch(c.app, c.daemon); got != c.want {
			t.Errorf("daemonMismatch(%q, %q) = %v, want %v", c.app, c.daemon, got, c.want)
		}
	}
}

// restartHarness is an App whose "daemon" runs in-process (the test
// binary cannot be one), with sessions.restore set to policy.
type restartHarness struct {
	a    *App
	dir  string
	mu   sync.Mutex
	runs []chan error
}

func newRestartHarness(t *testing.T, policy string) *restartHarness {
	oldV := version.Version
	t.Cleanup(func() { version.Version = oldV })
	version.Version = "9.9.9-test"

	dir, err := os.MkdirTemp("/tmp", "agrst") // short: unix socket path limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	if err := os.WriteFile(p.Profiles(), []byte("- id: sh\n  cmd: /bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Settings(), []byte("sessions:\n  restore: "+policy+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := &restartHarness{dir: dir}
	var cancels []context.CancelFunc
	launch := func() (chan error, error) {
		srv, err := daemon.New(daemon.Config{Paths: p, SearchPath: func() string { return "/usr/bin:/bin" },
			IdleAfter: time.Hour, KillGrace: time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- srv.Run(ctx) }()
		h.mu.Lock()
		h.runs, cancels = append(h.runs, done), append(cancels, cancel)
		h.mu.Unlock()
		return done, nil
	}
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, c := range cancels {
			c()
			<-h.runs[i]
		}
	})

	var rec recorder
	a := NewApp()
	a.emit = rec.emit
	a.launch = func(ctx context.Context) error {
		done, err := launch()
		if err != nil {
			return err
		}
		// Also called from the event loop's goroutine: no t.Fatal here.
		for a.client.Ping(ctx) != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err := <-done:
				done <- err // for the cleanup
				return fmt.Errorf("daemon exited: %v", err)
			case <-time.After(10 * time.Millisecond):
			}
		}
		return nil
	}
	a.start(context.Background(), p)
	t.Cleanup(func() { a.shutdown(context.Background()) })
	waitFor(t, "resync", func() bool { return rec.has(evResync) })
	h.a = a
	return h
}

// daemons returns how many daemons were started and the first one's exit.
func (h *restartHarness) daemons() (int, chan error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.runs), h.runs[0]
}

func (h *restartHarness) spawn(t *testing.T) protocol.SessionInfo {
	t.Helper()
	ps, err := h.a.client.ProjectList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var pid string
	if len(ps) > 0 {
		pid = ps[0].ID
	} else {
		proj, err := h.a.client.ProjectAdd(context.Background(), h.dir, "t")
		if err != nil {
			t.Fatal(err)
		}
		pid = proj.ID
	}
	info, err := h.a.Spawn(pid, "sh", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func states(t *testing.T, a *App) map[string]string {
	t.Helper()
	ss, err := a.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, s := range ss {
		out[s.ID] = s.State
	}
	return out
}

// TestRestartDaemon: "Restart now" stops the daemon (live sessions are
// suspended), waits for it to exit, starts a new one and resumes exactly
// the sessions that were live — whatever sessions.restore says.
func TestRestartDaemon(t *testing.T) {
	for _, policy := range []string{"ask", "always", "never"} {
		t.Run(policy, func(t *testing.T) {
			h := newRestartHarness(t, policy)
			a := h.a
			st, err := a.DaemonStatus()
			if err != nil || st.Mismatch || st.DaemonVersion != "9.9.9-test" || st.AppVersion != "9.9.9-test" {
				t.Fatalf("DaemonStatus = %+v, %v", st, err)
			}
			info := h.spawn(t)

			b, err := a.RestartDaemon()
			if err != nil {
				t.Fatal(err)
			}
			if len(b.Sessions) != 1 || b.Sessions[0].ID != info.ID || b.Sessions[0].State == "suspended" || len(b.Failed) != 0 {
				t.Fatalf("RestartDaemon = %+v", b)
			}
			n, first := h.daemons()
			if n != 2 {
				t.Fatalf("%d daemons started, want 2", n)
			}
			select { // replaceDaemon waited for it: it has already returned
			case err := <-first:
				if err != nil {
					t.Fatalf("old daemon: %v", err)
				}
				first <- nil // for the cleanup
			case <-time.After(5 * time.Second):
				t.Fatal("the old daemon is still running")
			}
			if s := states(t, a); len(s) != 1 || s[info.ID] == "suspended" || s[info.ID] == "" {
				t.Fatalf("after restart: %v", s)
			}
			if _, err := os.Stat(paths.In(h.dir).RestartFile()); !os.IsNotExist(err) {
				t.Fatalf("restart marker left behind: %v", err)
			}
		})
	}
}

// TestRestartDaemonKeepsSuspended: sessions the user had left suspended
// are not resumed by a restart.
func TestRestartDaemonKeepsSuspended(t *testing.T) {
	h := newRestartHarness(t, "ask")
	a := h.a
	old := h.spawn(t)
	// The daemon goes away on its own (not "Restart now"): old is
	// suspended and the user does not resume it.
	h.mu.Lock()
	first := h.runs[0]
	h.mu.Unlock()
	if err := a.client.Shutdown(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	first <- nil // for the cleanup
	waitFor(t, "suspended", func() bool { return states(t, a)[old.ID] == "suspended" })

	live := h.spawn(t)
	b, err := a.RestartDaemon()
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Sessions) != 1 || b.Sessions[0].ID != live.ID || len(b.Failed) != 0 {
		t.Fatalf("RestartDaemon = %+v", b)
	}
	s := states(t, a)
	if s[old.ID] != "suspended" || s[live.ID] == "suspended" || s[live.ID] == "" {
		t.Fatalf("after restart: %v", s)
	}
}
