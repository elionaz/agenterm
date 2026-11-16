package daemon_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/protocol"
)

// TestMain isolates git (and the shells the tests spawn) from the user's
// configuration: no system config, an empty global one, a throwaway HOME.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "agt-home")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(home, "gitconfig")
	_ = os.WriteFile(global, nil, 0o600)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	// Test daemons never post real desktop notifications (they also get a
	// no-op or recording Notifier).
	os.Setenv("AGENTERM_NOTIFY", "off")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// tempRepo is a throwaway repository on main with one commit.
func tempRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "config", "user.name", "Test")
	gitT(t, dir, "config", "user.email", "test@example.com")
	gitT(t, dir, "config", "commit.gpgsign", "false")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644)
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// runIn types a command into a shell session and waits for its marker.
func runIn(t *testing.T, c *client.Client, id, cmd string) {
	t.Helper()
	st, err := c.Attach(ctxT(t), id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool { return f.Type == protocol.FrameReplayDone })
	if err := st.Input([]byte(cmd + "; echo MARK''ER\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte("MARKER")) })
}

func listWorktrees(t *testing.T, c *client.Client, projectID string) []protocol.WorktreeInfo {
	t.Helper()
	wts, err := c.Worktrees(ctxT(t), projectID)
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

func TestWorktreeSession(t *testing.T) {
	c, p, stop := startDaemon(t)
	ctx := ctxT(t)
	repo := tempRepo(t)
	proj, err := c.ProjectAdd(ctx, repo, "Mi Repo")
	if err != nil {
		t.Fatal(err)
	}
	pg, err := c.ProjectGit(ctx, proj.ID)
	if err != nil || !pg.IsRepo || pg.CurrentBranch != "main" || pg.DefaultBase != "main" || pg.Error != "" {
		t.Fatalf("project.git = %+v, %v", pg, err)
	}

	// A non-repo project cannot use worktrees.
	plain, _ := c.ProjectAdd(ctx, t.TempDir(), "plain")
	if pg, _ := c.ProjectGit(ctx, plain.ID); pg.IsRepo || pg.Error == "" || pg.ErrorCode != apierr.NotARepo {
		t.Fatalf("project.git(plain) = %+v", pg)
	}
	if _, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: plain.ID, ProfileID: "shell", Mode: protocol.ModeWorktree}); err == nil ||
		!strings.Contains(err.Error(), "not a git repository") || !hasCode(err, apierr.NotARepo) {
		t.Fatalf("worktree spawn in plain dir: %v", err)
	}
	if _, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree, Base: "nope"}); err == nil ||
		!strings.Contains(err.Error(), "does not exist") || !hasCode(err, apierr.BaseMissing) {
		t.Fatalf("missing base: %v", err)
	}

	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(p.RuntimeDir, "worktrees", "mi-repo-"+proj.ID, info.ID)
	if real, err := filepath.EvalSymlinks(p.RuntimeDir); err == nil {
		wantDir = filepath.Join(real, "worktrees", "mi-repo-"+proj.ID, info.ID)
	}
	if info.Mode != protocol.ModeWorktree || info.WorktreeID != info.ID || info.WorktreePath != wantDir || info.Cwd != wantDir ||
		info.Base != "main" || info.Branch != "agent/shell-"+info.ID[:4] {
		t.Fatalf("spawn info = %+v (want dir %s)", info, wantDir)
	}
	if got := gitT(t, wantDir, "branch", "--show-current"); got != info.Branch {
		t.Fatalf("worktree branch = %q", got)
	}

	// Changes made through the session show up in its diff.
	runIn(t, c, info.ID, "pwd > where.txt && git add where.txt && git commit -qm 'add where' && echo two >> a.txt && printf 'x\\000y' > bin.dat")
	d, err := c.SessionDiff(ctx, info.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range d.Files {
		files[f.Path] = f.Status
		if f.Path == "bin.dat" && (!f.Binary || !f.Untracked || f.Patch != "") {
			t.Errorf("bin.dat = %+v", f)
		}
	}
	if len(d.Files) != 3 || files["where.txt"] != "A" || files["a.txt"] != "M" || files["bin.dat"] != "A" {
		t.Fatalf("diff files = %+v", d.Files)
	}
	if len(d.Commits) != 1 || d.Commits[0].Subject != "add where" || d.Git.Ahead != 1 || d.Git.Files != 3 || d.Git.Uncommitted != 2 {
		t.Fatalf("diff commits/git = %+v / %+v", d.Commits, d.Git)
	}
	if !strings.Contains(d.Patch(), "+two") || d.Mode != protocol.ModeWorktree || d.Branch != info.Branch {
		t.Fatalf("patch/meta wrong: %+v\n%s", d, d.Patch())
	}
	// The cached summary reaches session.list.
	if s := findSession(t, c, info.ID); s.Git == nil || s.Git.Files != 3 || s.Git.Ahead != 1 {
		t.Fatalf("session git = %+v", s.Git)
	}
	if wts := listWorktrees(t, c, proj.ID); len(wts) != 1 || !wts[0].Live || wts[0].Orphan || wts[0].Branch != info.Branch {
		t.Fatalf("worktree.list (live) = %+v", wts)
	}
	if _, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: info.ID, Force: true}); err == nil || !strings.Contains(err.Error(), "still running") || !hasCode(err, apierr.WorktreeLive) {
		t.Fatalf("remove with a live session: %v", err)
	}

	// Killing the session keeps the worktree; forgetting it makes it an orphan.
	if _, err := c.Kill(ctx, info.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, info.ID, "exited")
	if _, err := os.Stat(filepath.Join(wantDir, "where.txt")); err != nil {
		t.Fatalf("worktree gone after kill: %v", err)
	}
	if wts := listWorktrees(t, c, proj.ID); len(wts) != 1 || wts[0].Live || wts[0].Orphan {
		t.Fatalf("worktree.list (exited) = %+v", wts)
	}
	if _, err := c.Kill(ctx, info.ID); err != nil { // remove from the list
		t.Fatal(err)
	}
	wts := listWorktrees(t, c, proj.ID)
	if len(wts) != 1 || !wts[0].Orphan || wts[0].Git == nil || wts[0].Git.Files != 3 {
		t.Fatalf("worktree.list (orphan) = %+v", wts)
	}
	// An orphan still has a diff.
	if d, err := c.WorktreeDiff(ctx, info.ID, true); err != nil || len(d.Files) != 3 || d.Patch() != "" {
		t.Fatalf("worktree.diff = %+v, %v", d, err)
	}

	// The registry survives a daemon restart.
	stop()
	c, _ = startDaemonIn(t, p, func(daemon.Notification) {})
	ctx = ctxT(t)
	wts = listWorktrees(t, c, proj.ID)
	if len(wts) != 1 || !wts[0].Orphan || wts[0].ID != info.ID {
		t.Fatalf("worktree.list after restart = %+v", wts)
	}

	// Reopen a shell in it.
	re, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", WorktreeID: info.ID})
	if err != nil || re.Cwd != wantDir || re.WorktreeID != info.ID || re.Branch != info.Branch {
		t.Fatalf("reopen = %+v, %v", re, err)
	}
	if wts := listWorktrees(t, c, proj.ID); len(wts) != 1 || !wts[0].Live {
		t.Fatalf("worktree.list (reopened) = %+v", wts)
	}
	if _, err := c.Kill(ctx, re.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, re.ID, "exited")

	// Removal refuses to lose uncommitted work unless forced.
	dry, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: info.ID, DeleteBranch: true, DryRun: true})
	if err != nil || dry.Removed || dry.Loss == nil || len(dry.Loss.Uncommitted) != 2 || len(dry.Loss.Unmerged) != 1 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if _, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: info.ID, DeleteBranch: true}); err == nil ||
		!strings.Contains(err.Error(), "uncommitted") || !strings.Contains(err.Error(), "not merged") || !hasCode(err, apierr.WorktreeDirty) {
		t.Fatalf("unforced remove: %v", err)
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatal("refused remove deleted the worktree")
	}
	res, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: info.ID, DeleteBranch: true, Force: true})
	if err != nil || !res.Removed {
		t.Fatalf("forced remove = %+v, %v", res, err)
	}
	if _, err := os.Stat(wantDir); !os.IsNotExist(err) {
		t.Fatal("worktree folder still there")
	}
	if out := gitT(t, repo, "branch", "--list", info.Branch); out != "" {
		t.Fatalf("branch not deleted: %q", out)
	}
	if wts := listWorktrees(t, c, proj.ID); len(wts) != 0 {
		t.Fatalf("worktree.list after remove = %+v", wts)
	}
}

func TestWorktreeMergeAndSameMode(t *testing.T) {
	c, _, _ := startDaemon(t)
	ctx := ctxT(t)
	repo := tempRepo(t)
	proj, _ := c.ProjectAdd(ctx, repo, "")

	wt, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree, Branch: "Mi Feature"})
	if err != nil || wt.Branch != "Mi-Feature" {
		t.Fatalf("spawn = %+v, %v", wt, err)
	}
	runIn(t, c, wt.ID, "echo feat > feat.txt && git add feat.txt && git commit -qm feat")

	// Refused while the main tree is dirty.
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("dirty\n"), 0o644)
	if _, err := c.WorktreeMerge(ctx, wt.WorktreeID); err == nil || !strings.Contains(err.Error(), "uncommitted") || !hasCode(err, apierr.MergeMainDirty) {
		t.Fatalf("merge into dirty main: %v", err)
	}
	gitT(t, repo, "checkout", "--", "a.txt")
	res, err := c.WorktreeMerge(ctx, wt.WorktreeID)
	if err != nil || res.Commits != 1 || res.Base != "main" {
		t.Fatalf("merge = %+v, %v", res, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "feat.txt")); err != nil {
		t.Fatal("merged file missing")
	}

	// Hook integration works the same in a worktree: it goes through
	// --settings (a file in the daemon's hooks dir), never project files.
	hk, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hookedclaude", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	if len(hk.Args) < 2 || hk.Args[0] != "--settings" || !filepath.IsAbs(hk.Args[1]) || strings.HasPrefix(hk.Args[1], hk.WorktreePath) {
		t.Fatalf("hook args in worktree = %v (worktree %s)", hk.Args, hk.WorktreePath)
	}
	if _, err := os.Stat(filepath.Join(hk.WorktreePath, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("hook setup wrote into the worktree: %v", err)
	}

	// Same-folder sessions diff the project against HEAD.
	same, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil || same.Mode != protocol.ModeSame || same.Cwd != repo || same.WorktreeID != "" {
		t.Fatalf("same spawn = %+v, %v", same, err)
	}
	runIn(t, c, same.ID, "echo new > untracked.txt")
	// GitInfo is refreshed lazily once the shell goes idle.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := findSession(t, c, same.ID)
		if s.Git != nil && s.Git.Files == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("same session git = %+v", s.Git)
		}
		time.Sleep(50 * time.Millisecond)
	}
	d, err := c.SessionDiff(ctx, same.ID, false)
	if err != nil || d.Mode != protocol.ModeSame || len(d.Files) != 1 || d.Files[0].Path != "untracked.txt" || !d.Files[0].Untracked || d.Branch != "main" {
		t.Fatalf("same diff = %+v, %v", d, err)
	}
}

// hasCode reports whether a daemon error carries code (protocol errorCode).
func hasCode(err error, code string) bool {
	c, _, ok := apierr.Code(err)
	return ok && c == code
}
