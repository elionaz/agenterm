package daemon

import (
	"context"
	"errors"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/gitx"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/repoconfig"
	"github.com/elionaz/agenterm/internal/worktree"
)

// Setting up a new worktree from the repository's .agenterm.yaml
// (internal/repoconfig): worktree.copy is copied from the main folder
// right after `git worktree add`, and worktree.setup runs in the session's
// terminal before the agent starts (setupLaunch), once the user trusted
// that exact command. Only new worktrees get either: resuming a session or
// reopening a worktree runs nothing again.

// worktreePrep is what a new worktree gets, decided before it is created
// (an untrusted setup refuses the spawn without leaving a worktree behind).
type worktreePrep struct {
	repo    string // the main checkout
	cfg     repoconfig.Config
	run     string   // setup command to run ("" none)
	suggest []string // no .agenterm.yaml: ignored .env* files to offer copying
}

// worktreeSetup reads the configuration of proj's repository and checks
// its setup command against what the user trusted (p.TrustSetup trusts
// it now, p.SkipSetup leaves it out).
func (s *Server) worktreeSetup(proj project.Project, p protocol.SpawnParams) (worktreePrep, error) {
	ctx, cancel := context.WithTimeout(s.bg, gitQueryTO)
	defer cancel()
	var prep worktreePrep
	if !gitx.IsRepo(ctx, proj.Path) {
		return prep, nil // createWorktree reports it
	}
	top, err := gitx.RepoRoot(ctx, proj.Path)
	if err != nil {
		return prep, err
	}
	if prep.repo, err = gitx.MainWorktree(ctx, top); err != nil {
		return prep, err
	}
	if prep.cfg, err = repoconfig.Load(prep.repo); err != nil {
		return prep, err
	}
	if prep.cfg.Path == "" {
		prep.suggest = repoconfig.Suggest(ctx, prep.repo)
	}
	cmd := prep.cfg.Worktree.Setup
	switch {
	case cmd == "" || p.SkipSetup:
	case p.TrustSetup:
		if err := s.trust.Allow(prep.repo, cmd); err != nil {
			return prep, err
		}
		s.log.Info("worktree setup trusted", "repo", prep.repo, "command", cmd)
		prep.run = cmd
	case s.trust.Allowed(prep.repo, cmd):
		prep.run = cmd
	default:
		return prep, apierr.New(apierr.SetupUntrusted, map[string]any{"repo": prep.repo, "file": prep.cfg.Path, "command": cmd},
			"%s asks to run %q in every new worktree: trust it (agenterm spawn --trust-setup) or create the worktree without it (--no-setup)",
			prep.cfg.Path, cmd)
	}
	return prep, nil
}

// prepareWorktree copies worktree.copy into the new worktree rec.
func (s *Server) prepareWorktree(rec worktree.Record, prep worktreePrep) error {
	if len(prep.cfg.Worktree.Copy) == 0 || prep.repo == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(s.bg, gitActionTO)
	defer cancel()
	res, err := repoconfig.CopyInto(ctx, prep.repo, rec.Path, prep.cfg.Worktree.Copy)
	if err != nil {
		return err
	}
	s.log.Info("worktree files copied", "id", rec.ID, "copied", res.Copied, "skipped", res.Skipped, "missing", res.Missing)
	return nil
}

// setupScript runs $AGENTERM_SETUP in the user's login shell, then execs
// the agent ("$@"). A failed setup says so and waits: Enter starts the
// agent anyway, Ctrl-C ends the session.
const setupScript = `printf '\033[2m[agenterm] worktree setup: %s\033[0m\n' "$AGENTERM_SETUP"
"${SHELL:-/bin/sh}" -lc "$AGENTERM_SETUP"
s=$?
if [ "$s" -ne 0 ]; then
  printf '\n\033[33m[agenterm] the setup failed (exit %d). Press Enter to start the agent anyway, or Ctrl-C to end the session.\033[0m\n' "$s"
  read -r _ || exit "$s"
fi
unset AGENTERM_SETUP
exec "$@"`

// setupLaunch is the argv that runs the worktree setup and then cmd args.
func setupLaunch(cmd string, args []string) []string {
	return append([]string{"/bin/sh", "-c", setupScript, "agenterm-setup", cmd}, args...)
}

// worktreeCopy copies files from the main folder into a worktree and,
// with Save, writes them into a new .agenterm.yaml (the "copy these .env
// files" offer of a repository that has none).
func (s *Server) worktreeCopy(p protocol.WorktreeCopyParams) (protocol.WorktreeCopyResult, error) {
	var out protocol.WorktreeCopyResult
	rec, err := s.wts.Get(p.ID)
	if err != nil {
		return out, err
	}
	if len(p.Files) == 0 {
		return out, errors.New("no files to copy")
	}
	ctx, cancel := context.WithTimeout(s.bg, gitActionTO)
	defer cancel()
	if p.Save {
		if out.Created, err = repoconfig.Create(rec.Repo, p.Files); err != nil {
			return out, err
		}
		s.log.Info("repository config created", "path", out.Created)
	}
	res, err := repoconfig.CopyInto(ctx, rec.Repo, rec.Path, p.Files)
	if err != nil {
		return out, err
	}
	out.Copied, out.Skipped, out.Missing = res.Copied, res.Skipped, res.Missing
	if out.Copied == nil {
		out.Copied = []string{}
	}
	s.log.Info("worktree files copied", "id", rec.ID, "copied", res.Copied, "via", "offer")
	for _, id := range s.sessionsIn(recGit(rec)) {
		s.requestGit(id)
	}
	return out, nil
}
