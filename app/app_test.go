package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/settings"
)

// TestMain: whatever a test wires up (a real desktop, a daemon's default
// notifier), it never posts real desktop notifications.
type recorder struct {
	mu  sync.Mutex
	evs []recorded
}

type recorded struct {
	name string
	data []any
}

func (r *recorder) emit(name string, data ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, recorded{name, data})
}

// output concatenates the decoded pty:<id> payloads.
func (r *recorder) output(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var sb strings.Builder
	for _, e := range r.evs {
		if e.name == evPty+":"+id {
			b, _ := base64.StdEncoding.DecodeString(e.data[0].(string))
			sb.Write(b)
		}
	}
	return sb.String()
}

func (r *recorder) has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.evs {
		if e.name == name {
			return true
		}
	}
	return false
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestAppAgainstDaemon drives the bound methods against an in-process daemon
// and checks the event stream the frontend would see.
func TestAppAgainstDaemon(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "agapp") // short: unix socket path limit
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	if err := os.WriteFile(p.Profiles(), []byte("- id: sh\n  cmd: /bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := daemon.New(daemon.Config{Paths: p, SearchPath: func() string { return "/usr/bin:/bin" }, IdleAfter: time.Hour, KillGrace: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Notifier: func(daemon.Notification) {}})
	if err != nil {
		t.Fatal(err)
	}
	dctx, stopDaemon := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(dctx) }()
	t.Cleanup(func() {
		stopDaemon()
		<-done
	})

	var rec recorder
	a := NewApp()
	a.emit = rec.emit
	a.start(context.Background(), p)
	t.Cleanup(func() { a.shutdown(context.Background()) })

	waitFor(t, "resync", func() bool { return rec.has(evResync) })

	proj, err := a.client.ProjectAdd(context.Background(), dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := a.ListProjects()
	if err != nil || len(ps) != 1 || ps[0].ID != proj.ID {
		t.Fatalf("ListProjects = %v, %v", ps, err)
	}
	if ss, err := a.ListSessions(); err != nil || ss == nil || len(ss) != 0 {
		t.Fatalf("ListSessions = %#v, %v (want empty, non-nil)", ss, err)
	}

	info, err := a.Spawn(proj.ID, "sh", 90, 20)
	if err != nil {
		t.Fatal(err)
	}
	id := info.ID
	waitFor(t, "spawned event", func() bool { return rec.has(evDaemon) })

	if err := a.Input(id, "x"); err == nil {
		t.Fatal("Input before Attach should fail")
	}
	if _, err := a.Attach(id, 100, 30); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "replay done", func() bool { return rec.has(evReplayDone + ":" + id) })

	if err := a.Input(id, "echo agent-$((40+2))\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "echo output", func() bool { return strings.Contains(rec.output(id), "agent-42") })

	if err := a.Resize(id, 120, 40); err != nil {
		t.Fatal(err)
	}
	if err := a.Input(id, "stty size\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "new size", func() bool { return strings.Contains(rec.output(id), "40 120") })

	// Reattach replays the scrollback (what a reloaded frontend relies on)
	// and the replaced stream must not report a spurious error.
	before := strings.Count(rec.output(id), "agent-42")
	if _, err := a.Attach(id, 0, 0); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "replayed scrollback", func() bool { return strings.Count(rec.output(id), "agent-42") > before })
	if rec.has(evError + ":" + id) {
		t.Fatal("replacing a stream emitted pty-error")
	}

	// Each Settings tab saves only its own section: the others keep their
	// values (they used to be overwritten with zero values).
	if _, err := a.SaveSettings(SettingsPatch{Activity: &settings.Activity{StallSeconds: 7}, Notify: &settings.Notify{Enabled: true, Sound: true, OnDone: true, OnStall: true, Fallback: settings.FallbackOsascript}}); err != nil {
		t.Fatal(err)
	}
	// A notify section without fallback (an older frontend) keeps it.
	if st, err := a.SaveSettings(SettingsPatch{Notify: &settings.Notify{Enabled: true, Sound: true, OnDone: true, OnStall: true}}); err != nil || st.Notify.Fallback != settings.FallbackOsascript {
		t.Fatalf("notify without fallback: %+v %v", st.Notify, err)
	}
	if _, err := a.SaveSettings(SettingsPatch{Git: &settings.Git{Editor: "zed", DefaultMode: "worktree"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveSettings(SettingsPatch{Sessions: &settings.Sessions{Restore: settings.RestoreNever}}); err != nil {
		t.Fatal(err)
	}
	term := settings.Terminal{FontFamily: "Menlo", FontSize: 15, LineHeight: 1.3, CursorStyle: "bar", Theme: "funeral"}
	if _, err := a.SaveSettings(SettingsPatch{Terminal: &term}); err != nil {
		t.Fatal(err)
	}
	// The Appearance tab saves ui.appearance alone: ui.language stays.
	if _, err := a.SaveSettings(SettingsPatch{UI: &settings.UI{Language: "es"}}); err != nil {
		t.Fatal(err)
	}
	st, err := a.SaveSettings(SettingsPatch{UI: &settings.UI{Appearance: "light"}})
	if err != nil {
		t.Fatal(err)
	}
	if st.UI.Language != "es" || st.UI.Appearance != "light" || st.Git.Editor != "zed" || st.Git.DefaultMode != "worktree" ||
		st.Sessions.Restore != settings.RestoreNever || st.Terminal != term ||
		st.Activity.StallSeconds != 7 || !st.Notify.Sound || !st.Notify.OnDone || !st.Notify.OnStall || st.Notify.Fallback != settings.FallbackOsascript {
		t.Fatalf("partial saves clobbered other sections: %+v", st)
	}
	// And the language alone keeps the appearance.
	if st, err = a.SaveSettings(SettingsPatch{UI: &settings.UI{Language: "en"}}); err != nil || st.UI.Appearance != "light" || st.UI.Language != "en" {
		t.Fatalf("language save: %+v %v", st.UI, err)
	}

	if _, err := a.Kill(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "exit event", func() bool { return rec.has(evExit + ":" + id) })

	ss, err := a.ListSessions()
	if err != nil || len(ss) != 1 || ss[0].State != "exited" {
		t.Fatalf("after kill: %v, %v", ss, err)
	}
}
