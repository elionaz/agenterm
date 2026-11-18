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

// chooseRemote picks the remote to push to, in git's order: the first of
// candidates (branch.<b>.pushRemote, remote.pushDefault, branch.<b>.remote)
// that is a configured remote, else origin, else the only remote ("" if
// none fits).
func chooseRemote(candidates, remotes []string) string {
	has := func(r string) bool {
		for _, x := range remotes {
			if x == r {
				return true
			}
		}
		return false
	}
	for _, c := range candidates {
		if c != "" && c != "." && has(c) {
			return c
		}
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
	var candidates []string
	for _, key := range []string{"branch." + branch + ".pushRemote", "remote.pushDefault", "branch." + branch + ".remote"} {
		v, _ := git(ctx, dir, "config", "--get", key)
		candidates = append(candidates, strings.TrimSpace(v))
	}
	if r := chooseRemote(candidates, remotes); r != "" {
		return r, nil
	}
	if len(remotes) == 0 {
		return "", apierr.New(apierr.PRNoRemote, map[string]any{"branch": branch, "remotes": remotes},
			"the repository has no remote to push %s to: add one (git remote add origin <url>) and try again", branch)
	}
	return "", apierr.New(apierr.PRNoRemote, map[string]any{"branch": branch, "remotes": remotes},
		"cannot choose a remote for %s among %s (no upstream and no origin): set its upstream and try again", branch, strings.Join(remotes, ", "))
}

// sshPushCommand makes ssh fail instead of prompting, and give up on an
// unreachable or stalled server.
const sshPushCommand = "ssh -o BatchMode=yes -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=3"

// PushBranch runs `git push -u <remote> <branch>` (never forced, never
// interactive: credential and ssh prompts fail instead of waiting; stalled
// transfers are dropped). A failure is apierr.PushFailed with git's stderr.
func PushBranch(ctx context.Context, dir, remote, branch string) (string, error) {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"}
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		if c, _ := git(ctx, dir, "config", "--get", "core.sshCommand"); c == "" {
			env = append(env, "GIT_SSH_COMMAND="+sshPushCommand)
		}
	}
	ref := "refs/heads/" + branch
	out, _, err := run(ctx, dir, runOpts{write: true, env: env},
		"-c", "http.lowSpeedLimit=1000", "-c", "http.lowSpeedTime=30",
		"push", "--porcelain", "-u", "--", remote, ref+":"+ref)
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

// remoteInfo is a remote URL split for building web links.
type remoteInfo struct {
	scheme string // web scheme: http for http remotes, else https
	host   string // lower case; with port only for http(s) remotes
	path   string // repository path, without .git
	ssh    bool   // ssh or scp-like (host may be an ssh config alias)
}

// parseRemote splits a remote URL (https and ssh URLs, scp-like
// git@host:path). The port is kept only for http(s) URLs: an ssh port says
// nothing about the web server.
func parseRemote(remote string) (remoteInfo, bool) {
	ri := remoteInfo{scheme: "https"}
	remote = strings.TrimSpace(remote)
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return ri, false
		}
		switch u.Scheme {
		case "https", "http":
			ri.scheme, ri.host = u.Scheme, u.Host
		case "ssh", "git", "git+ssh", "ssh+git":
			ri.host, ri.ssh = u.Hostname(), true
		default:
			return ri, false
		}
		ri.path = u.Path
	} else if m := scpLike.FindStringSubmatch(remote); m != nil {
		ri.host, ri.path, ri.ssh = m[1], m[2], true
	} else {
		return ri, false
	}
	ri.path = strings.Trim(strings.TrimSuffix(strings.Trim(ri.path, "/"), ".git"), "/")
	ri.host = strings.ToLower(ri.host)
	return ri, ri.path != ""
}

// isGitHub: github.com, ssh.github.com (ssh over port 443) and, for ssh
// remotes, config aliases like github.com-work.
func (ri remoteInfo) isGitHub() bool {
	switch ri.host {
	case "github.com", "www.github.com", "ssh.github.com":
		return true
	}
	return ri.ssh && strings.HasPrefix(ri.host, "github.com-")
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
	ri, ok := parseRemote(remoteURL)
	if !ok || branch == "" {
		return ""
	}
	switch {
	case ri.isGitHub():
		if strings.Count(ri.path, "/") != 1 {
			return ""
		}
		spec := escapeRef(branch)
		if base != "" {
			spec = escapeRef(base) + "..." + spec
		}
		return "https://github.com/" + ri.path + "/compare/" + spec + "?expand=1"
	case strings.Contains(ri.host, "gitlab"):
		q := "merge_request[source_branch]=" + url.QueryEscape(branch)
		if base != "" {
			q += "&merge_request[target_branch]=" + url.QueryEscape(base)
		}
		return ri.scheme + "://" + ri.host + "/" + ri.path + "/-/merge_requests/new?" + q
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

// Remote is a remote URL as a code host sees it (see RemoteOf).
type Remote struct {
	Host string // lower case (with port for http(s) remotes)
	Path string // owner/repo or group/subgroup/project, without .git
	// Web is the repository page (https://host/path).
	Web    string
	GitHub bool // github.com (ssh aliases like github.com-work included)
	GitLab bool // gitlab.com or a host whose name contains "gitlab"
}

// RemoteOf splits a remote URL (https, ssh or scp-like) for the code host
// integrations; ok is false when it does not parse.
func RemoteOf(remoteURL string) (Remote, bool) {
	ri, ok := parseRemote(remoteURL)
	if !ok {
		return Remote{}, false
	}
	host := ri.host
	if ri.isGitHub() {
		host = "github.com"
	}
	return Remote{
		Host: host, Path: ri.path, Web: ri.scheme + "://" + host + "/" + ri.path,
		GitHub: ri.isGitHub(), GitLab: !ri.isGitHub() && strings.Contains(ri.host, "gitlab"),
	}, true
}

// RemoteDefaultBranch is the branch remote's HEAD points to (refs/remotes/
// <remote>/HEAD, set by clone or `git remote set-head`), else main or
// master if they exist locally; "" when none is known.
func RemoteDefaultBranch(ctx context.Context, dir, remote string) string {
	if remote != "" {
		if out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
			if b := strings.TrimPrefix(strings.TrimSpace(out), remote+"/"); b != "" {
				return b
			}
		}
	}
	for _, b := range []string{"main", "master"} {
		if BranchExists(ctx, dir, b) {
			return b
		}
	}
	return ""
}
