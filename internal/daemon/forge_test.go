package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/protocol"
)

// fakeGH puts a gh on the daemon's search path that answers `pr list` and
// `repo view` with canned JSON (no network, no real account).
func fakeGH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  "pr list"*) echo '[{"number":12,"title":"Add x","state":"OPEN","url":"https://github.com/acme/app/pull/12","baseRefName":"main","headRefName":"feat/x","statusCheckRollup":[]}]' ;;
  "repo view"*) echo '{"squashMergeAllowed":true}' ;;
  *) echo "fake gh: $*" >&2; exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	testExtraPath.Store(dir)
	t.Cleanup(func() { testExtraPath.Store("") })
}

func TestForgeStatusInMainFolder(t *testing.T) {
	fakeGH(t)
	c, _, stop := startDaemon(t)
	defer stop()
	repo := tempRepo(t)
	// A GitHub remote that is never contacted: its main is faked locally.
	gitT(t, repo, "remote", "add", "origin", "https://github.com/acme/app.git")
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitT(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	proj, err := c.ProjectAdd(ctxT(t), repo, "app")
	if err != nil {
		t.Fatal(err)
	}
	sh := spawnWith(t, c, proj.ID, "shell", "", "")
	ref := protocol.ForgeRef{SessionID: sh.ID}

	// On the base branch: nothing to show.
	st, err := c.ForgeStatus(ctxT(t), ref)
	if err != nil || st.Unavailable != apierr.ForgeOnBase {
		t.Fatalf("on main: %+v, %v", st, err)
	}

	// On a branch with two commits: its PR, merge methods and a suggestion.
	gitT(t, repo, "switch", "-q", "-c", "feat/add-x")
	for _, m := range []string{"first step", "second step"} {
		os.WriteFile(filepath.Join(repo, "a.txt"), []byte(m+"\n"), 0o644)
		gitT(t, repo, "commit", "-q", "-am", m)
	}
	st, err = c.ForgeStatus(ctxT(t), ref)
	if err != nil {
		t.Fatal(err)
	}
	if st.Unavailable != "" || st.Kind != "github" || st.Tool != "gh" || st.Branch != "feat/add-x" || st.Base != "main" {
		t.Fatalf("status: %+v", st)
	}
	if st.PR == nil || st.PR.Number != 12 || len(st.MergeMethods) != 1 || st.MergeMethods[0] != "squash" {
		t.Fatalf("pr: %+v, methods %v", st.PR, st.MergeMethods)
	}
	if st.Ahead != 2 || st.Title != "Add x" || st.Body != "- first step\n- second step" {
		t.Fatalf("suggestion: ahead %d, %q / %q", st.Ahead, st.Title, st.Body)
	}

	// The folder moved to another branch since the PR was shown: no merge.
	if _, err := c.ForgeMerge(ctxT(t), protocol.ForgeMergeParams{ForgeRef: ref, Number: 12, Method: "squash", Head: "feat/other"}); !hasCode(err, apierr.ForgeHeadMismatch) {
		t.Fatalf("merge for another branch: %v", err)
	}

	// Suggested titles keep non-ASCII branch names whole.
	gitT(t, repo, "switch", "-q", "-c", "ñandú-fix")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "one")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "two")
	if st, err = c.ForgeStatus(ctxT(t), ref); err != nil || st.Title != "Ñandú fix" {
		t.Fatalf("non-ASCII title: %q, %v", st.Title, err)
	}

	// Without gh: the integration is off, not an error (unless the
	// system itself has one in /usr/bin or /bin, as CI's Ubuntu does).
	testExtraPath.Store("")
	if _, sys := profile.LookPathIn("gh", hermeticPath()); !sys {
		st, err = c.ForgeStatus(ctxT(t), ref)
		if err != nil || st.Unavailable != apierr.ForgeCLIMissing {
			t.Fatalf("no gh: %+v, %v", st, err)
		}
	}
}
