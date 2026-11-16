// Package gitx wraps the `git` command line for agenterm: repository
// queries, per-session worktrees, diffs and merges. Every call shells out to
// git with a context timeout (no libgit2); read-only commands run with
// GIT_OPTIONAL_LOCKS=0 so they never take index.lock under an agent that is
// using git in the same tree.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultTimeout bounds a single git invocation when ctx has no deadline.
const DefaultTimeout = 15 * time.Second

// EmptyTree is git's well-known empty tree object (diff base for a
// repository without commits).
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// ErrNotRepo is returned when a directory is not inside a git work tree.
var ErrNotRepo = errors.New("no es un repositorio git")

// Error is a failed git invocation.
type Error struct {
	Args   []string
	Code   int
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

func (e *Error) Unwrap() error { return e.Err }

// exitCode extracts git's exit status (-1 if it did not run).
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

type runOpts struct {
	limit   int   // max stdout bytes kept (0 = unlimited)
	okCodes []int // extra exit codes that are not errors (diff --no-index: 1)
	write   bool  // may take locks (worktree add, merge…)
}

// run executes `git -C dir args…` and returns stdout. truncated reports
// that stdout exceeded opts.limit (the process is then stopped).
func run(ctx context.Context, dir string, o runOpts, args ...string) (out []byte, truncated bool, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	full := append([]string{"-C", dir, "-c", "core.quotepath=off", "-c", "color.ui=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no", "GIT_PAGER=cat", "LC_ALL=C", "LANG=C")
	if !o.write {
		cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0")
	}
	var stdout limitBuf
	stdout.limit = o.limit
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if o.limit > 0 {
		stdout.onFull = func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
	}
	err = cmd.Run()
	if stdout.full {
		return stdout.buf.Bytes(), true, nil
	}
	if err != nil {
		code := exitCode(err)
		for _, c := range o.okCodes {
			if code == c {
				return stdout.buf.Bytes(), false, nil
			}
		}
		if ctx.Err() != nil {
			err = fmt.Errorf("%w (timeout)", ctx.Err())
		}
		return stdout.buf.Bytes(), false, &Error{Args: args, Code: code, Stderr: stderr.String(), Err: err}
	}
	return stdout.buf.Bytes(), false, nil
}

// limitBuf keeps at most limit bytes; once exceeded it flags full and calls
// onFull (once).
type limitBuf struct {
	buf    bytes.Buffer
	limit  int
	full   bool
	onFull func()
}

func (l *limitBuf) Write(p []byte) (int, error) {
	if l.full {
		return len(p), nil
	}
	if l.limit > 0 && l.buf.Len()+len(p) > l.limit {
		l.buf.Write(p[:l.limit-l.buf.Len()])
		l.full = true
		if l.onFull != nil {
			l.onFull()
		}
		return len(p), nil
	}
	return l.buf.Write(p)
}

// git runs a read-only command and returns trimmed stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, _, err := run(ctx, dir, runOpts{}, args...)
	return strings.TrimRight(string(out), "\n"), err
}

// gitWrite runs a command that may modify the repository.
func gitWrite(ctx context.Context, dir string, args ...string) (string, error) {
	out, _, err := run(ctx, dir, runOpts{write: true}, args...)
	return strings.TrimRight(string(out), "\n"), err
}

// Available reports whether a git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(ctx context.Context, dir string) bool {
	out, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// RepoRoot is the top-level directory of the work tree containing dir.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if !IsRepo(ctx, dir) {
			return "", fmt.Errorf("%s: %w", dir, ErrNotRepo)
		}
		return "", err
	}
	return out, nil
}

// MainWorktree is the repository's main work tree (the first entry of
// `git worktree list`), whatever linked worktree dir belongs to.
func MainWorktree(ctx context.Context, dir string) (string, error) {
	wts, err := ListWorktrees(ctx, dir)
	if err != nil {
		return "", err
	}
	if len(wts) == 0 || wts[0].Bare {
		return "", errors.New("el repositorio no tiene una carpeta de trabajo principal (¿bare?)")
	}
	return wts[0].Path, nil
}

// CurrentBranch is the branch checked out in dir ("" when detached).
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && ge.Code == 1 { // detached HEAD
			return "", nil
		}
		return "", err
	}
	return out, nil
}

// BranchExists reports whether refs/heads/name exists.
func BranchExists(ctx context.Context, dir, name string) bool {
	_, err := git(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Branches lists local branch names.
func Branches(ctx context.Context, dir string) ([]string, error) {
	out, err := git(ctx, dir, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// DefaultBase is the base for a new worktree: the branch checked out in
// dir, else main or master. A repository without any of them (detached
// HEAD, other names) yields an error.
func DefaultBase(ctx context.Context, dir string) (string, error) {
	if b, err := CurrentBranch(ctx, dir); err == nil && b != "" {
		return b, nil
	}
	for _, b := range []string{"main", "master"} {
		if BranchExists(ctx, dir, b) {
			return b, nil
		}
	}
	return "", errors.New("no se encuentra la rama base (HEAD separado y sin main/master): indica una base")
}

// ResolveCommit returns the commit hash rev points to.
func ResolveCommit(ctx context.Context, dir, rev string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil || out == "" {
		return "", fmt.Errorf("la base %q no existe en el repositorio", rev)
	}
	return out, nil
}

// HasCommits reports whether HEAD points to a commit.
func HasCommits(ctx context.Context, dir string) bool {
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return err == nil
}

// Status counts the changes of a work tree (git status --porcelain=v2).
type Status struct {
	Staged    int      `json:"staged"`
	Unstaged  int      `json:"unstaged"`
	Untracked int      `json:"untracked"`
	Conflicts int      `json:"conflicts"`
	Files     []string `json:"files,omitempty"` // every path with a change
}

// Dirty reports uncommitted changes, untracked files included.
func (s Status) Dirty() bool { return len(s.Files) > 0 }

// Changed is the number of paths with any change.
func (s Status) Changed() int { return len(s.Files) }

// GetStatus parses `git status --porcelain=v2 -z` in dir.
func GetStatus(ctx context.Context, dir string) (Status, error) {
	out, _, err := run(ctx, dir, runOpts{}, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=dirty")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out), nil
}

func parseStatus(out []byte) Status {
	var s Status
	recs := bytes.Split(out, []byte{0})
	for i := 0; i < len(recs); i++ {
		r := string(recs[i])
		if r == "" {
			continue
		}
		switch r[0] {
		case '1', '2':
			// "1 XY sub mH mI mW hH hI path" / "2 XY … Xscore path\0orig"
			f := strings.SplitN(r, " ", 9)
			n := 9
			if r[0] == '2' {
				f = strings.SplitN(r, " ", 10)
				n = 10
				i++ // the original path follows as its own record
			}
			if len(f) < n {
				continue
			}
			xy := f[1]
			if xy[0] != '.' {
				s.Staged++
			}
			if xy[1] != '.' {
				s.Unstaged++
			}
			s.Files = append(s.Files, f[n-1])
		case 'u':
			f := strings.SplitN(r, " ", 11)
			s.Conflicts++
			if len(f) == 11 {
				s.Files = append(s.Files, f[10])
			}
		case '?':
			s.Untracked++
			s.Files = append(s.Files, r[2:])
		}
	}
	return s
}

// Commit is one entry of a commit list.
type Commit struct {
	Hash    string    `json:"hash"`
	Short   string    `json:"short"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// maxCommits caps commit lists.
const maxCommits = 200

// CommitsAhead lists the commits in head that base does not have
// (`git log base..head`), newest first, at most 200.
func CommitsAhead(ctx context.Context, dir, base, head string) ([]Commit, error) {
	out, err := git(ctx, dir, "log", "--no-color", "--max-count="+strconv.Itoa(maxCommits),
		"--format=%H%x1f%h%x1f%s%x1f%an%x1f%aI", base+".."+head, "--")
	if err != nil {
		return nil, err
	}
	var list []Commit
	for _, l := range splitLines(out) {
		f := strings.Split(l, "\x1f")
		if len(f) != 5 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, f[4])
		list = append(list, Commit{Hash: f[0], Short: f[1], Subject: f[2], Author: f[3], Date: t})
	}
	return list, nil
}

// AheadCount is the number of commits in head not in base.
func AheadCount(ctx context.Context, dir, base, head string) (int, error) {
	out, err := git(ctx, dir, "rev-list", "--count", base+".."+head, "--")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(out))
}

// MergeBase is the best common ancestor of a and b.
func MergeBase(ctx context.Context, dir, a, b string) (string, error) {
	return git(ctx, dir, "merge-base", a, b)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// samePath compares paths after resolving symlinks (/tmp vs /private/tmp).
func samePath(a, b string) bool {
	return canon(a) == canon(b)
}

func canon(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return filepath.Clean(p)
}
