package daemon

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/forge"
	"github.com/elionaz/agenterm/internal/gitx"
	"github.com/elionaz/agenterm/internal/protocol"
)

// Pull requests on the code host (forge.*), for the branch of a worktree
// or of a session's folder (the main folder included, on whatever branch
// it is). The host is reached through its CLI and the user's own login
// (internal/forge); without one, forge.status says why (Unavailable) and
// the app falls back to pushing and opening the host's page.

// forgeTarget is the branch a forge.* call is about.
type forgeTarget struct {
	g         sessGit
	dir       string
	branch    string
	base      string
	baseRef   string // what commits ahead are counted against
	remote    string
	remoteURL string
}

// unavailableCodes are the refusals forge.status reports as Unavailable
// (the integration is off for this branch) rather than as errors.
var unavailableCodes = map[string]bool{
	apierr.ForgeUnsupported: true, apierr.ForgeCLIMissing: true, apierr.ForgeNotAuthed: true,
	apierr.ForgeOnBase: true, apierr.PRNoRemote: true, apierr.NotARepo: true,
}

func (s *Server) forgeTarget(ctx context.Context, ref protocol.ForgeRef) (forgeTarget, error) {
	var t forgeTarget
	switch {
	case ref.WorktreeID != "":
		rec, err := s.wts.Get(ref.WorktreeID)
		if err != nil {
			return t, err
		}
		t.g = recGit(rec)
	case ref.SessionID != "":
		sg := s.sessionGit(ref.SessionID)
		if sg == nil {
			if _, err := s.lookup(ref.SessionID); err != nil {
				return t, err
			}
			return t, errors.New("the session has no git information")
		}
		t.g = *sg
	default:
		return t, errors.New("missing sessionId or worktreeId")
	}
	t.dir = t.g.Dir
	if _, err := os.Stat(t.dir); err != nil {
		return t, apierr.New(apierr.DirMissing, map[string]any{"dir": t.dir}, "the folder no longer exists: %s", t.dir)
	}
	if !gitx.IsRepo(ctx, t.dir) {
		return t, apierr.New(apierr.NotARepo, map[string]any{"dir": t.dir}, "%s is not a git repository", t.dir)
	}
	t.branch = t.g.Branch
	if t.g.Mode != protocol.ModeWorktree {
		t.branch, _ = gitx.CurrentBranch(ctx, t.dir)
	}
	if t.branch == "" {
		return t, apierr.New(apierr.ForgeOnBase, map[string]any{"branch": ""}, "the folder is on a detached HEAD: there is no branch to show a pull request for")
	}
	remote, err := gitx.PushRemote(ctx, t.dir, t.branch)
	if err != nil {
		return t, err
	}
	t.remote = remote
	if t.remoteURL, err = gitx.RemoteURL(ctx, t.dir, remote); err != nil {
		return t, err
	}
	t.remoteURL = strings.TrimSpace(t.remoteURL)
	if t.g.Mode == protocol.ModeWorktree {
		t.base = t.g.Base
		t.baseRef = baseRef(ctx, t.dir, t.g.Base, t.g.BaseCommit)
	} else {
		t.base = gitx.RemoteDefaultBranch(ctx, t.dir, remote)
		t.baseRef = t.base
		// The remote's copy is what a pull request compares against.
		if t.base != "" {
			if _, err := gitx.ResolveCommit(ctx, t.dir, remote+"/"+t.base); err == nil {
				t.baseRef = remote + "/" + t.base
			}
		}
	}
	if t.branch == t.base {
		return t, apierr.New(apierr.ForgeOnBase, map[string]any{"branch": t.branch},
			"the folder is on %s, the base branch: there is no pull request to show", t.branch)
	}
	return t, nil
}

func (s *Server) forgeFor(t forgeTarget) (forge.Forge, error) {
	return forge.For(t.remoteURL, t.dir, s.cfg.SearchPath())
}

// unavailable turns an integration refusal into a ForgeStatus; other
// errors are returned as they are.
func unavailable(res protocol.ForgeStatus, err error) (protocol.ForgeStatus, error) {
	if code, data, ok := apierr.Code(err); ok && unavailableCodes[code] {
		res.Unavailable, res.Message, res.UnavailableData = code, err.Error(), data
		return res, nil
	}
	return res, err
}

func (s *Server) forgeStatus(ref protocol.ForgeRef) (protocol.ForgeStatus, error) {
	ctx, cancel := context.WithTimeout(s.bg, gitPushTO)
	defer cancel()
	return s.forgeStatusCtx(ctx, ref)
}

func (s *Server) forgeStatusCtx(ctx context.Context, ref protocol.ForgeRef) (protocol.ForgeStatus, error) {
	var res protocol.ForgeStatus
	t, err := s.forgeTarget(ctx, ref)
	res.Branch, res.Base = t.branch, t.base
	if err != nil {
		return unavailable(res, err)
	}
	if r, ok := gitx.RemoteOf(t.remoteURL); ok {
		res.Host = r.Host
	}
	f, err := s.forgeFor(t)
	if err != nil {
		return unavailable(res, err)
	}
	res.Kind, res.Tool = f.Kind(), f.Tool()
	pr, err := f.Status(ctx, t.branch)
	if err != nil {
		return unavailable(res, err)
	}
	res.PR = pr
	if pr != nil && pr.Base != "" {
		res.Base = pr.Base
	}
	if pr != nil && (pr.State == forge.StateOpen || pr.State == forge.StateDraft) {
		if res.MergeMethods, err = f.MergeMethods(ctx); err != nil {
			return res, err
		}
	}
	if t.baseRef != "" {
		if cs, err := gitx.CommitsAhead(ctx, t.dir, t.baseRef, "refs/heads/"+t.branch); err == nil {
			res.Ahead = len(cs)
			res.Title, res.Body = suggestPR(t.branch, cs)
		}
	}
	return res, nil
}

// suggestPR is a pull request title and body for the commits of branch:
// a single commit gives its subject; several give the branch name, and
// the body lists their subjects (oldest first).
func suggestPR(branch string, cs []gitx.Commit) (title, body string) {
	if len(cs) == 1 {
		return cs[0].Subject, ""
	}
	name := branch
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	title = strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(name))
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	var b strings.Builder
	for i := len(cs) - 1; i >= 0; i-- { // CommitsAhead lists newest first
		b.WriteString("- " + cs[i].Subject + "\n")
	}
	return title, strings.TrimRight(b.String(), "\n")
}

func (s *Server) forgeComments(p protocol.ForgeCommentsParams) ([]forge.Comment, error) {
	ctx, cancel := context.WithTimeout(s.bg, gitPushTO)
	defer cancel()
	t, err := s.forgeTarget(ctx, p.ForgeRef)
	if err != nil {
		return nil, err
	}
	f, err := s.forgeFor(t)
	if err != nil {
		return nil, err
	}
	return f.Comments(ctx, p.Number)
}

func (s *Server) forgeMerge(p protocol.ForgeMergeParams) (protocol.ForgeStatus, error) {
	ctx, cancel := context.WithTimeout(s.bg, gitPushTO)
	defer cancel()
	t, err := s.forgeTarget(ctx, p.ForgeRef)
	if err != nil {
		return protocol.ForgeStatus{}, err
	}
	f, err := s.forgeFor(t)
	if err != nil {
		return protocol.ForgeStatus{}, err
	}
	if err := f.Merge(ctx, p.Number, p.Method, t.branch, p.DeleteBranch); err != nil {
		return protocol.ForgeStatus{}, err
	}
	s.log.Info("pull request merged", "branch", t.branch, "number", p.Number, "method", p.Method, "tool", f.Tool())
	s.forgeChanged(t)
	return s.forgeStatusCtx(ctx, p.ForgeRef)
}

func (s *Server) forgeCreate(p protocol.ForgeCreateParams) (protocol.ForgeStatus, error) {
	ctx, cancel := context.WithTimeout(s.bg, gitPushTO)
	defer cancel()
	t, err := s.forgeTarget(ctx, p.ForgeRef)
	if err != nil {
		return protocol.ForgeStatus{}, err
	}
	f, err := s.forgeFor(t)
	if err != nil {
		return protocol.ForgeStatus{}, err
	}
	if t.base == "" {
		return protocol.ForgeStatus{}, apierr.New(apierr.BaseMissing, map[string]any{"base": ""}, "cannot tell which branch to open the pull request into")
	}
	if t.baseRef != "" {
		if n, err := gitx.AheadCount(ctx, t.dir, t.baseRef, "refs/heads/"+t.branch); err == nil && n == 0 {
			return protocol.ForgeStatus{}, apierr.New(apierr.PRNothing, map[string]any{"branch": t.branch, "base": t.base},
				"nothing to open a pull request for: %s has no commits that are not already in %s", t.branch, t.base)
		}
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = t.branch
	}
	if _, err := gitx.PushBranch(ctx, t.dir, t.remote, t.branch); err != nil {
		return protocol.ForgeStatus{}, err
	}
	if err := f.Create(ctx, forge.CreateParams{Head: t.branch, Base: t.base, Title: title, Body: p.Body, Draft: p.Draft}); err != nil {
		return protocol.ForgeStatus{}, err
	}
	s.log.Info("pull request created", "branch", t.branch, "base", t.base, "tool", f.Tool())
	s.forgeChanged(t)
	return s.forgeStatusCtx(ctx, p.ForgeRef)
}

// forgeChanged refreshes what a push or a merge on the host may change:
// the git info of the sessions on that branch, and the worktree list.
func (s *Server) forgeChanged(t forgeTarget) {
	for _, id := range s.sessionsIn(t.g) {
		s.requestGit(id)
	}
	if t.g.WorktreeID != "" {
		if rec, err := s.wts.Get(t.g.WorktreeID); err == nil {
			s.publishWorktrees(rec.ProjectID)
		}
	}
}
