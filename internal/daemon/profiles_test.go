package daemon_test

import (
	"os"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/settings"
)

func TestProfileRPCs(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)

	cat, err := c.ProfileCatalog(ctx)
	if err != nil || len(cat) < 12 {
		t.Fatalf("catalog %d %v", len(cat), err)
	}
	for _, e := range cat {
		// hermeticPath has no agents; only the shell is "installed".
		if e.Installed != (e.ID == "shell") {
			t.Errorf("%s installed=%v", e.ID, e.Installed)
		}
	}

	enabled := false
	ps, err := c.SaveProfile(ctx, profile.Entry{ID: "cat", Cmd: "/bin/cat", Status: "hooks", Enabled: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if pr, _ := profile.Find(ps, "cat"); pr.Enabled || !pr.Installed || pr.Source != profile.SourceCustom {
		t.Fatalf("cat = %+v", pr)
	}
	if _, err := c.SaveProfile(ctx, profile.Entry{ID: "bad", Cmd: "x", AttentionPatterns: &[]string{"("}}); err == nil ||
		!strings.Contains(err.Error(), "attention_patterns") {
		t.Fatalf("validation error = %v", err)
	}
	if _, err := os.Stat(p.Profiles() + ".bak"); err != nil {
		t.Fatalf("no .bak: %v", err)
	}
	ps, err = c.ReorderProfiles(ctx, []string{"shell"})
	if err != nil || ps[0].ID != "shell" {
		t.Fatalf("reorder %v %v", ps, err)
	}
	if ps, err = c.DeleteProfile(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := profile.Find(ps, "hello"); ok {
		t.Fatal("hello not deleted")
	}
	if ps, err = c.RescanProfiles(ctx); err != nil || ps[0].ID != "shell" {
		t.Fatalf("rescan %v", err)
	}

	// A catalog agent that is not installed fails clearly at spawn.
	proj, err := c.ProjectAdd(ctx, p.ConfigDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "gemini"}); err == nil ||
		!strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "npm") {
		t.Fatalf("spawn missing agent: %v", err)
	}

	st, err := c.Settings(ctx)
	if err != nil || st != settings.Defaults() {
		t.Fatalf("settings %+v %v", st, err)
	}
	st.Notify.OnDone = false
	if _, err := c.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	if got, _ := settings.Load(p.Settings()); got != st {
		t.Fatalf("saved %+v", got)
	}
}
