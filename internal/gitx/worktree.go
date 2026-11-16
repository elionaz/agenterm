package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Worktree is an entry of `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short name; "" when detached
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool // its directory is gone
}

// ListWorktrees lists the work trees of the repository containing dir.
func ListWorktrees(ctx context.Context, dir string) ([]Worktree, error) {
	out, err := git(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur *Worktree
	for _, l := range strings.Split(out, "\n") {
		key, val, _ := strings.Cut(l, " ")
		switch key {
		case "worktree":
			list = append(list, Worktree{Path: val})
			cur = &list[len(list)-1]
		case "HEAD":
			if cur != nil {
				cur.Head = val
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(val, "refs/heads/")
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	return list, nil
}

var (
	badRefChars = regexp.MustCompile(`[^A-Za-z0-9._/-]+`)
	dashes      = regexp.MustCompile(`-{2,}`)
	slashes     = regexp.MustCompile(`/{2,}`)
)

// SanitizeBranch turns s into a valid branch name (best effort; "" if
// nothing usable is left): spaces and odd characters become "-", no "..",
// no leading "-" or ".", no trailing "/", "." or ".lock".
func SanitizeBranch(s string) string {
	s = strings.TrimSpace(s)
	s = badRefChars.ReplaceAllString(s, "-")
	s = strings.ReplaceAll(s, "..", ".")
	s = slashes.ReplaceAllString(s, "/")
	s = dashes.ReplaceAllString(s, "-")
	parts := strings.Split(s, "/")
	var keep []string
	for _, p := range parts {
		p = strings.TrimLeft(p, ".-")
		p = strings.TrimSuffix(p, ".lock")
		p = strings.TrimRight(p, ".")
		if p != "" {
			keep = append(keep, p)
		}
	}
	s = strings.Join(keep, "/")
	if s == "@" || s == "HEAD" {
		return ""
	}
	return s
}

// UniqueBranch returns name, or name-2, name-3… if it is already taken.
func UniqueBranch(ctx context.Context, dir, name string) string {
	if !BranchExists(ctx, dir, name) {
		return name
	}
	for i := 2; ; i++ {
		c := name + "-" + strconv.Itoa(i)
		if !BranchExists(ctx, dir, c) {
			return c
		}
	}
}

// CheckBranchName validates name with `git check-ref-format --branch`.
func CheckBranchName(ctx context.Context, dir, name string) error {
	if name == "" {
		return errors.New("nombre de rama vacío")
	}
	if _, err := git(ctx, dir, "check-ref-format", "--branch", name); err != nil {
		return fmt.Errorf("nombre de rama no válido: %q", name)
	}
	return nil
}

// AddWorktree runs `git worktree add <path> -b <branch> <base>` in the
// repository containing repo. The main work tree may be dirty: the new one
// starts from base's commit.
func AddWorktree(ctx context.Context, repo, path, branch, base string) error {
	if !IsRepo(ctx, repo) {
		return fmt.Errorf("%s: %w: el modo worktree no está disponible", repo, ErrNotRepo)
	}
	if !HasCommits(ctx, repo) {
		return errors.New("el repositorio no tiene commits todavía: haz un primer commit para usar worktrees")
	}
	if _, err := ResolveCommit(ctx, repo, base); err != nil {
		return err
	}
	if err := CheckBranchName(ctx, repo, branch); err != nil {
		return err
	}
	if BranchExists(ctx, repo, branch) {
		return fmt.Errorf("la rama %q ya existe", branch)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("la carpeta del worktree ya existe: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_, err := gitWrite(ctx, repo, "worktree", "add", "-b", branch, "--", path, base)
	return err
}

// RemoveOptions configure RemoveWorktree.
type RemoveOptions struct {
	// Force removes even with uncommitted changes (and, with
	// DeleteBranch, unmerged commits).
	Force bool
	// DeleteBranch also deletes the worktree's branch.
	DeleteBranch bool
	// Base is the branch unmerged commits are counted against.
	Base string
}

// LossError lists what removing a worktree would throw away.
type LossError struct {
	Path        string   `json:"path"`
	Branch      string   `json:"branch"`
	Base        string   `json:"base"`
	Uncommitted []string `json:"uncommitted,omitempty"`
	Unmerged    []Commit `json:"unmerged,omitempty"`
}

func (e *LossError) Error() string {
	var parts []string
	if n := len(e.Uncommitted); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s sin commitear (%s)", n, plural(n, "archivo", "archivos"), preview(e.Uncommitted, 5)))
	}
	if n := len(e.Unmerged); n > 0 {
		subjects := make([]string, 0, len(e.Unmerged))
		for _, c := range e.Unmerged {
			subjects = append(subjects, c.Short+" "+c.Subject)
		}
		parts = append(parts, fmt.Sprintf("%d %s de %s sin mergear en %s (%s)", n, plural(n, "commit", "commits"), e.Branch, e.Base, preview(subjects, 3)))
	}
	return "no se borra el worktree: se perderían " + strings.Join(parts, " y ") + "; usa force para borrarlo igualmente"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func preview(list []string, n int) string {
	if len(list) <= n {
		return strings.Join(list, ", ")
	}
	return strings.Join(list[:n], ", ") + fmt.Sprintf(", … +%d", len(list)-n)
}

// Losses reports what removing the worktree at path would lose: its
// uncommitted changes and, if the branch is deleted too, the commits not
// merged into base. It returns nil when nothing would be lost.
func Losses(ctx context.Context, repo, path, branch string, o RemoveOptions) (*LossError, error) {
	le := &LossError{Path: path, Branch: branch, Base: o.Base}
	if _, err := os.Stat(path); err == nil {
		st, err := GetStatus(ctx, path)
		if err != nil {
			return nil, err
		}
		le.Uncommitted = st.Files
	}
	if o.DeleteBranch && branch != "" && o.Base != "" {
		if BranchExists(ctx, repo, branch) {
			cs, err := CommitsAhead(ctx, repo, o.Base, "refs/heads/"+branch)
			if err != nil {
				return nil, err
			}
			le.Unmerged = cs
		}
	}
	if len(le.Uncommitted) == 0 && len(le.Unmerged) == 0 {
		return nil, nil
	}
	return le, nil
}

// RemoveWorktree removes the linked worktree at path from the repository
// containing repo (a directory of the main work tree). Without o.Force it
// refuses with a *LossError when that would lose uncommitted changes (or,
// with o.DeleteBranch, unmerged commits). A worktree whose directory is
// already gone is just pruned.
func RemoveWorktree(ctx context.Context, repo, path, branch string, o RemoveOptions) error {
	if !o.Force {
		le, err := Losses(ctx, repo, path, branch, o)
		if err != nil {
			return err
		}
		if le != nil {
			return le
		}
	}
	if _, err := os.Stat(path); err == nil {
		args := []string{"worktree", "remove"}
		if o.Force {
			args = append(args, "--force", "--force") // twice: also locked ones
		}
		if _, err := gitWrite(ctx, repo, append(args, "--", path)...); err != nil {
			return err
		}
	}
	if err := Prune(ctx, repo); err != nil {
		return err
	}
	if o.DeleteBranch && branch != "" && BranchExists(ctx, repo, branch) {
		// Unmerged commits were checked above (against Base, which is
		// what matters here, not whatever HEAD is): -D is safe.
		if _, err := gitWrite(ctx, repo, "branch", "-D", "--", branch); err != nil {
			return err
		}
	}
	return nil
}

// Prune drops administrative data of worktrees whose directory is gone.
func Prune(ctx context.Context, repo string) error {
	_, err := gitWrite(ctx, repo, "worktree", "prune")
	return err
}

// MergeResult describes a successful merge.
type MergeResult struct {
	Base    string `json:"base"`
	Branch  string `json:"branch"`
	Commits int    `json:"commits"`
	Output  string `json:"output,omitempty"`
	// Uncommitted: changes left in the worktree that were not merged.
	Uncommitted int `json:"uncommitted,omitempty"`
}

// Merge merges branch into base with `git merge --no-ff` in the main work
// tree, only when that tree is checked out on base and has no uncommitted
// changes to tracked files (no auto-stash). A conflicting merge is aborted
// and reported. worktree (optional) is the branch's worktree, to report
// changes that were not committed (and so are not merged).
func Merge(ctx context.Context, repo, branch, base, worktree string) (MergeResult, error) {
	res := MergeResult{Base: base, Branch: branch}
	main, err := MainWorktree(ctx, repo)
	if err != nil {
		return res, err
	}
	cur, err := CurrentBranch(ctx, main)
	if err != nil {
		return res, err
	}
	if cur != base {
		where := "con HEAD separado"
		if cur != "" {
			where = "en la rama " + cur
		}
		return res, fmt.Errorf("no se puede hacer merge: la carpeta principal (%s) está %s, no en %s; cámbiala a %s y vuelve a intentarlo", main, where, base, base)
	}
	st, err := GetStatus(ctx, main)
	if err != nil {
		return res, err
	}
	if n := st.Staged + st.Unstaged + st.Conflicts; n > 0 {
		return res, fmt.Errorf("no se puede hacer merge: la carpeta principal (%s) tiene %d %s sin commitear; haz commit o stash antes (agenterm no hace stash automático)", main, n, plural(n, "cambio", "cambios"))
	}
	if !BranchExists(ctx, main, branch) {
		return res, fmt.Errorf("la rama %q ya no existe", branch)
	}
	n, err := AheadCount(ctx, main, base, "refs/heads/"+branch)
	if err != nil {
		return res, err
	}
	res.Commits = n
	if worktree != "" {
		if wst, err := GetStatus(ctx, worktree); err == nil {
			res.Uncommitted = wst.Changed()
		}
	}
	if n == 0 {
		return res, fmt.Errorf("nada que mergear: %s no tiene commits que no estén ya en %s", branch, base)
	}
	out, err := gitWrite(ctx, main, "merge", "--no-ff", "--no-edit", "-m", fmt.Sprintf("Merge branch '%s' (agenterm)", branch), "--", branch)
	if err != nil {
		conflicts, _ := git(ctx, main, "diff", "--name-only", "--diff-filter=U")
		_, _ = gitWrite(ctx, main, "merge", "--abort")
		if c := splitLines(conflicts); len(c) > 0 {
			return res, fmt.Errorf("el merge de %s en %s tiene conflictos (%s): se ha abortado; resuélvelo a mano", branch, base, preview(c, 5))
		}
		return res, err
	}
	res.Output = out
	return res, nil
}
