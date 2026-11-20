package daemon_test

import (
	"context"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/protocol"
)

func TestHandoff(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := context.Background()
	repo := tempRepo(t)
	proj, err := c.ProjectAdd(ctx, repo, "web")
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitT(t, repo, "rev-parse", "HEAD"))

	old, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rename(ctx, old.ID, "fix login"); err != nil {
		t.Fatal(err)
	}
	runIn(t, c, old.ID, "echo halfway > notes.txt; echo HANDOFF-MARK")

	n, err := c.HandoffNote(ctx, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Handoff", `"fix login"`, `project "web"`, "Folder " + repo, "Uncommitted changes (1):", "- notes.txt",
		"## The end of its terminal", "HANDOFF-MARK"} {
		if !strings.Contains(n.Text, want) {
			t.Errorf("note lacks %q:\n%s", want, n.Text)
		}
	}
	if n.Shared {
		t.Error("shared with nobody")
	}

	if _, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: old.ID, ProfileID: "hookedclaude", Note: " "}); err == nil {
		t.Fatal("empty note accepted")
	}
	if _, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: old.ID, ProfileID: "hookedclaude", Note: "x", Where: "moon"}); err == nil {
		t.Fatal("bad where accepted")
	}

	// Claude Code gets the note as its first prompt, in the same folder;
	// the original closes and its name carries over.
	cl, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: old.ID, ProfileID: "hookedclaude", Note: "Continue: finish notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if cl.Cwd != repo || cl.Mode == protocol.ModeWorktree || len(cl.Args) == 0 || cl.Args[len(cl.Args)-1] != "Continue: finish notes.txt" || cl.Name != "fix login" {
		t.Fatalf("claude handoff = %+v", cl)
	}
	waitFor(t, "original closed", func() bool { return findSession(t, c, old.ID).State == "exited" })

	// A new worktree from the current commit, kept original, an agent
	// without integration: it is told where the note is.
	src, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: src.ID, ProfileID: "shell", Where: protocol.HandoffWorktree,
		Note: "Continue in a worktree", Keep: true, Cols: 220, Rows: 40}) // wide: the typed line must not wrap
	if err != nil {
		t.Fatal(err)
	}
	if wt.Mode != protocol.ModeWorktree || wt.WorktreeID == "" || wt.Base != head {
		t.Fatalf("worktree handoff = %+v (head %s)", wt, head)
	}
	defer func() {
		if t.Failed() {
			lines, _ := c.Tail(ctx, wt.ID, 20)
			t.Logf("state %s tail:\n%s", findSession(t, c, wt.ID).State, strings.Join(lines, "\n"))
		}
	}()
	waitFor(t, "the note's path typed", func() bool {
		lines, _ := c.Tail(ctx, wt.ID, 20)
		return strings.Contains(strings.Join(lines, "\n"), "Read the handoff note in ")
	})
	if s := findSession(t, c, src.ID); s.State == "exited" {
		t.Fatal("kept original closed")
	}
}
