package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
)

type fakeDesktop struct {
	mu        sync.Mutex
	active    bool
	notes     []note
	badge     int
	withdrawn []string
	bounces   int
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
	var daemonNotes sync.Map
	srv, err := daemon.New(daemon.Config{Paths: p, IdleAfter: time.Hour, KillGrace: time.Second,
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
	a.SetVisibleSession(id)
	a.client.Notify(ctx, id, "running")
	a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "idle", Message: "Listo"})
	waitFor(t, "done badge", func() bool { _, b, _ := fd.snapshot(); return b == 1 })
	time.Sleep(100 * time.Millisecond)
	if n, _, _ := fd.snapshot(); len(n) != 1 {
		t.Fatalf("notified about the visible session: %+v", n)
	}
	// Another session visible: notify, silently for "done".
	a.SetVisibleSession("other")
	a.client.Notify(ctx, id, "running")
	a.client.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: "idle", Message: "Listo"})
	waitFor(t, "done note", func() bool { n, _, _ := fd.snapshot(); return len(n) == 2 })
	if n, _, _ := fd.snapshot(); n[1].Body != "Listo" || n[1].Sound || n[1].Kind != "done" {
		t.Fatalf("bad done note %+v", n[1])
	}
}
