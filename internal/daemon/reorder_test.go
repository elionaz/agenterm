package daemon_test

import (
	"slices"
	"testing"

	"github.com/elionaz/panal/internal/protocol"
)

func TestReorderProjectsAndSessions(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	a, err := c.ProjectAdd(ctx, t.TempDir(), "a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.ProjectAdd(ctx, t.TempDir(), "b")
	if err != nil {
		t.Fatal(err)
	}
	events := eventStream(t, c, "")

	list, err := c.ReorderProjects(ctx, []string{b.ID})
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("reorder projects: %v %v", list, err)
	}
	waitEvent(t, events, "project.changed", func(e protocol.Event) bool { return e.Type == protocol.EventProjects })
	if got, _ := c.ProjectList(ctx); got[0].ID != b.ID {
		t.Fatalf("project order not kept: %v", got)
	}

	// a1 b1 a2 a3 → a's sessions become a3 a1 a2; b1 keeps its place.
	a1 := spawnWith(t, c, a.ID, "shell", "", "")
	b1 := spawnWith(t, c, b.ID, "shell", "", "")
	a2 := spawnWith(t, c, a.ID, "shell", "", "")
	a3 := spawnWith(t, c, a.ID, "shell", "", "")
	sessions, err := c.ReorderSessions(ctx, "a", []string{a3.ID}) // by name, like spawn
	if err != nil {
		t.Fatal(err)
	}
	ids := func(ss []protocol.SessionInfo) []string {
		var out []string
		for _, s := range ss {
			out = append(out, s.ID)
		}
		return out
	}
	want := []string{a3.ID, b1.ID, a1.ID, a2.ID}
	if got := ids(sessions); !slices.Equal(got, want) {
		t.Fatalf("session order %v, want %v", got, want)
	}
	ev := waitEvent(t, events, "session.reordered", func(e protocol.Event) bool { return e.Type == protocol.EventSessionsReordered })
	if ev.ProjectID != a.ID {
		t.Fatalf("reordered event %+v", ev)
	}
	var recs []string
	for _, r := range readRecords(t, p) {
		recs = append(recs, r.ID)
	}
	if !slices.Equal(recs, want) {
		t.Fatalf("persisted order %v, want %v", recs, want)
	}
	if _, err := c.ReorderSessions(ctx, a.ID, []string{b1.ID}); err == nil {
		t.Fatal("a session of another project must fail")
	}
	if _, err := c.ReorderProjects(ctx, []string{"nope"}); err == nil {
		t.Fatal("unknown project must fail")
	}
}
