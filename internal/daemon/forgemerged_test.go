package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elionaz/panal/internal/protocol"
)

// When the host says a worktree's pull request was merged and the local
// base does not show it yet, forge.status fetches the base right away
// (not at the next periodic fetch) and the session's changes drop to zero.
func TestForgeStatusMergedFetchesBase(t *testing.T) {
	// gh answers MERGED for feat/x.
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *"--head feat/x "*) echo '[{"number":31,"title":"X","state":"MERGED","url":"https://github.com/acme/app/pull/31","baseRefName":"main","headRefName":"feat/x","statusCheckRollup":[]}]' ;;
  "pr list"*) echo '[]' ;;
  "repo view"*) echo '{"squashMergeAllowed":true}' ;;
  *) echo "fake gh: $*" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	testExtraPath.Store(bin)
	t.Cleanup(func() { testExtraPath.Store("") })

	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	repo := tempRepo(t)
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare, host := filepath.Join(tmp, "remote.git"), filepath.Join(tmp, "host")
	// main follows a local bare repository ("hub", what gets fetched);
	// branches are pushed to a GitHub origin (what gh is asked about,
	// never contacted).
	gitT(t, tmp, "init", "-q", "--bare", "-b", "main", bare)
	gitT(t, repo, "remote", "add", "hub", bare)
	gitT(t, repo, "push", "-q", "-u", "hub", "main")
	gitT(t, repo, "remote", "add", "origin", "https://github.com/acme/app.git")
	gitT(t, repo, "config", "remote.pushDefault", "origin")
	gitT(t, tmp, "clone", "-q", bare, host)
	gitT(t, host, "config", "user.name", "Host")
	gitT(t, host, "config", "user.email", "host@example.com")
	gitT(t, host, "config", "commit.gpgsign", "false")

	proj, err := c.ProjectAdd(ctx, repo, "app")
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, s.Cwd, "switch", "-q", "-c", "feat/x")
	if err := os.WriteFile(filepath.Join(s.Cwd, "c.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, s.Cwd, "add", "c.txt")
	gitT(t, s.Cwd, "commit", "-q", "-m", "my change")
	d, err := c.SessionDiff(ctx, s.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Git.Ahead != 1 || len(d.Files) != 1 {
		t.Fatalf("before the merge: ahead %d, files %d; want 1, 1", d.Git.Ahead, len(d.Files))
	}

	// The host squash-merges it; nothing here has fetched since.
	if err := os.WriteFile(filepath.Join(host, "c.txt"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, host, "add", "c.txt")
	gitT(t, host, "commit", "-q", "-m", "X (#31)")
	gitT(t, host, "push", "-q", "origin", "main")

	ev := eventStream(t, c, "test")
	st, err := c.ForgeStatus(ctx, protocol.ForgeRef{SessionID: s.ID})
	if err != nil {
		t.Fatal(err)
	}
	if st.PR == nil || st.PR.State != "merged" || st.Ahead != 1 {
		t.Fatalf("status: pr %+v, ahead %d", st.PR, st.Ahead)
	}
	waitEvent(t, ev, "the session's changes to drop to zero", func(e protocol.Event) bool {
		return e.Type == protocol.EventSessionGit && e.SessionID == s.ID && e.Git != nil && e.Git.Merged && e.Git.Files == 0
	})
	if d, err = c.SessionDiff(ctx, s.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(d.Files) != 0 || len(d.Commits) != 0 {
		t.Fatalf("after the fetch: files %d, commits %d; want 0, 0", len(d.Files), len(d.Commits))
	}
}
