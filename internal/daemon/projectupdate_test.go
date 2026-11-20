package daemon_test

import (
	"testing"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/protocol"
)

func TestProjectUpdate(t *testing.T) {
	c, _, _ := startDaemon(t)
	ctx := ctxT(t)
	a, err := c.ProjectAdd(ctx, t.TempDir(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ProjectAdd(ctx, t.TempDir(), "web"); err != nil {
		t.Fatal(err)
	}
	sess := spawnWith(t, c, a.ID, "shell", "", "")
	events := eventStream(t, c, "")

	name, color, icon := "backend", "purple", "🦊"
	got, err := c.UpdateProject(ctx, protocol.ProjectUpdateParams{ID: "api", Name: &name, Color: &color, Icon: &icon}) // by name
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != a.ID || got.Name != name || got.Color != color || got.Icon != icon {
		t.Fatalf("update: %+v", got)
	}
	waitEvent(t, events, "project.changed", func(e protocol.Event) bool { return e.Type == protocol.EventProjects })
	if list, _ := c.ProjectList(ctx); list[0].Name != name || list[0].Icon != icon {
		t.Fatalf("list: %+v", list)
	}
	// Running sessions report the new name (notifications use it).
	sessions, err := c.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.ID == sess.ID && s.ProjectName != name {
			t.Fatalf("session project name %q, want %q", s.ProjectName, name)
		}
	}

	taken := "web"
	_, err = c.UpdateProject(ctx, protocol.ProjectUpdateParams{ID: a.ID, Name: &taken})
	if code, _, _ := apierr.Code(err); code != apierr.ProjectNameTaken {
		t.Fatalf("taken name: %v (code %q)", err, code)
	}
}
