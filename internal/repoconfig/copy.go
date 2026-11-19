package repoconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// CopyResult is what CopyInto did.
type CopyResult struct {
	Copied  []string // relative paths copied
	Skipped []string // already in the worktree (tracked by git, or copied before)
	Missing []string // entries that matched nothing in the main checkout
}

// CopyInto copies the entries (relative paths or globs, see
// Worktree.Copy) from the main checkout src into the worktree dst,
// keeping their relative paths. It never overwrites what dst already has.
// On macOS files are cloned (APFS clonefile), so a large folder costs
// almost nothing; elsewhere, or if cloning fails, they are copied.
func CopyInto(ctx context.Context, src, dst string, entries []string) (CopyResult, error) {
	var res CopyResult
	srcAbs, err := filepath.EvalSymlinks(src)
	if err != nil {
		return res, err
	}
	seen := map[string]bool{}
	for _, e := range entries {
		matches, err := filepath.Glob(filepath.Join(src, e))
		if err != nil {
			return res, fmt.Errorf("worktree.copy %q: %w", e, err)
		}
		if len(matches) == 0 {
			res.Missing = append(res.Missing, e)
			continue
		}
		sort.Strings(matches)
		for _, m := range matches {
			rel, err := filepath.Rel(src, m)
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") || rel == ".git" || strings.HasPrefix(rel, ".git/") {
				continue
			}
			if seen[rel] {
				continue
			}
			seen[rel] = true
			// What the entry resolves to must stay inside the checkout (a
			// symlink there could point anywhere).
			if real, err := filepath.EvalSymlinks(m); err != nil || !inside(srcAbs, real) {
				return res, fmt.Errorf("worktree.copy: %s resolves outside the repository", rel)
			}
			to := filepath.Join(dst, rel)
			if _, err := os.Lstat(to); err == nil {
				res.Skipped = append(res.Skipped, rel)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
				return res, err
			}
			if err := copyPath(ctx, m, to); err != nil {
				return res, fmt.Errorf("copy %s: %w", rel, err)
			}
			res.Copied = append(res.Copied, rel)
		}
	}
	return res, nil
}

func inside(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
}

// copyPath copies a file or a folder (recursively, symlinks kept as links)
// from to to, which must not exist.
func copyPath(ctx context.Context, from, to string) error {
	if runtime.GOOS == "darwin" {
		if err := exec.CommandContext(ctx, "cp", "-Rc", from, to).Run(); err == nil {
			return nil
		}
		_ = os.RemoveAll(to) // a clone that failed halfway
	}
	out, err := exec.CommandContext(ctx, "cp", "-R", from, to).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// Suggest lists what a repository without .agenterm.yaml probably wants
// copied into new worktrees: the .env* files at the root of its main
// checkout that git ignores (so a worktree never has them).
func Suggest(ctx context.Context, root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, ".env*"))
	var rel []string
	for _, m := range matches {
		if fi, err := os.Stat(m); err != nil || fi.IsDir() {
			continue
		}
		rel = append(rel, filepath.Base(m))
	}
	if len(rel) == 0 {
		return nil
	}
	sort.Strings(rel)
	// check-ignore prints the ignored ones (exit 1: none is).
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root, "check-ignore", "--"}, rel...)...)
	out, _ := cmd.Output()
	var ignored []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			ignored = append(ignored, l)
		}
	}
	return ignored
}
