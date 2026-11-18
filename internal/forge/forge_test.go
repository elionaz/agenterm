package forge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
)

// fakeCLI installs a fake tool (gh or glab) in a temp PATH. Each rule maps
// a substring of the command line to a canned stdout (or, prefixed with
// "!", stderr and exit 1); the first match wins. Every call is logged.
type fakeCLI struct {
	t    *testing.T
	dir  string
	log  string
	path string
}

func newFake(t *testing.T, tool string, rules [][2]string) *fakeCLI {
	t.Helper()
	dir := t.TempDir()
	// The script itself needs cat, head… from the system.
	f := &fakeCLI{t: t, dir: dir, log: filepath.Join(dir, "calls.log"), path: dir + ":/usr/bin:/bin"}
	var rs strings.Builder
	for i, r := range rules {
		name := filepath.Join(dir, "r"+string(rune('a'+i)))
		if err := os.WriteFile(name, []byte(r[1]), 0o600); err != nil {
			t.Fatal(err)
		}
		rs.WriteString(r[0] + "\t" + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "rules"), []byte(rs.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
printf '%s\n' "$*" >> "` + f.log + `"
tab=$(printf '\t')
while IFS="$tab" read -r pat file; do
  case "$*" in
    *"$pat"*)
      if [ "$(head -c1 "$file")" = "!" ]; then tail -c +2 "$file" >&2; exit 1; fi
      cat "$file"; exit 0 ;;
  esac
done < "` + filepath.Join(dir, "rules") + `"
echo "fake ` + tool + `: no rule for: $*" >&2
exit 2
`
	if err := os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fakeCLI) calls() []string {
	b, _ := os.ReadFile(f.log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func code(err error) string {
	c, _, _ := apierr.Code(err)
	return c
}

const ghList = `[{"number":7,"title":"old","state":"MERGED","isDraft":false,"url":"https://github.com/acme/app/pull/7","baseRefName":"main","headRefName":"feat/x","statusCheckRollup":[]},
 {"number":12,"title":"Add x","state":"OPEN","isDraft":false,"url":"https://github.com/acme/app/pull/12","baseRefName":"main","headRefName":"feat/x",
  "reviewDecision":"CHANGES_REQUESTED","mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED",
  "statusCheckRollup":[
   {"__typename":"CheckRun","name":"go","status":"COMPLETED","conclusion":"SUCCESS"},
   {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE"},
   {"__typename":"CheckRun","name":"e2e","status":"IN_PROGRESS","conclusion":""},
   {"__typename":"StatusContext","context":"ci/legacy","state":"SUCCESS"}]}]`

func TestGitHubStatus(t *testing.T) {
	f := newFake(t, "gh", [][2]string{{"pr list", ghList}})
	fg, err := For("git@github.com:acme/app.git", t.TempDir(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := fg.Status(ctxT(t), "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	// The open one wins over the newer-listed merged one.
	if pr.Number != 12 || pr.State != StateOpen || pr.Review != ReviewChangesRequested || !pr.Blocked || pr.Conflicts {
		t.Fatalf("pr = %+v", pr)
	}
	c := pr.Checks
	if c.State != ChecksFailing || c.Total != 4 || c.Passed != 2 || c.Failed != 1 || c.Pending != 1 || len(c.Failing) != 1 || c.Failing[0] != "lint" {
		t.Fatalf("checks = %+v", c)
	}
	if got := f.calls()[0]; !strings.Contains(got, "-R acme/app --head feat/x --state all") {
		t.Fatalf("call = %q", got)
	}
}

func TestGitHubNoPR(t *testing.T) {
	f := newFake(t, "gh", [][2]string{{"pr list", "[]"}})
	fg, _ := For("https://github.com/acme/app", t.TempDir(), f.path)
	pr, err := fg.Status(ctxT(t), "feat/none")
	if err != nil || pr != nil {
		t.Fatalf("no PR: %+v, %v", pr, err)
	}
}

func TestGitHubComments(t *testing.T) {
	f := newFake(t, "gh", [][2]string{
		{"pr view 12", `{"url":"https://github.com/acme/app/pull/12",
		  "comments":[{"author":{"login":"ana"},"body":"looks good","createdAt":"2026-01-02T10:00:00Z","url":"https://github.com/acme/app/pull/12#issuecomment-1"}],
		  "reviews":[{"author":{"login":"bo"},"body":"","state":"COMMENTED","submittedAt":"2026-01-02T11:00:00Z"},
		             {"author":{"login":"bo"},"body":"fix the test","state":"CHANGES_REQUESTED","submittedAt":"2026-01-02T12:00:00Z"}]}`},
		{"pulls/12/comments", `[{"user":{"login":"bo"},"body":"off by one","path":"a.go","line":0,"original_line":42,"html_url":"https://github.com/acme/app/pull/12#discussion_r1","created_at":"2026-01-02T11:00:00Z"}]`},
	})
	fg, _ := For("https://github.com/acme/app", t.TempDir(), f.path)
	cs, err := fg.Comments(ctxT(t), 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("comments = %+v", cs)
	}
	if cs[0].Author != "ana" || cs[1].Kind != "line" || cs[1].Path != "a.go" || cs[1].Line != 42 || cs[2].Review != ReviewChangesRequested {
		t.Fatalf("comments = %+v", cs)
	}
}

func TestGitHubMergeAndCreate(t *testing.T) {
	f := newFake(t, "gh", [][2]string{
		{"repo view", `{"mergeCommitAllowed":false,"rebaseMergeAllowed":true,"squashMergeAllowed":true}`},
		{"pr merge", ""},
		{"-X DELETE", "!gh: Reference does not exist (HTTP 422)"},
		{"pr create", "https://github.com/acme/app/pull/13\n"},
	})
	fg, _ := For("https://github.com/acme/app", t.TempDir(), f.path)
	ms, err := fg.MergeMethods(ctxT(t))
	if err != nil || strings.Join(ms, ",") != "squash,rebase" {
		t.Fatalf("methods = %v, %v", ms, err)
	}
	// The branch was already deleted by the repository: not an error.
	if err := fg.Merge(ctxT(t), 12, "squash", "feat/x", true); err != nil {
		t.Fatal(err)
	}
	if err := fg.Merge(ctxT(t), 12, "octopus", "feat/x", false); err == nil {
		t.Fatal("unknown method accepted")
	}
	if err := fg.Create(ctxT(t), CreateParams{Head: "feat/x", Base: "main", Title: "Add x", Body: "- one\n- two", Draft: true}); err != nil {
		t.Fatal(err)
	}
	calls := f.calls()
	if !strings.Contains(calls[1], "pr merge 12 -R acme/app --squash") || strings.Contains(calls[1], "--delete-branch") {
		t.Fatalf("merge call = %q", calls[1])
	}
	if !strings.Contains(calls[2], "api -X DELETE repos/acme/app/git/refs/heads/feat/x") {
		t.Fatalf("delete call = %q", calls[2])
	}
	// The multi-line body spans log lines: look at the whole log.
	if all := strings.Join(calls, "\n"); !strings.Contains(all, "pr create -R acme/app --head feat/x --base main --title Add x --body - one\n- two --draft") {
		t.Fatalf("create call missing in:\n%s", all)
	}
}

func TestErrors(t *testing.T) {
	if _, err := For("https://bitbucket.org/a/b", t.TempDir(), t.TempDir()); code(err) != apierr.ForgeUnsupported {
		t.Fatalf("bitbucket: %v", err)
	}
	if _, err := For("https://github.com/a/b", t.TempDir(), t.TempDir()); code(err) != apierr.ForgeCLIMissing {
		t.Fatalf("no gh: %v", err)
	}
	f := newFake(t, "gh", [][2]string{{"pr list", "!To get started with GitHub CLI, please run:  gh auth login"}})
	fg, _ := For("https://github.com/a/b", t.TempDir(), f.path)
	if _, err := fg.Status(ctxT(t), "x"); code(err) != apierr.ForgeNotAuthed {
		t.Fatalf("not logged in: %v", err)
	}
	f = newFake(t, "gh", [][2]string{{"pr merge", "!Pull request is not mergeable: the base branch policy prohibits the merge."}})
	fg, _ = For("https://github.com/a/b", t.TempDir(), f.path)
	err := fg.Merge(ctxT(t), 1, "squash", "", false)
	if code(err) != apierr.ForgeFailed || !strings.Contains(err.Error(), "not mergeable") {
		t.Fatalf("merge refused: %v", err)
	}
}

func TestGitLab(t *testing.T) {
	f := newFake(t, "glab", [][2]string{
		{"merge_requests?", `[{"iid":3,"state":"merged"},{"iid":5,"state":"opened"}]`},
		{"merge_requests/5/discussions", `[{"notes":[
		   {"id":1,"author":{"username":"ana"},"body":"assigned to @bo","system":true,"created_at":"2026-01-01T00:00:00Z"},
		   {"id":2,"author":{"username":"bo"},"body":"rename this","system":false,"created_at":"2026-01-02T00:00:00Z","position":{"new_path":"b.go","new_line":7}}]}]`},
		{"merge_requests/5", `{"iid":5,"title":"Add y","state":"opened","draft":true,"web_url":"https://gitlab.example.org/team/proj/-/merge_requests/5",
		   "target_branch":"main","source_branch":"feat/y","detailed_merge_status":"not_approved","head_pipeline":{"status":"running"}}`},
		{"mr merge", ""},
		{"mr create", ""},
		{"api --hostname gitlab.example.org projects/team%2Fproj", `{"squash_option":"default_on"}`},
	})
	fg, err := For("git@gitlab.example.org:team/proj.git", t.TempDir(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := fg.Status(ctxT(t), "feat/y")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 5 || pr.State != StateDraft || pr.Review != ReviewRequired || !pr.Blocked || pr.Checks.State != ChecksPending || pr.Checks.Total != 1 {
		t.Fatalf("mr = %+v", pr)
	}
	cs, err := fg.Comments(ctxT(t), 5)
	if err != nil || len(cs) != 1 || cs[0].Path != "b.go" || cs[0].Line != 7 || !strings.HasSuffix(cs[0].URL, "/-/merge_requests/5#note_2") {
		t.Fatalf("comments = %+v, %v", cs, err)
	}
	ms, err := fg.MergeMethods(ctxT(t))
	if err != nil || strings.Join(ms, ",") != "squash,merge" {
		t.Fatalf("methods = %v, %v", ms, err)
	}
	if err := fg.Merge(ctxT(t), 5, "squash", "feat/y", true); err != nil {
		t.Fatal(err)
	}
	if err := fg.Create(ctxT(t), CreateParams{Head: "feat/y", Base: "main", Title: "Add y", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	calls := strings.Join(f.calls(), "\n")
	for _, want := range []string{
		"api --hostname gitlab.example.org projects/team%2Fproj/merge_requests?",
		"mr merge 5 -R https://gitlab.example.org/team/proj --yes --squash --remove-source-branch",
		"mr create -R https://gitlab.example.org/team/proj --source-branch feat/y --target-branch main --title Add y --description b --yes",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("missing call %q in:\n%s", want, calls)
		}
	}
}
