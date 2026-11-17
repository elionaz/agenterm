package gitx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/apierr"
)

func TestCompareURL(t *testing.T) {
	gh := "https://github.com/o/r/compare/main...agent/x?expand=1"
	gl := "https://gitlab.com/g/sub/r/-/merge_requests/new?merge_request[source_branch]=agent%2Fx&merge_request[target_branch]=main"
	cases := []struct {
		remote, base, branch, want string
	}{
		{"https://github.com/o/r.git", "main", "agent/x", gh},
		{"https://github.com/o/r", "main", "agent/x", gh},
		{"https://user@github.com/o/r.git/", "main", "agent/x", gh},
		{"git@github.com:o/r.git", "main", "agent/x", gh},
		{"git@github.com:o/r", "main", "agent/x", gh},
		{"ssh://git@github.com/o/r", "main", "agent/x", gh},
		{"ssh://git@github.com:22/o/r.git", "main", "agent/x", gh},
		{"git@GitHub.com:o/r.git", "main", "agent/x", gh},
		{"git@github.com:o/r.git", "release/1.0", "feat#1 é", "https://github.com/o/r/compare/release/1.0...feat%231%20%C3%A9?expand=1"},
		{"git@github.com:o/r.git", "", "agent/x", "https://github.com/o/r/compare/agent/x?expand=1"},
		{"https://github.com/o", "main", "agent/x", ""}, // not owner/repo
		{"git@gitlab.com:g/sub/r.git", "main", "agent/x", gl},
		{"https://gitlab.com/g/sub/r.git", "main", "agent/x", gl},
		{"ssh://git@gitlab.example.com:2222/team/r.git", "dev", "b", "https://gitlab.example.com/team/r/-/merge_requests/new?merge_request[source_branch]=b&merge_request[target_branch]=dev"},
		{"https://gitlab.example.com:8443/team/r", "dev", "b", "https://gitlab.example.com:8443/team/r/-/merge_requests/new?merge_request[source_branch]=b&merge_request[target_branch]=dev"},
		{"git@bitbucket.org:o/r.git", "main", "b", ""},
		{"/srv/git/r.git", "main", "b", ""},
		{"file:///srv/git/r.git", "main", "b", ""},
		{"", "main", "b", ""},
	}
	for _, c := range cases {
		if got := CompareURL(c.remote, c.base, c.branch); got != c.want {
			t.Errorf("CompareURL(%q, %q, %q) = %q, want %q", c.remote, c.base, c.branch, got, c.want)
		}
	}
}

func TestChooseRemote(t *testing.T) {
	cases := []struct {
		upstream string
		remotes  []string
		want     string
	}{
		{"fork", []string{"origin", "fork"}, "fork"},
		{"", []string{"upstream", "origin"}, "origin"},
		{"gone", []string{"origin"}, "origin"},
		{".", []string{"origin"}, "origin"},
		{"", []string{"only"}, "only"},
		{"", []string{"a", "b"}, ""},
		{"", nil, ""},
	}
	for _, c := range cases {
		if got := chooseRemote(c.upstream, c.remotes); got != c.want {
			t.Errorf("chooseRemote(%q, %v) = %q, want %q", c.upstream, c.remotes, got, c.want)
		}
	}
}

func TestPullRequest(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	wt := filepath.Join(canon(t.TempDir()), "s1")
	if err := AddWorktree(c, repo, wt, "agent/pr", "main"); err != nil {
		t.Fatal(err)
	}
	write(t, wt, "feature.txt", "f\n")
	sh(t, wt, "add", "-A")
	sh(t, wt, "commit", "-q", "-m", "feature")

	// No remote yet.
	if _, err := PullRequest(c, repo, "agent/pr", "main", "", wt, false); err == nil || code(err) != apierr.PRNoRemote {
		t.Fatalf("no-remote pr: %v", err)
	}

	bare := filepath.Join(canon(t.TempDir()), "remote.git")
	sh(t, repo, "init", "-q", "--bare", "-b", "main", bare)
	sh(t, repo, "remote", "add", "origin", bare)
	sh(t, repo, "push", "-q", "origin", "main")

	// Several remotes and no origin: ambiguous.
	sh(t, repo, "remote", "rename", "origin", "a")
	sh(t, repo, "remote", "add", "b", bare)
	if _, err := PullRequest(c, repo, "agent/pr", "main", "", wt, true); err == nil || code(err) != apierr.PRNoRemote || !strings.Contains(err.Error(), "a, b") {
		t.Fatalf("ambiguous-remote pr: %v", err)
	}
	sh(t, repo, "remote", "remove", "b")
	sh(t, repo, "remote", "rename", "a", "origin")

	write(t, wt, "wip.txt", "uncommitted\n")
	dry, err := PullRequest(c, repo, "agent/pr", "main", "", wt, true)
	if err != nil || dry.Pushed || dry.Remote != "origin" || dry.Commits != 1 || dry.Uncommitted != 1 || dry.URL != "" {
		t.Fatalf("dry pr = %+v, %v", dry, err)
	}
	if out := sh(t, bare, "branch", "--list", "agent/pr"); out != "" {
		t.Fatalf("dry run pushed: %q", out)
	}

	res, err := PullRequest(c, wt, "agent/pr", "main", "", wt, false) // any dir of the repo works
	if err != nil || !res.Pushed || res.Remote != "origin" || res.Commits != 1 || res.Uncommitted != 1 {
		t.Fatalf("pr = %+v, %v", res, err)
	}
	if got, want := sh(t, bare, "rev-parse", "refs/heads/agent/pr"), sh(t, wt, "rev-parse", "HEAD"); got != want {
		t.Fatalf("remote branch at %s, want %s", got, want)
	}
	if up := sh(t, repo, "config", "--get", "branch.agent/pr.remote"); up != "origin" {
		t.Fatalf("upstream not set: %q", up)
	}

	// Rejected pushes report git's message (not forced).
	write(t, wt, "more.txt", "m\n")
	sh(t, wt, "add", "more.txt")
	sh(t, wt, "commit", "-q", "-m", "more")
	other := filepath.Join(canon(t.TempDir()), "other")
	sh(t, repo, "clone", "-q", bare, other)
	sh(t, other, "config", "user.name", "T")
	sh(t, other, "config", "user.email", "t@example.com")
	sh(t, other, "checkout", "-q", "agent/pr")
	write(t, other, "x.txt", "x\n")
	sh(t, other, "add", "x.txt")
	sh(t, other, "commit", "-q", "-m", "diverge")
	sh(t, other, "push", "-q", "origin", "agent/pr")
	if _, err := PullRequest(c, repo, "agent/pr", "main", "", wt, false); err == nil || code(err) != apierr.PushFailed || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("diverged push: %v", err)
	}

	// The upstream remote wins over origin; GitHub URLs are built from it.
	sh(t, repo, "remote", "add", "fork", "git@github.com:me/r.git")
	sh(t, repo, "config", "branch.agent/pr.remote", "fork")
	dry, err = PullRequest(c, repo, "agent/pr", "main", "", wt, true)
	if err != nil || dry.Remote != "fork" || dry.URL != "https://github.com/me/r/compare/main...agent/pr?expand=1" {
		t.Fatalf("fork dry pr = %+v, %v", dry, err)
	}

	// A branch with nothing ahead of its base is refused.
	if err := AddWorktree(c, repo, filepath.Join(canon(t.TempDir()), "s2"), "agent/empty", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := PullRequest(c, repo, "agent/empty", "main", "", "", false); err == nil || code(err) != apierr.PRNothing {
		t.Fatalf("empty pr: %v", err)
	}
	if _, err := PullRequest(c, repo, "agent/gone", "main", "", "", false); err == nil || code(err) != apierr.BranchMissing {
		t.Fatalf("missing branch pr: %v", err)
	}
}
