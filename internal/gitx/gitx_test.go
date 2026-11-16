package gitx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
)

// TestMain isolates git from the user's configuration: no system config,
// an empty global config and a throwaway HOME.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "gitx-home")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(home, "gitconfig")
	_ = os.WriteFile(global, nil, 0o600)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		os.Unsetenv(k)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

// sh runs git in dir and fails the test on error.
func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates a repository on main with one commit.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := canon(t.TempDir())
	sh(t, dir, "init", "-q", "-b", "main")
	sh(t, dir, "config", "user.name", "Test")
	sh(t, dir, "config", "user.email", "test@example.com")
	sh(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")
	write(t, dir, "b.txt", "bee\n")
	write(t, dir, "c.txt", "a file that will be renamed\nwith some lines\nso git sees it\n")
	write(t, dir, "img.bin", "\x00\x01\x02binary")
	sh(t, dir, "add", "-A")
	sh(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

func TestRepoQueries(t *testing.T) {
	c := ctx(t)
	plain := t.TempDir()
	if IsRepo(c, plain) {
		t.Fatal("plain dir reported as repo")
	}
	if _, err := RepoRoot(c, plain); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("RepoRoot(non-repo) = %v, want ErrNotRepo", err)
	}
	repo := newRepo(t)
	write(t, repo, "sub/x.txt", "x\n")
	if !IsRepo(c, filepath.Join(repo, "sub")) {
		t.Fatal("subdir not in repo")
	}
	if root, err := RepoRoot(c, filepath.Join(repo, "sub")); err != nil || root != repo {
		t.Fatalf("RepoRoot = %q, %v; want %q", root, err, repo)
	}
	if b, _ := CurrentBranch(c, repo); b != "main" {
		t.Fatalf("CurrentBranch = %q", b)
	}
	sh(t, repo, "branch", "feature")
	if !BranchExists(c, repo, "feature") || BranchExists(c, repo, "nope") {
		t.Fatal("BranchExists")
	}
	bs, _ := Branches(c, repo)
	if strings.Join(bs, ",") != "feature,main" {
		t.Fatalf("Branches = %v", bs)
	}
	sh(t, repo, "checkout", "-q", "feature")
	if b, _ := DefaultBase(c, repo); b != "feature" {
		t.Fatalf("DefaultBase on feature = %q", b)
	}
	sh(t, repo, "checkout", "-q", "--detach")
	if b, _ := CurrentBranch(c, repo); b != "" {
		t.Fatalf("detached CurrentBranch = %q", b)
	}
	if b, _ := DefaultBase(c, repo); b != "main" {
		t.Fatalf("detached DefaultBase = %q, want main", b)
	}
	if main, err := MainWorktree(c, repo); err != nil || main != repo {
		t.Fatalf("MainWorktree = %q, %v", main, err)
	}
}

func TestStatus(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	st, err := GetStatus(c, repo)
	if err != nil || st.Dirty() {
		t.Fatalf("clean status = %+v, %v", st, err)
	}
	write(t, repo, "a.txt", "changed\n")  // unstaged
	write(t, repo, "new file.txt", "n\n") // untracked (space in name)
	write(t, repo, "staged.txt", "s\n")   // staged
	sh(t, repo, "add", "staged.txt")
	sh(t, repo, "mv", "c.txt", "renamed.txt") // staged rename
	st, err = GetStatus(c, repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Staged != 2 || st.Unstaged != 1 || st.Untracked != 1 || st.Changed() != 4 {
		t.Fatalf("status = %+v", st)
	}
	joined := strings.Join(st.Files, "|")
	for _, want := range []string{"a.txt", "new file.txt", "staged.txt", "renamed.txt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("status files %q missing %q", joined, want)
		}
	}
}

func TestSanitizeBranch(t *testing.T) {
	for in, want := range map[string]string{
		"agent/claude-ab12":     "agent/claude-ab12",
		" mi rama  nueva ":      "mi-rama-nueva",
		"agent//x..y":           "agent/x.y",
		"-.lead/trail.lock":     "lead/trail",
		"feat/ñandú~^:?*[":      "feat/and-",
		"HEAD":                  "",
		"..":                    "",
		"agent/claude (yolo)/1": "agent/claude-yolo-/1",
	} {
		if got := SanitizeBranch(in); got != want {
			t.Errorf("SanitizeBranch(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorktreeDiff(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	// A dirty main tree is fine: the worktree starts from main's commit.
	write(t, repo, "a.txt", "dirty in main\n")
	wt := filepath.Join(canon(t.TempDir()), "proj", "s1")

	if err := AddWorktree(c, repo, wt, "agent/x", "nope"); err == nil || !strings.Contains(err.Error(), "does not exist") || code(err) != apierr.BaseMissing {
		t.Fatalf("missing base: %v", err)
	}
	if err := AddWorktree(c, repo, wt, "agent/x", "main"); err != nil {
		t.Fatal(err)
	}
	if err := AddWorktree(c, repo, wt+"b", "agent/x", "main"); err == nil {
		t.Fatal("duplicate branch accepted")
	}
	if got := UniqueBranch(c, repo, "agent/x"); got != "agent/x-2" {
		t.Fatalf("UniqueBranch = %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "a.txt")); string(b) != "one\ntwo\nthree\n" {
		t.Fatalf("worktree a.txt = %q (main's dirty change leaked)", b)
	}
	wts, _ := ListWorktrees(c, repo)
	if len(wts) != 2 || wts[1].Path != wt || wts[1].Branch != "agent/x" {
		t.Fatalf("ListWorktrees = %+v", wts)
	}

	// One committed change…
	write(t, wt, "committed.txt", "c1\nc2\n")
	sh(t, wt, "add", "committed.txt")
	sh(t, wt, "commit", "-q", "-m", "add committed")
	// …and uncommitted ones of every kind.
	write(t, wt, "a.txt", "one\nTWO\nthree\nfour\n")
	sh(t, wt, "rm", "-q", "b.txt")
	sh(t, wt, "mv", "c.txt", "d.txt")
	write(t, wt, "img.bin", "\x00\x09\x08other binary")
	write(t, wt, "untracked.txt", "u1\nu2\nu3\n")
	write(t, wt, "blob.dat", "\x00\x00bin")
	write(t, wt, ".gitignore", "ignored.log\n")
	write(t, wt, "ignored.log", "nope\n")

	cs, err := CommitsAhead(c, wt, "main", "HEAD")
	if err != nil || len(cs) != 1 || cs[0].Subject != "add committed" || cs[0].Short == "" {
		t.Fatalf("CommitsAhead = %+v, %v", cs, err)
	}
	if n, _ := AheadCount(c, wt, "main", "HEAD"); n != 1 {
		t.Fatalf("AheadCount = %d", n)
	}
	mb, err := MergeBase(c, wt, "main", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	d, err := WorkingDiff(c, wt, DiffOptions{From: mb})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]DiffFile{}
	for _, f := range d.Files {
		got[f.Path] = f
	}
	check := func(path, status string, add, del int, binary, untracked bool) {
		t.Helper()
		f, ok := got[path]
		if !ok {
			t.Errorf("%s missing from diff (%v)", path, keys(got))
			return
		}
		if f.Status != status || f.Additions != add || f.Deletions != del || f.Binary != binary || f.Untracked != untracked {
			t.Errorf("%s = %+v; want status %s +%d -%d binary=%v untracked=%v", path, f, status, add, del, binary, untracked)
		}
		if !binary && !strings.HasPrefix(f.Patch, "diff --git ") {
			t.Errorf("%s patch = %q", path, f.Patch)
		}
		if binary && f.Patch != "" {
			t.Errorf("%s: binary file with patch content", path)
		}
	}
	check("committed.txt", "A", 2, 0, false, false)
	check("a.txt", "M", 2, 1, false, false)
	check("b.txt", "D", 0, 1, false, false)
	check("d.txt", "R", 0, 0, false, false)
	check("img.bin", "M", 0, 0, true, false)
	check("untracked.txt", "A", 3, 0, false, true)
	check("blob.dat", "A", 0, 0, true, true)
	check(".gitignore", "A", 1, 0, false, true)
	if got["d.txt"].OldPath != "c.txt" {
		t.Errorf("rename old path = %q", got["d.txt"].OldPath)
	}
	if _, ok := got["ignored.log"]; ok {
		t.Error("ignored file listed")
	}
	if len(d.Files) != 8 || d.Truncated {
		t.Errorf("files = %d (%v), truncated = %v", len(d.Files), keys(got), d.Truncated)
	}
	if !strings.Contains(got["a.txt"].Patch, "+TWO") || !strings.Contains(got["untracked.txt"].Patch, "+u3") {
		t.Errorf("patch content wrong:\n%s", d.Patch())
	}
	if n, err := ChangedFiles(c, wt, mb); err != nil || n != 8 {
		t.Errorf("ChangedFiles = %d, %v", n, err)
	}

	// Stat only: no patch text.
	ds, _ := WorkingDiff(c, wt, DiffOptions{From: mb, StatOnly: true})
	if ds.Patch() != "" || len(ds.Files) != 8 {
		t.Errorf("stat-only diff = %+v", ds)
	}
	// Size cap: patches dropped, files kept and flagged.
	dt, _ := WorkingDiff(c, wt, DiffOptions{From: mb, MaxPatch: 200})
	if !dt.Truncated || len(dt.Files) != 8 || dt.PatchSize > 200 {
		t.Errorf("capped diff: truncated=%v files=%d size=%d", dt.Truncated, len(dt.Files), dt.PatchSize)
	}
	flagged := 0
	for _, f := range dt.Files {
		if f.Truncated {
			flagged++
		}
	}
	if flagged == 0 {
		t.Error("no file flagged as truncated")
	}

	// Same-folder mode diff (against HEAD) in the main tree.
	dm, err := WorkingDiff(c, repo, DiffOptions{})
	if err != nil || len(dm.Files) != 1 || dm.Files[0].Path != "a.txt" || dm.From != "HEAD" {
		t.Fatalf("main diff = %+v, %v", dm, err)
	}
}

func keys(m map[string]DiffFile) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestEmptyRepo(t *testing.T) {
	c := ctx(t)
	dir := canon(t.TempDir())
	sh(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "x.txt", "x\n")
	d, err := WorkingDiff(c, dir, DiffOptions{})
	if err != nil || d.From != EmptyTree || len(d.Files) != 1 || !d.Files[0].Untracked {
		t.Fatalf("empty repo diff = %+v, %v", d, err)
	}
	if err := AddWorktree(c, dir, filepath.Join(t.TempDir(), "w"), "agent/x", "main"); err == nil || !strings.Contains(err.Error(), "commits") {
		t.Fatalf("worktree in empty repo: %v", err)
	}
	if err := AddWorktree(c, t.TempDir(), filepath.Join(t.TempDir(), "w"), "agent/x", "main"); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("worktree outside a repo: %v", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	wt := filepath.Join(canon(t.TempDir()), "s1")
	if err := AddWorktree(c, repo, wt, "agent/rm", "main"); err != nil {
		t.Fatal(err)
	}
	write(t, wt, "a.txt", "dirty\n")
	write(t, wt, "new.txt", "new\n")
	opts := RemoveOptions{Base: "main", DeleteBranch: true}

	err := RemoveWorktree(c, repo, wt, "agent/rm", opts)
	var le *LossError
	if !errors.As(err, &le) || len(le.Uncommitted) != 2 || len(le.Unmerged) != 0 {
		t.Fatalf("dirty remove = %v (%+v)", err, le)
	}
	if !strings.Contains(err.Error(), "uncommitted") || !strings.Contains(err.Error(), "force") || code(err) != apierr.WorktreeDirty {
		t.Errorf("message = %q", err)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("refused remove deleted the worktree")
	}

	sh(t, wt, "add", "-A")
	sh(t, wt, "commit", "-q", "-m", "work")
	err = RemoveWorktree(c, repo, wt, "agent/rm", opts)
	if !errors.As(err, &le) || len(le.Unmerged) != 1 || len(le.Uncommitted) != 0 || !strings.Contains(err.Error(), "not merged into main") || code(err) != apierr.WorktreeUnmerged {
		t.Fatalf("unmerged remove = %v", err)
	}
	// Keeping the branch loses nothing.
	if le, err := Losses(c, repo, wt, "agent/rm", RemoveOptions{Base: "main"}); err != nil || le != nil {
		t.Fatalf("Losses keeping branch = %v, %v", le, err)
	}

	write(t, wt, "more.txt", "more\n")
	opts.Force = true
	if err := RemoveWorktree(c, repo, wt, "agent/rm", opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("worktree dir still there")
	}
	if BranchExists(c, repo, "agent/rm") {
		t.Fatal("branch not deleted")
	}
	if wts, _ := ListWorktrees(c, repo); len(wts) != 1 {
		t.Fatalf("worktrees after remove = %+v", wts)
	}

	// A clean worktree whose branch is kept goes without force; one whose
	// directory vanished is pruned.
	wt2 := filepath.Join(canon(t.TempDir()), "s2")
	if err := AddWorktree(c, repo, wt2, "agent/keep", "main"); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(wt2)
	if err := RemoveWorktree(c, repo, wt2, "agent/keep", RemoveOptions{Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if !BranchExists(c, repo, "agent/keep") {
		t.Fatal("kept branch deleted")
	}
	if wts, _ := ListWorktrees(c, repo); len(wts) != 1 {
		t.Fatalf("vanished worktree not pruned: %+v", wts)
	}
}

func TestMerge(t *testing.T) {
	c := ctx(t)
	repo := newRepo(t)
	wt := filepath.Join(canon(t.TempDir()), "s1")
	if err := AddWorktree(c, repo, wt, "agent/m", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := Merge(c, repo, "agent/m", "main", wt); err == nil || !strings.Contains(err.Error(), "nothing to merge") || code(err) != apierr.MergeNothing {
		t.Fatalf("empty merge: %v", err)
	}
	write(t, wt, "feature.txt", "f\n")
	sh(t, wt, "add", "-A")
	sh(t, wt, "commit", "-q", "-m", "feature")
	write(t, wt, "wip.txt", "uncommitted\n")

	// Refused while the main tree is dirty…
	write(t, repo, "a.txt", "dirty main\n")
	if _, err := Merge(c, repo, "agent/m", "main", wt); err == nil || !strings.Contains(err.Error(), "uncommitted") || code(err) != apierr.MergeMainDirty {
		t.Fatalf("dirty main merge: %v", err)
	}
	sh(t, repo, "checkout", "-q", "--", "a.txt")
	// …or on another branch.
	sh(t, repo, "checkout", "-q", "-b", "other")
	if _, err := Merge(c, repo, "agent/m", "main", wt); err == nil || !strings.Contains(err.Error(), "is on branch other") || code(err) != apierr.MergeWrongBranch {
		t.Fatalf("wrong-branch merge: %v", err)
	}
	sh(t, repo, "checkout", "-q", "main")
	// Untracked files in the main tree do not block it.
	write(t, repo, "scratch.txt", "s\n")

	res, err := Merge(c, wt, "agent/m", "main", wt) // any dir of the repo works
	if err != nil {
		t.Fatal(err)
	}
	if res.Commits != 1 || res.Uncommitted != 1 {
		t.Fatalf("merge result = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Fatal("merged file missing in main tree")
	}
	if parents := sh(t, repo, "log", "-1", "--format=%P"); len(strings.Fields(parents)) != 2 {
		t.Fatalf("not a merge commit (--no-ff): parents %q", parents)
	}

	// Conflicts are aborted, leaving main clean.
	write(t, wt, "a.txt", "from worktree\n")
	sh(t, wt, "commit", "-q", "-am", "wt change")
	write(t, repo, "a.txt", "from main\n")
	sh(t, repo, "commit", "-q", "-am", "main change")
	if _, err := Merge(c, repo, "agent/m", "main", ""); err == nil || !strings.Contains(err.Error(), "conflicts (a.txt)") || code(err) != apierr.MergeConflict {
		t.Fatalf("conflicting merge: %v", err)
	}
	if st, _ := GetStatus(c, repo); st.Staged+st.Unstaged+st.Conflicts != 0 {
		t.Fatalf("main not clean after aborted merge: %+v", st)
	}
}

func TestOpenArgv(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "code" || name == "cursor" {
			return "/usr/local/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	none := func(string) (string, error) { return "", errors.New("not found") }
	cases := []struct {
		target, editor string
		look           func(string) (string, error)
		want           string
	}{
		{"finder", "", look, "open /p"},
		{"editor", "", look, "/usr/local/bin/code /p"},
		{"editor", "", none, "open /p"},
		{"editor", "cursor -n", look, "/usr/local/bin/cursor -n /p"},
		{"editor", "Visual Studio Code", look, "open -a Visual Studio Code /p"},
		{"editor", "Zed.app", look, "open -a Zed.app /p"},
	}
	for _, tc := range cases {
		argv, err := OpenArgv(tc.target, tc.editor, "/p", tc.look)
		if err != nil || strings.Join(argv, " ") != tc.want {
			t.Errorf("OpenArgv(%q, %q) = %q, %v; want %q", tc.target, tc.editor, argv, err, tc.want)
		}
	}
	if _, err := OpenArgv("browser", "", "/p", look); err == nil {
		t.Error("unknown target accepted")
	}
}

func code(err error) string {
	c, _, _ := apierr.Code(err)
	return c
}
