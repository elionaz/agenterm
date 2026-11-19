package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/gitx"
	"github.com/elionaz/agenterm/internal/protocol"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", msg)
}

func identity(t *testing.T, dir, name string) {
	t.Helper()
	gitIn(t, dir, "config", "user.name", name)
	gitIn(t, dir, "config", "user.email", strings.ToLower(name)+"@example.com")
	gitIn(t, dir, "config", "commit.gpgsign", "false")
}

// A worktree whose pull requests were merged on the host — with a merge
// commit for an older one and a squash for its own — shows no changes,
// even though the local main was never pulled.
func TestGitInfoAfterMergesOnTheHost(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, bare, host, wt := filepath.Join(tmp, "repo"), filepath.Join(tmp, "remote.git"), filepath.Join(tmp, "host"), filepath.Join(tmp, "wt")
	gitIn(t, tmp, "init", "-q", "-b", "main", repo)
	identity(t, repo, "Test")
	commitFile(t, repo, "a.txt", "one\n", "initial")
	gitIn(t, tmp, "init", "-q", "--bare", "-b", "main", bare)
	gitIn(t, repo, "remote", "add", "origin", bare)
	gitIn(t, repo, "push", "-q", "-u", "origin", "main")
	gitIn(t, tmp, "clone", "-q", bare, host)
	identity(t, host, "Host")
	commitFile(t, host, "b.txt", "pr 1\n", "pr 1")
	gitIn(t, host, "push", "-q", "origin", "main")

	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := gitx.Fetch(c, repo, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "worktree", "add", "-q", "-b", "agent/x", wt, "origin/main")
	commitFile(t, wt, "c.txt", "mine\n", "my change")
	g := sessGit{Mode: protocol.ModeWorktree, Dir: wt, Branch: "agent/x", Base: "main"}

	info, err := gitInfoFor(c, g)
	if err != nil {
		t.Fatal(err)
	}
	if info.Ahead != 1 || info.Files != 1 {
		t.Fatalf("before the merge: ahead %d, files %d; want 1, 1 (not pr 1's file)", info.Ahead, info.Files)
	}

	// The host squash-merges the branch.
	commitFile(t, host, "c.txt", "mine\n", "squashed: my change")
	gitIn(t, host, "push", "-q", "origin", "main")
	if err := gitx.Fetch(c, repo, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if info, err = gitInfoFor(c, g); err != nil {
		t.Fatal(err)
	}
	if info.Ahead != 0 || info.Files != 0 {
		t.Fatalf("after the squash merge: ahead %d, files %d; want 0, 0", info.Ahead, info.Files)
	}
	// Uncommitted work still counts.
	if err := os.WriteFile(filepath.Join(wt, "d.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err = gitInfoFor(c, g); err != nil {
		t.Fatal(err)
	}
	if info.Files != 1 || info.Uncommitted != 1 {
		t.Fatalf("with uncommitted work: files %d, uncommitted %d; want 1, 1", info.Files, info.Uncommitted)
	}
}
