package daemon_test

import (
	"context"
	"slices"
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
	if !slices.Equal(cl.ContinuedFrom, []string{old.ID}) || !slices.Equal(findSession(t, c, cl.ID).ContinuedFrom, []string{old.ID}) {
		t.Fatalf("continuedFrom = %q, want [%s]", findSession(t, c, cl.ID).ContinuedFrom, old.ID)
	}
	if o := findSession(t, c, old.ID); !o.Stopped {
		t.Fatalf("handed-off session not marked stopped: %+v", o)
	}

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

func TestStoppedVsFailed(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := context.Background()
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "p")
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Kill(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stopped exit", func() bool { return findSession(t, c, a.ID).State == "exited" })
	if s := findSession(t, c, a.ID); !s.Stopped {
		t.Fatalf("stopped session: %+v", s)
	}
	b, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, c, b.ID, "true")
	st, err := c.Attach(ctx, b.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Input([]byte("exit 3\n"))
	st.Close()
	waitFor(t, "failed exit", func() bool { return findSession(t, c, b.ID).State == "exited" })
	if s := findSession(t, c, b.ID); s.Stopped || s.ExitCode == nil || *s.ExitCode != 3 {
		t.Fatalf("failed session: stopped %v exit %v", s.Stopped, s.ExitCode)
	}
}

func TestHandoffInto(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := context.Background()
	repo := tempRepo(t)
	proj, err := c.ProjectAdd(ctx, repo, "web")
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.ProjectAdd(ctx, t.TempDir(), "other")
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(pid string) protocol.SessionInfo {
		t.Helper()
		s, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: pid, ProfileID: "shell"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a, b, target, far := spawn(proj.ID), spawn(proj.ID), spawn(proj.ID), spawn(other.ID)

	for _, bad := range []string{a.ID, far.ID, "nope"} {
		if _, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: a.ID, TargetID: bad, Note: "x"}); err == nil {
			t.Errorf("handoff into %s accepted", bad)
		}
	}

	got, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: a.ID, TargetID: target.ID, Note: "Finish the cart."})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != target.ID || !slices.Equal(got.ContinuedFrom, []string{a.ID}) {
		t.Fatalf("into = %+v", got)
	}
	waitFor(t, "a closed", func() bool { s := findSession(t, c, a.ID); return s.State == "exited" && s.Stopped })
	ms, err := c.MsgList(ctx, protocol.MsgListParams{SessionID: target.ID, PendingOnly: true})
	if err != nil || len(ms) != 1 || !strings.Contains(ms[0].Body, "-from-"+a.ID+".md") {
		t.Fatalf("pending for the target = %+v, %v", ms, err)
	}

	// A second session handed into the same one: both listed, oldest first.
	if _, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: b.ID, TargetID: target.ID, Note: "And the tests."}); err != nil {
		t.Fatal(err)
	}
	if s := findSession(t, c, target.ID); !slices.Equal(s.ContinuedFrom, []string{a.ID, b.ID}) {
		t.Fatalf("continuedFrom = %q", s.ContinuedFrom)
	}
	if _, err := c.Handoff(ctx, protocol.HandoffParams{SessionID: far.ID, TargetID: a.ID, Note: "x"}); err == nil {
		t.Fatal("handoff into an exited session accepted")
	}
}
