package gitx

import (
	"context"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/elionaz/agenterm/internal/apierr"
)

// PRResult describes a pull request preparation: the branch pushed to
// Remote and, for known hosts, the page that opens the pull (merge)
// request. With DryRun nothing is pushed (Pushed false) but Remote and
// URL are what a real run would use.
type PRResult struct {
	Branch  string `json:"branch"`
	Base    string `json:"base"`
	Remote  string `json:"remote"`
	Pushed  bool   `json:"pushed"`
	URL     string `json:"url,omitempty"` // empty: unknown host (push only)
	Commits int    `json:"commits"`
	Output  string `json:"output,omitempty"`
	// Uncommitted: changes left in the worktree that were not pushed.
	Uncommitted int `json:"uncommitted,omitempty"`
}

// chooseRemote picks the remote to push to: the branch's upstream remote,
// else origin, else the only remote ("" if none fits).
func chooseRemote(upstream string, remotes []string) string {
	has := func(r string) bool {
		for _, x := range remotes {
			if x == r {
				return true
			}
		}
		return false
	}
	if upstream != "" && upstream != "." && has(upstream) {
		return upstream
	}
	if has("origin") {
		return "origin"
	}
	if len(remotes) == 1 {
		return remotes[0]
	}
	return ""
}

// PushRemote is the remote branch is pushed to (see chooseRemote). It
// fails with apierr.PRNoRemote when the repository has no usable remote.
func PushRemote(ctx context.Context, dir, branch string) (string, error) {
	out, err := git(ctx, dir, "remote")
	if err != nil {
		return "", err
	}
	remotes := splitLines(out)
	upstream, _ := git(ctx, dir, "config", "--get", "branch."+branch+".remote")
	if r := chooseRemote(strings.TrimSpace(upstream), remotes); r != "" {
		return r, nil
	}
	if len(remotes) == 0 {
		return "", apierr.New(apierr.PRNoRemote, map[string]any{"branch": branch, "remotes": remotes},
			"the repository has no remote to push %s to: add one (git remote add origin <url>) and try again", branch)
	}
	return "", apierr.New(apierr.PRNoRemote, map[string]any{"branch": branch, "remotes": remotes},
		"cannot choose a remote for %s among %s (no upstream and no origin): set its upstream and try again", branch, strings.Join(remotes, ", "))
}

// PushBranch runs `git push -u <remote> <branch>` (never forced, never
// interactive: credential and ssh prompts fail instead of waiting). A
// failure is apierr.PushFailed with git's stderr.
func PushBranch(ctx context.Context, dir, remote, branch string) (string, error) {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		if c, _ := git(ctx, dir, "config", "--get", "core.sshCommand"); c == "" {
			env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
		}
	}
	ref := "refs/heads/" + branch
	out, _, err := run(ctx, dir, runOpts{write: true, env: env}, "push", "--porcelain", "-u", remote, ref+":"+ref)
	if err != nil {
		stderr := err.Error()
		var ge *Error
		if errors.As(err, &ge) && strings.TrimSpace(ge.Stderr) != "" {
			stderr = strings.TrimSpace(ge.Stderr)
		}
		return "", apierr.New(apierr.PushFailed, map[string]any{"remote": remote, "branch": branch, "stderr": stderr},
			"pushing %s to %s failed: %s", branch, remote, stderr)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// RemoteURL is the fetch URL of remote.
func RemoteURL(ctx context.Context, dir, remote string) (string, error) {
	return git(ctx, dir, "remote", "get-url", "--", remote)
}

var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^:/]+):(.+)$`)

// parseRemote splits a remote URL into the web host and repository path
// (https and ssh URLs, scp-like git@host:path). The port is kept only for
// http(s) URLs: an ssh port says nothing about the web server.
func parseRemote(remote string) (host, path string, ok bool) {
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return "", "", false
		}
		switch u.Scheme {
		case "https", "http":
			host = u.Host
		case "ssh", "git", "git+ssh", "ssh+git":
			host = u.Hostname()
		default:
			return "", "", false
		}
		path = u.Path
	} else if m := scpLike.FindStringSubmatch(remote); m != nil {
		host, path = m[1], m[2]
	} else {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", false
	}
	return strings.ToLower(host), path, true
}

// escapeRef escapes a branch name for a URL path, keeping its slashes.
func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// CompareURL is the web page that opens a pull request of branch into base
// on the host of remoteURL: GitHub (compare view) and GitLab (new merge
// request; gitlab.com or any host whose name contains "gitlab"). Other
// hosts give "".
func CompareURL(remoteURL, base, branch string) string {
	host, path, ok := parseRemote(remoteURL)
	if !ok || branch == "" {
		return ""
	}
	switch {
	case host == "github.com" || host == "www.github.com":
		if strings.Count(path, "/") != 1 {
			return ""
		}
		spec := escapeRef(branch)
		if base != "" {
			spec = escapeRef(base) + "..." + spec
		}
		return "https://github.com/" + path + "/compare/" + spec + "?expand=1"
	case strings.Contains(host, "gitlab"):
		q := "merge_request[source_branch]=" + url.QueryEscape(branch)
		if base != "" {
			q += "&merge_request[target_branch]=" + url.QueryEscape(base)
		}
		return "https://" + host + "/" + path + "/-/merge_requests/new?" + q
	}
	return ""
}

// PullRequest pushes branch to its remote (see PushRemote) and returns the
// page that opens a pull request into base. aheadOf is the ref commits are
// counted against (base, or its commit when the branch is gone); with no
// commits ahead it refuses (apierr.PRNothing). worktree (optional) is the
// branch's worktree, to report changes that were not committed (and so
// are not pushed). dryRun reports remote and URL without pushing.
func PullRequest(ctx context.Context, repo, branch, base, aheadOf, worktree string, dryRun bool) (PRResult, error) {
	res := PRResult{Branch: branch, Base: base}
	if !BranchExists(ctx, repo, branch) {
		return res, apierr.New(apierr.BranchMissing, map[string]any{"branch": branch}, "branch %q no longer exists", branch)
	}
	if worktree != "" {
		if wst, err := GetStatus(ctx, worktree); err == nil {
			res.Uncommitted = wst.Changed()
		}
	}
	if aheadOf == "" {
		aheadOf = base
	}
	if aheadOf != "" {
		n, err := AheadCount(ctx, repo, aheadOf, "refs/heads/"+branch)
		if err != nil {
			return res, err
		}
		res.Commits = n
	}
	if res.Commits == 0 {
		return res, apierr.New(apierr.PRNothing, map[string]any{"branch": branch, "base": base},
			"nothing to open a pull request for: %s has no commits that are not already in %s", branch, base)
	}
	remote, err := PushRemote(ctx, repo, branch)
	if err != nil {
		return res, err
	}
	res.Remote = remote
	if u, err := RemoteURL(ctx, repo, remote); err == nil {
		res.URL = CompareURL(u, base, branch)
	}
	if dryRun {
		return res, nil
	}
	out, err := PushBranch(ctx, repo, remote, branch)
	if err != nil {
		return res, err
	}
	res.Pushed = true
	res.Output = out
	return res, nil
}
