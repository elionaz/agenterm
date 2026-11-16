package daemon_test

import (
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/protocol"
)

// session.rename: the name is normalized, announced, persisted in
// sessions.json and survives a daemon restart and a resume; an empty name
// resets it.
func TestSessionRenamePersists(t *testing.T) {
	c, p, stop := startDaemon(t)
	proj, err := c.ProjectAdd(ctxT(t), t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	s := spawnWith(t, c, proj.ID, "shell", "", "")
	events := eventStream(t, c, "")

	info, err := c.Rename(ctxT(t), s.ID, "  api\tserver  ")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "api server" {
		t.Fatalf("rename result name %q", info.Name)
	}
	ev := waitEvent(t, events, "session.renamed", func(e protocol.Event) bool { return e.Type == protocol.EventSessionRenamed })
	if ev.SessionID != s.ID || ev.SessionName != "api server" || ev.ProjectName != "demo" {
		t.Fatalf("renamed event %+v", ev)
	}
	if got := findSession(t, c, s.ID).Name; got != "api server" {
		t.Fatalf("listed name %q", got)
	}
	if recs := readRecords(t, p); len(recs) != 1 || recs[0].Name != "api server" {
		t.Fatalf("records %+v", recs)
	}
	long, err := c.Rename(ctxT(t), s.ID, strings.Repeat("é", 200))
	if err != nil || len([]rune(long.Name)) != protocol.MaxSessionName {
		t.Fatalf("long name: %d runes, %v", len([]rune(long.Name)), err)
	}
	if _, err := c.Rename(ctxT(t), s.ID, "api server"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rename(ctxT(t), "nope", "x"); err == nil {
		t.Fatal("rename of an unknown session succeeded")
	}

	// stop --force + restart: the suspended session keeps its name, and so
	// does the resumed one.
	if err := c.Shutdown(ctxT(t), true); err != nil {
		t.Fatal(err)
	}
	stop()
	c, stop = restartDaemon(t, p)
	defer stop()
	if got := findSession(t, c, s.ID); got.State != "suspended" || got.Name != "api server" {
		t.Fatalf("after restart: %+v", got)
	}
	res, err := c.Resume(ctxT(t), s.ID, false)
	if err != nil || len(res.Sessions) != 1 || res.Sessions[0].Name != "api server" {
		t.Fatalf("resume: %+v %v", res, err)
	}
	if recs := readRecords(t, p); len(recs) != 1 || recs[0].Name != "api server" {
		t.Fatalf("records after resume %+v", recs)
	}

	// An empty name resets to the profile's.
	info, err = c.Rename(ctxT(t), s.ID, "   ")
	if err != nil || info.Name != "" {
		t.Fatalf("reset: %+v %v", info, err)
	}
	if recs := readRecords(t, p); len(recs) != 1 || recs[0].Name != "" {
		t.Fatalf("records after reset %+v", recs)
	}
}
