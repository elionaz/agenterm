package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/macnotify"
	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/protocol"
)

type fakeDesktop struct {
	mu        sync.Mutex
	active    bool
	notes     []note
	badge     int
	withdrawn []string
	bounces   int
	reason    string // what Refresh returns
	postErr   error  // what Post fails with
}

func (f *fakeDesktop) Notify(n note) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, n)
	return nil
}
func (f *fakeDesktop) Withdraw(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.withdrawn = append(f.withdrawn, id)
}
func (f *fakeDesktop) SetBadge(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.badge = n
}
func (f *fakeDesktop) RequestAttention() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bounces++
}
func (f *fakeDesktop) IsActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}
func (f *fakeDesktop) Activate() {}
func (f *fakeDesktop) Refresh() (macnotify.Status, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reason != "" {
		return macnotify.StatusDenied, f.reason
	}
	return macnotify.StatusAuthorized, ""
}

// Post fails like the real one: denied without the fallback, or a post error.
func (f *fakeDesktop) Post(n note) error {
	f.mu.Lock()
	reason, perr := f.reason, f.postErr
	f.mu.Unlock()
	if reason != "" && !n.Osascript {
		return errors.New(reason)
	}
	if perr != nil {
		return perr
	}
	return f.Notify(n)
}

func (f *fakeDesktop) snapshot() (notes []note, badge, bounces int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]note(nil), f.notes...), f.badge, f.bounces
}

func TestAppAttentionNotifications(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "agatt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	os.WriteFile(p.Profiles(), []byte("- id: cat\n  cmd: /bin/cat\n  status: hooks\n"), 0o600)
	os.WriteFile(p.Settings(), []byte("ui:\n  language: es\n"), 0o600) // notification texts
	var daemonNotes sync.Map
	srv, err := daemon.New(daemon.Config{Paths: p, SearchPath: func() string { return "/usr/bin:/bin" }, IdleAfter: time.Hour, KillGrace: time.Second,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Notifier: func(n daemon.Notification) { daemonNotes.Store(n.SessionID, n) }})
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
	fd := &fakeDesktop{}
	a := NewApp()
	a.emit = rec.emit
	a.desk = fd
	a.start(context.Background(), p)
	t.Cleanup(func() { a.shutdown(context.Background()) })
	waitFor(t, "resync", func() bool { return rec.has(evResync) })

	ctx := context.Background()
	proj, _ := a.client.ProjectAdd(ctx, dir, "web")
	info, err := a.Spawn(proj.ID, "cat", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	id := info.ID

	// App in the background: notify, bounce for permission, badge 1.
	if _, err := a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "waiting", Kind: "permission"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "notification", func() bool { n, b, _ := fd.snapshot(); return len(n) == 1 && b == 1 })
	notes, _, bounces := fd.snapshot()
	if n := notes[0]; n.Title != "web · cat" || n.Body != "Necesita tu aprobación" || !n.Sound || n.SessionID != id || bounces != 1 {
		t.Fatalf("bad note %+v bounces=%d", n, bounces)
	}
	if _, ok := daemonNotes.Load(id); ok {
		t.Fatal("daemon notified although the app is subscribed")
	}

	// Ack clears the badge and withdraws the notification.
	if err := a.Ack(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "badge cleared", func() bool { _, b, _ := fd.snapshot(); return b == 0 })

	// Frontmost and showing that session: no notification (the UI acks).
	fd.mu.Lock()
	fd.active = true
	fd.mu.Unlock()
	a.SetVisibleSessions([]string{"other", id}) // one of several split panes
	a.client.Notify(ctx, id, "running")
	a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "idle", Message: "Listo"})
	waitFor(t, "done badge", func() bool { _, b, _ := fd.snapshot(); return b == 1 })
	time.Sleep(100 * time.Millisecond)
	if n, _, _ := fd.snapshot(); len(n) != 1 {
		t.Fatalf("notified about the visible session: %+v", n)
	}
	// Another session visible: notify, silently for "done".
	a.SetVisibleSessions([]string{"other"})
	a.client.Notify(ctx, id, "running")
	a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "idle", Message: "Listo"})
	waitFor(t, "done note", func() bool { n, _, _ := fd.snapshot(); return len(n) == 2 })
	if n, _, _ := fd.snapshot(); n[1].Body != "Listo" || n[1].Sound || n[1].Kind != "done" {
		t.Fatalf("bad done note %+v", n[1])
	}
	// A renamed session is titled "<project> · <name>".
	if _, err := a.Rename(id, "api"); err != nil {
		t.Fatal(err)
	}
	a.client.Notify(ctx, id, "running")
	a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "idle", Message: "Listo"})
	waitFor(t, "renamed note", func() bool { n, _, _ := fd.snapshot(); return len(n) == 3 })
	if n, _, _ := fd.snapshot(); n[2].Title != "web · api" {
		t.Fatalf("renamed note title %q", n[2].Title)
	}
}

func TestAppStallNotificationAndTail(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "agstl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	os.WriteFile(p.Profiles(), []byte("- id: cat\n  cmd: /bin/cat\n  status: hooks\n"), 0o600)
	os.WriteFile(p.Settings(), []byte("ui:\n  language: es\n"), 0o600) // notification texts
	os.WriteFile(p.Settings(), []byte("activity:\n  stall_seconds: 1\n"), 0o600)
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
	fd := &fakeDesktop{}
	a := NewApp()
	a.emit = rec.emit
	a.desk = fd
	a.start(context.Background(), p)
	t.Cleanup(func() { a.shutdown(context.Background()) })
	waitFor(t, "resync", func() bool { return rec.has(evResync) })

	ctx := context.Background()
	proj, _ := a.client.ProjectAdd(ctx, dir, "web")
	info, err := a.Spawn(proj.ID, "cat", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: info.ID, State: "running", Activity: "Leyendo go.mod"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stall note", func() bool { n, _, _ := fd.snapshot(); return len(n) == 1 })
	notes, badge, bounces := fd.snapshot()
	if n := notes[0]; n.Kind != "stall" || n.Sound || badge != 0 || bounces != 0 || n.Body == "" {
		t.Fatalf("stall note %+v badge=%d bounces=%d", n, badge, bounces)
	}
	lines, err := a.Tail(info.ID, 8)
	if err != nil || lines == nil {
		t.Fatalf("tail %v %v", lines, err)
	}
}
