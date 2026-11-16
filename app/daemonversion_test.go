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
		{"dev", "0.1.0", false}, // development builds never ask
		{"dev", "dev", false},
	} {
		if got := daemonMismatch(c.app, c.daemon); got != c.want {
			t.Errorf("daemonMismatch(%q, %q) = %v, want %v", c.app, c.daemon, got, c.want)
		}
	}
}

// TestRestartDaemon: the "Restart now" flow stops the daemon (sessions are
// suspended), starts a new one and resumes them under sessions.restore ask.
func TestRestartDaemon(t *testing.T) {
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

	// In-process daemons stand in for `agenterm daemon` (the test binary
	// cannot be one).
	var mu sync.Mutex
	var runs []chan error
	var cancels []context.CancelFunc
	launch := func(context.Context) error {
		srv, err := daemon.New(daemon.Config{Paths: p, SearchPath: func() string { return "/usr/bin:/bin" },
			IdleAfter: time.Hour, KillGrace: time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- srv.Run(ctx) }()
		mu.Lock()
		runs, cancels = append(runs, done), append(cancels, cancel)
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for i, c := range cancels {
			c()
			<-runs[i]
		}
	})

	var rec recorder
	a := NewApp()
	a.emit = rec.emit
	a.launch = func(ctx context.Context) error {
		if err := launch(ctx); err != nil {
			return err
		}
		// Also called from the event loop's goroutine: no t.Fatal here.
		mu.Lock()
		done := runs[len(runs)-1]
		mu.Unlock()
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

	st, err := a.DaemonStatus()
	if err != nil || st.Mismatch || st.DaemonVersion != "9.9.9-test" || st.AppVersion != "9.9.9-test" {
		t.Fatalf("DaemonStatus = %+v, %v", st, err)
	}

	proj, err := a.client.ProjectAdd(context.Background(), dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	info, err := a.Spawn(proj.ID, "sh", 80, 24)
	if err != nil {
		t.Fatal(err)
	}

	b, err := a.RestartDaemon()
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Sessions) != 1 || b.Sessions[0].ID != info.ID || b.Sessions[0].State == "suspended" || len(b.Failed) != 0 {
		t.Fatalf("RestartDaemon = %+v", b)
	}
	mu.Lock()
	n := len(runs)
	first := runs[0]
	mu.Unlock()
	if n != 2 {
		t.Fatalf("%d daemons started, want 2", n)
	}
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("old daemon: %v", err)
		}
		first <- nil // for the cleanup
	default:
		t.Fatal("the old daemon is still running")
	}
	ss, err := a.ListSessions()
	if err != nil || len(ss) != 1 || ss[0].ID != info.ID || ss[0].State == "suspended" {
		t.Fatalf("after restart: %+v, %v", ss, err)
	}
}
