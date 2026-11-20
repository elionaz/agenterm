package gitx

import (
	"context"
	"strconv"
	"strings"
)

// Upstream is the full ref of the remote-tracking branch that local
// branch follows (refs/remotes/origin/main): its configured upstream,
// else origin/<branch> when that exists; "" when there is none.
func Upstream(ctx context.Context, dir, branch string) string {
	if branch == "" || !BranchExists(ctx, dir, branch) {
		return ""
	}
	// for-each-ref, not rev-parse <branch>@{upstream}: refs/heads/x@{upstream}
	// is not valid syntax, and a bare x could name a tag.
	if out, err := git(ctx, dir, "for-each-ref", "--format=%(upstream)", "refs/heads/"+branch); err == nil {
		if ref := strings.TrimSpace(out); strings.HasPrefix(ref, "refs/remotes/") {
			return ref
		}
	}
	ref := "refs/remotes/origin/" + branch
	if _, err := git(ctx, dir, "show-ref", "--verify", "--quiet", ref); err == nil {
		return ref
	}
	return ""
}

// ShortRef drops the refs/remotes/ or refs/heads/ prefix of a ref.
func ShortRef(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, "refs/remotes/"), "refs/heads/")
}

// IsAncestor reports whether commit a is an ancestor of (or equal to) b.
func IsAncestor(ctx context.Context, dir, a, b string) bool {
	_, err := git(ctx, dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// FreshestBase is what head should be compared with: local branch base,
// or its upstream when that is more recent from head's point of view
// (head's merge base with the upstream descends from the one with base).
// That happens when pull requests were merged on the host and the local
// base was never pulled, while head already contains the newer commits:
// against the stale local base, those commits would show as head's own.
func FreshestBase(ctx context.Context, dir, base, head string) string {
	up := Upstream(ctx, dir, base)
	if up == "" {
		return base
	}
	mbUp, err := MergeBase(ctx, dir, up, head)
	if err != nil {
		return base
	}
	mbLocal, err := MergeBase(ctx, dir, base, head)
	if err != nil {
		return up
	}
	if mbLocal != mbUp && IsAncestor(ctx, dir, mbLocal, mbUp) {
		return up
	}
	return base
}

// maxMergedPaths bounds the pathspec AlreadyIn passes to git diff.
const maxMergedPaths = 2000

// AlreadyIn reports whether the commits from..head are already in base by
// content: head changed some files since from, and those files are the
// same in base as in head. That is how a branch looks after its pull
// request was squash- or rebase-merged: its commits are not ancestors of
// base, yet base holds their changes. A file base changed again after the
// merge makes it false (the branch then still shows its commits).
func AlreadyIn(ctx context.Context, dir, base, from, head string) bool {
	out, _, err := run(ctx, dir, runOpts{}, "diff", "--no-ext-diff", "--name-only", "-z", from, head, "--")
	if err != nil {
		return false
	}
	files := splitZ(out)
	if len(files) == 0 || len(files) > maxMergedPaths {
		return false
	}
	args := append([]string{"diff", "--no-ext-diff", "--quiet", base, head, "--"}, files...)
	_, _, err = run(ctx, dir, runOpts{}, args...)
	return err == nil
}

// SplitRemoteRef splits refs/remotes/<remote>/<branch> (remote names
// without slashes); ok is false for any other ref.
func SplitRemoteRef(ref string) (remote, branch string, ok bool) {
	rest, found := strings.CutPrefix(ref, "refs/remotes/")
	if !found {
		return "", "", false
	}
	remote, branch, ok = strings.Cut(rest, "/")
	return remote, branch, ok && remote != "" && branch != ""
}

// Fetch updates the remote-tracking ref of one branch from remote. It
// never prompts (GIT_TERMINAL_PROMPT=0, no terminal for ssh): a missing
// credential fails instead.
func Fetch(ctx context.Context, dir, remote, branch string) error {
	_, _, err := run(ctx, dir, runOpts{write: true},
		"fetch", "--quiet", "--no-tags", "--no-write-fetch-head", remote, branch)
	return err
}

// maxReflog bounds how much of a worktree's HEAD reflog VisitedBranches reads.
const maxReflog = 500

// VisitedBranches lists the branches dir's HEAD was checked out on, newest
// first and without duplicates, from its reflog ("checkout: moving from A
// to B"; each worktree has its own). Commit hashes (a detached HEAD) are
// left out; branches deleted since are kept (their pull requests still
// exist).
func VisitedBranches(ctx context.Context, dir string) []string {
	out, err := git(ctx, dir, "reflog", "show", "--format=%gs", "-n", strconv.Itoa(maxReflog), "HEAD", "--")
	if err != nil {
		return nil
	}
	var list []string
	seen := map[string]bool{}
	add := func(b string) {
		b = strings.TrimSpace(b)
		if b == "" || b == "HEAD" || seen[b] || isHash(b) || strings.HasPrefix(b, "refs/") {
			return
		}
		seen[b] = true
		list = append(list, b)
	}
	for _, l := range splitLines(out) {
		rest, ok := strings.CutPrefix(l, "checkout: moving from ")
		if !ok {
			continue
		}
		from, to, ok := strings.Cut(rest, " to ")
		if !ok {
			continue
		}
		add(to)
		add(from)
	}
	return list
}

func isHash(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
