package daemon_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/protocol"
)

// waitFile waits for path to exist.
func waitFile(t *testing.T, path string, d time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func TestWorktreeSetup(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	repo := tempRepo(t)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\nsetup-ran\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".panal.yaml"), []byte("worktree:\n  copy: [.env]\n  setup: touch setup-ran\n"), 0o644)
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-qm", "panal config")
	os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1\n"), 0o600)
	proj, err := c.ProjectAdd(ctx, repo, "setup")
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(p protocol.SpawnParams) (protocol.SessionInfo, error) {
		p.ProjectID, p.ProfileID = proj.ID, "shell"
		if p.WorktreeID == "" {
			p.Mode = protocol.ModeWorktree
		}
		return c.Spawn(ctx, p)
	}

	// Not trusted yet: refused, and no worktree is left behind.
	if _, err := spawn(protocol.SpawnParams{}); !hasCode(err, apierr.SetupUntrusted) {
		t.Fatalf("untrusted setup: %v", err)
	}
	if wts := listWorktrees(t, c, proj.ID); len(wts) != 0 {
		t.Fatalf("worktrees after a refusal: %+v", wts)
	}

	// Trusted: the file is copied, the setup runs in the worktree, then the
	// agent (here a shell) takes over; the session shows the agent's own
	// command, not the wrapper.
	a, err := spawn(protocol.SpawnParams{TrustSetup: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(a.WorktreePath, ".env")); string(got) != "SECRET=1\n" {
		t.Fatalf(".env in the worktree = %q", got)
	}
	if !waitFile(t, filepath.Join(a.WorktreePath, "setup-ran"), 10*time.Second) {
		t.Fatal("the setup did not run")
	}
	if slices.Contains(a.Args, "panal-setup") || slices.Contains(a.Args, "-c") {
		t.Fatalf("session shows the wrapper: %s %v", a.Cmd, a.Args)
	}
	runIn(t, c, a.ID, "true") // the shell runs after the setup

	// Trusted once: the next worktree runs it without asking.
	b, err := spawn(protocol.SpawnParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !waitFile(t, filepath.Join(b.WorktreePath, "setup-ran"), 10*time.Second) {
		t.Fatal("the trusted setup did not run again")
	}

	// Reopening a worktree runs nothing again.
	os.Remove(filepath.Join(a.WorktreePath, "setup-ran"))
	if _, err := c.Kill(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := c.Kill(ctx, a.ID); err != nil { // forget the exited session
		t.Fatal(err)
	}
	re, err := spawn(protocol.SpawnParams{WorktreeID: a.WorktreeID})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, c, re.ID, "true")
	if _, err := os.Stat(filepath.Join(a.WorktreePath, "setup-ran")); err == nil {
		t.Fatal("reopening the worktree ran the setup again")
	}

	// A changed command asks again; skipSetup creates it without running it.
	os.WriteFile(filepath.Join(repo, ".panal.yaml"), []byte("worktree:\n  copy: [.env]\n  setup: touch setup-ran && true\n"), 0o644)
	if _, err := spawn(protocol.SpawnParams{}); !hasCode(err, apierr.SetupUntrusted) {
		t.Fatalf("changed setup: %v", err)
	}
	s, err := spawn(protocol.SpawnParams{SkipSetup: true})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, c, s.ID, "true")
	if _, err := os.Stat(filepath.Join(s.WorktreePath, "setup-ran")); err == nil {
		t.Fatal("skipSetup ran the setup")
	}
	if _, err := os.Stat(filepath.Join(s.WorktreePath, ".env")); err != nil {
		t.Fatal("skipSetup should still copy the files")
	}
}

func TestWorktreeCopyOffer(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	repo := tempRepo(t)
	os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env*\n"), 0o644)
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-qm", "ignore env")
	os.WriteFile(filepath.Join(repo, ".env"), []byte("A=1\n"), 0o600)
	os.WriteFile(filepath.Join(repo, ".env.local"), []byte("B=1\n"), 0o600)
	proj, err := c.ProjectAdd(ctx, repo, "offer")
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(info.SuggestCopy, []string{".env", ".env.local"}) {
		t.Fatalf("suggestCopy = %v", info.SuggestCopy)
	}
	if _, err := os.Stat(filepath.Join(info.WorktreePath, ".env")); err == nil {
		t.Fatal("copied without a config")
	}
	res, err := c.WorktreeCopy(ctx, protocol.WorktreeCopyParams{ID: info.WorktreeID, Files: info.SuggestCopy, Save: true})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Copied, []string{".env", ".env.local"}) || res.Created != filepath.Join(repo, ".panal.yaml") {
		t.Fatalf("copy = %+v", res)
	}
	// The next worktree gets them by itself, and no offer.
	next, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.SuggestCopy) != 0 {
		t.Fatalf("offer with a config: %v", next.SuggestCopy)
	}
	if got, _ := os.ReadFile(filepath.Join(next.WorktreePath, ".env.local")); string(got) != "B=1\n" {
		t.Fatalf(".env.local = %q", got)
	}
}

// A failed setup says so and waits: Enter starts the agent anyway.
func TestWorktreeSetupFails(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	repo := tempRepo(t)
	os.WriteFile(filepath.Join(repo, ".panal.yaml"), []byte("worktree:\n  setup: exit 3\n"), 0o644)
	proj, err := c.ProjectAdd(ctx, repo, "fails")
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree, TrustSetup: true})
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, info.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool {
		return bytes.Contains(out, []byte("the setup failed (exit 3)"))
	})
	if err := st.Input([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	// The agent (a shell) starts anyway: wait for its prompt before typing.
	// runIn types on another connection, whose input may reach the
	// terminal before this Enter and be eaten by the setup's read.
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte("$ ")) })
	runIn(t, c, info.ID, "true")
}
