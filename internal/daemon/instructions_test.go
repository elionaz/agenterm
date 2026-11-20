package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/protocol"
)

func TestInstructions(t *testing.T) {
	codexHome := t.TempDir() // never the user's ~/.codex
	t.Setenv("CODEX_HOME", codexHome)
	c, p, stop := startDaemon(t)
	defer stop()
	writeProfiles(t, p.Profiles(), "- id: hookedcodex\n  cmd: /bin/echo\n  setup_hooks: codex\n")
	ctx := context.Background()
	web, err := c.ProjectAdd(ctx, t.TempDir(), "web")
	if err != nil {
		t.Fatal(err)
	}
	api, err := c.ProjectAdd(ctx, t.TempDir(), "api")
	if err != nil {
		t.Fatal(err)
	}

	save := func(tp protocol.InstructionTopic, old string) protocol.Instructions {
		t.Helper()
		r, err := c.SaveInstruction(ctx, tp, old, "web")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	save(protocol.InstructionTopic{Title: "General", Load: "always", Enabled: true, Body: "Use pnpm."}, "")
	save(protocol.InstructionTopic{Title: "Git", Description: "Branches and PRs", Load: "on_demand", Enabled: true, Body: "Never force-push."}, "")
	r := save(protocol.InstructionTopic{Title: "Web only", Load: "always", Enabled: true, Projects: []string{web.ID}, Body: "Run make test."}, "")
	if len(r.Topics) != 3 || r.ProjectID != web.ID {
		t.Fatalf("topics = %+v", r.Topics)
	}
	for _, want := range []string{"Use pnpm.", "Run make test.", "- git: Branches and PRs"} {
		if !strings.Contains(r.Composed, want) {
			t.Fatalf("web composed lacks %q:\n%s", want, r.Composed)
		}
	}
	if strings.Contains(r.Composed, "force-push") {
		t.Fatal("an on-demand text was included")
	}
	if ra, _ := c.Instructions(ctx, api.ID); strings.Contains(ra.Composed, "make test") || !strings.Contains(ra.Composed, "pnpm") {
		t.Fatalf("api composed:\n%s", ra.Composed)
	}
	// Private files in the config dir, nothing in the project.
	if fi, err := os.Stat(filepath.Join(p.ConfigDir, "instructions", "git.md")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("topic file: %v %v", fi, err)
	}
	if ents, _ := os.ReadDir(web.Path); len(ents) != 0 {
		t.Fatalf("project folder written: %v", ents)
	}

	// show: what an agent reads for an on-demand topic.
	if txt, err := c.ShowInstruction(ctx, "git"); err != nil || txt.Body != "Never force-push." || txt.Title != "Git" {
		t.Fatalf("show = %+v, %v", txt, err)
	}
	if _, err := c.ShowInstruction(ctx, "nope"); notCode(err, apierr.InstructionsNotFound) {
		t.Fatalf("show missing: %v", err)
	}

	// codex gets the composed text as developer_instructions; claude in
	// its SessionStart hook, not in its arguments.
	hasInstr := func(args []string) bool {
		return slices.ContainsFunc(args, func(a string) bool {
			return strings.HasPrefix(a, "developer_instructions=") && strings.Contains(a, "Use pnpm.") && strings.Contains(a, "Run make test.")
		})
	}
	cx, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: web.ID, ProfileID: "hookedcodex"})
	if err != nil {
		t.Fatal(err)
	}
	if !hasInstr(cx.Args) {
		t.Fatalf("codex args = %q", cx.Args)
	}
	cl, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: web.ID, ProfileID: "hookedclaude"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(cl.Args, func(a string) bool { return strings.Contains(a, "pnpm") }) {
		t.Fatalf("claude args = %q", cl.Args)
	}

	// Reorder, disable, delete.
	r, err = c.ReorderInstructions(ctx, []string{"web-only", "git"}, "")
	if err != nil || r.Topics[0].Slug != "web-only" || r.Topics[2].Slug != "general" {
		t.Fatalf("reorder = %+v, %v", r.Topics, err)
	}
	gen := r.Topics[2]
	gen.Enabled = false
	if r = save(gen, "general"); strings.Contains(r.Composed, "pnpm") {
		t.Fatalf("disabled topic composed:\n%s", r.Composed)
	}
	if _, err := c.DeleteInstruction(ctx, "git", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteInstruction(ctx, "git", ""); notCode(err, apierr.InstructionsNotFound) {
		t.Fatalf("delete twice: %v", err)
	}

	// The user's own developer_instructions in config.toml are left alone.
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("developer_instructions = \"mine\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r, _ := c.Instructions(ctx, ""); !r.CodexOwn {
		t.Fatal("CodexOwn not reported")
	}
	cx2, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: web.ID, ProfileID: "hookedcodex"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(cx2.Args, func(a string) bool { return strings.HasPrefix(a, "developer_instructions=") }) {
		t.Fatalf("codex args replaced the user's instructions: %q", cx2.Args)
	}
}

// notCode: err is not a refusal with code want.
func notCode(err error, want string) bool {
	code, _, _ := apierr.Code(err)
	return code != want
}
