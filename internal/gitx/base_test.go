package gitx

import (
	"path/filepath"
	"testing"

	"github.com/elionaz/agenterm/internal/apierr"
)

// hostRepo makes repo's main track a bare origin and returns a clone of
// it that plays the code host: commits pushed from there are pull
// requests merged on the host.
func hostRepo(t *testing.T, repo string) string {
	t.Helper()
	bare := filepath.Join(canon(t.TempDir()), "remote.git")
	sh(t, repo, "init", "-q", "--bare", "-b", "main", bare)
	sh(t, repo, "remote", "add", "origin", bare)
	sh(t, repo, "push", "-q", "-u", "origin", "main")
	host := filepath.Join(canon(t.TempDir()), "host")
	sh(t, repo, "clone", "-q", bare, host)
	sh(t, host, "config", "user.name", "Host")
	sh(t, host, "config", "user.email", "host@example.com")
	sh(t, host, "config", "commit.gpgsign", "false")
	return host
}

func hostCommit(t *testing.T, host, file, content, msg string) {
	t.Helper()
	write(t, host, file, content)
	sh(t, host, "add", "-A")
	sh(t, host, "commit", "-q", "-m", msg)
	sh(t, host, "push", "-q", "origin", "main")
}

func TestFreshestBaseUsesUpstreamWhenLocalIsBehind(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	host := hostRepo(t, repo)
	if got := Upstream(c, repo, "main"); got != "refs/remotes/origin/main" {
		t.Fatalf("Upstream = %q", got)
	}
	if got := Upstream(c, repo, "nope"); got != "" {
		t.Fatalf("Upstream(missing) = %q", got)
	}
	// Up to date: the local branch is the base.
	sh(t, repo, "branch", "early", "main")
	if got := FreshestBase(c, repo, "main", "refs/heads/early"); got != "main" {
		t.Fatalf("FreshestBase(up to date) = %q, want main", got)
	}

	// Pull requests merged on the host; the local main is never pulled,
	// and a new branch starts from origin/main.
	hostCommit(t, host, "pr1.txt", "one\n", "pr 1")
	hostCommit(t, host, "pr2.txt", "two\n", "pr 2")
	if err := Fetch(c, repo, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	sh(t, repo, "branch", "feat", "origin/main")
	sh(t, repo, "checkout", "-q", "feat")
	write(t, repo, "feat.txt", "mine\n")
	sh(t, repo, "add", "-A")
	sh(t, repo, "commit", "-q", "-m", "feat")
	sh(t, repo, "checkout", "-q", "main")

	base := FreshestBase(c, repo, "main", "refs/heads/feat")
	if base != "refs/remotes/origin/main" {
		t.Fatalf("FreshestBase(behind) = %q, want refs/remotes/origin/main", base)
	}
	if n, _ := AheadCount(c, repo, base, "refs/heads/feat"); n != 1 {
		t.Fatalf("ahead of %s = %d, want 1 (against the stale main: 3)", base, n)
	}
	// A branch made from the stale local main still compares with it.
	if got := FreshestBase(c, repo, "main", "refs/heads/early"); got != "main" {
		t.Fatalf("FreshestBase(old branch) = %q, want main", got)
	}

	// Merging locally into the stale main is refused.
	_, err := Merge(c, repo, "feat", "main", "")
	if code(err) != apierr.MergeBaseBehind {
		t.Fatalf("Merge into a stale base = %v, want %s", err, apierr.MergeBaseBehind)
	}
	sh(t, repo, "merge", "-q", "--ff-only", "origin/main")
	if _, err := Merge(c, repo, "feat", "main", ""); err != nil {
		t.Fatalf("Merge after pulling: %v", err)
	}
}

func TestAlreadyInSquashMerged(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	host := hostRepo(t, repo)
	sh(t, repo, "checkout", "-q", "-b", "sq")
	write(t, repo, "a.txt", "one\ntwo\nthree\nfour\n")
	sh(t, repo, "add", "-A")
	sh(t, repo, "commit", "-q", "-m", "step 1")
	write(t, repo, "new.txt", "new\n")
	sh(t, repo, "add", "-A")
	sh(t, repo, "commit", "-q", "-m", "step 2")
	sh(t, repo, "checkout", "-q", "main")
	from := sh(t, repo, "merge-base", "main", "sq")
	if AlreadyIn(c, repo, "refs/remotes/origin/main", from, "refs/heads/sq") {
		t.Fatal("AlreadyIn before the merge")
	}
	if AlreadyIn(c, repo, "main", "refs/heads/main", "refs/heads/main") {
		t.Fatal("AlreadyIn with no commits")
	}

	// The host squash-merges it (one new commit with the same content)
	// and someone else merges an unrelated change.
	write(t, host, "new.txt", "new\n")
	hostCommit(t, host, "a.txt", "one\ntwo\nthree\nfour\n", "squashed sq")
	hostCommit(t, host, "other.txt", "other\n", "unrelated")
	if err := Fetch(c, repo, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	up := "refs/remotes/origin/main"
	from = sh(t, repo, "merge-base", up, "sq")
	if !AlreadyIn(c, repo, up, from, "refs/heads/sq") {
		t.Fatal("AlreadyIn after a squash merge = false")
	}
	// The base changes one of those files again: no longer a match.
	hostCommit(t, host, "a.txt", "changed later\n", "later")
	if err := Fetch(c, repo, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if AlreadyIn(c, repo, up, from, "refs/heads/sq") {
		t.Fatal("AlreadyIn after the base changed the file again = true")
	}
}

func TestSplitRemoteRef(t *testing.T) {
	for ref, want := range map[string][2]string{
		"refs/remotes/origin/main": {"origin", "main"},
		"refs/remotes/up/feat/x":   {"up", "feat/x"},
		"refs/heads/main":          {"", ""},
		"refs/remotes/origin":      {"", ""},
		"":                         {"", ""},
	} {
		r, b, ok := SplitRemoteRef(ref)
		if ok != (want[0] != "") || r != want[0] && ok || b != want[1] && ok {
			t.Errorf("SplitRemoteRef(%q) = %q, %q, %v; want %v", ref, r, b, ok, want)
		}
	}
	if got := ShortRef("refs/remotes/origin/main"); got != "origin/main" {
		t.Errorf("ShortRef = %q", got)
	}
}
