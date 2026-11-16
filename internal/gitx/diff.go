package gitx

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Diff limits.
const (
	// DefaultMaxPatch caps the patch text of a diff (all files together).
	DefaultMaxPatch = 2 << 20
	// maxUntracked: untracked files beyond this are listed without content.
	maxUntracked = 300
	// maxUntrackedSize: bigger untracked files are listed without content.
	maxUntrackedSize = 1 << 20
)

// DiffFile summarises one changed path.
type DiffFile struct {
	Path    string `json:"path"`
	OldPath string `json:"oldPath,omitempty"` // renames and copies
	// Status: A (added), M (modified), D (deleted), R (renamed), C (copied),
	// T (type change), U (unmerged).
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
	// Patch is this file's unified diff (empty for binary files, and when
	// the size cap was reached: see Truncated).
	Patch     string `json:"patch,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// DiffOptions configure WorkingDiff.
type DiffOptions struct {
	// From is the commit (or tree) the work tree is compared with. Empty
	// means HEAD (or the empty tree in a repository without commits).
	From string
	// MaxPatch caps the total patch size (0 = DefaultMaxPatch).
	MaxPatch int
	// StatOnly skips the patch text.
	StatOnly bool
}

// DiffResult is a work tree's diff against DiffOptions.From.
type DiffResult struct {
	From      string     `json:"from"`
	Files     []DiffFile `json:"files"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	// Truncated: the patch text hit the size cap; some files have no
	// Patch (their Truncated flag is set).
	Truncated bool `json:"truncated,omitempty"`
	PatchSize int  `json:"patchSize"`
}

// Patch concatenates every file's patch.
func (d DiffResult) Patch() string {
	var b strings.Builder
	for _, f := range d.Files {
		b.WriteString(f.Patch)
	}
	return b.String()
}

// WorkingDiff diffs the work tree at dir (tracked changes, staged or not,
// plus untracked files that are not ignored) against o.From. For a
// worktree session From is the merge base with its base branch, so the
// result is "committed + uncommitted" changes of the session in one list.
func WorkingDiff(ctx context.Context, dir string, o DiffOptions) (DiffResult, error) {
	if o.MaxPatch <= 0 {
		o.MaxPatch = DefaultMaxPatch
	}
	if o.From == "" {
		o.From = "HEAD"
		if !HasCommits(ctx, dir) {
			o.From = EmptyTree
		}
	}
	res := DiffResult{From: o.From, Files: []DiffFile{}}
	base := []string{"diff", "--no-color", "--no-ext-diff", "--find-renames", "--ignore-submodules=dirty"}

	ns, _, err := run(ctx, dir, runOpts{}, append(base, "--name-status", "-z", o.From, "--")...)
	if err != nil {
		return res, err
	}
	files := parseNameStatus(ns)
	num, _, err := run(ctx, dir, runOpts{}, append(base, "--numstat", "-z", o.From, "--")...)
	if err != nil {
		return res, err
	}
	applyNumstat(files, num)

	budget := o.MaxPatch
	if !o.StatOnly && len(files) > 0 {
		patch, truncated, err := run(ctx, dir, runOpts{limit: budget}, append(base, "--patch", o.From, "--")...)
		if err != nil {
			return res, err
		}
		chunks := splitPatch(patch)
		if truncated && len(chunks) > 0 {
			chunks = chunks[:len(chunks)-1] // the last one is cut
			res.Truncated = true
		}
		if len(chunks) <= len(files) && (truncated || len(chunks) == len(files)) {
			// Same diff queue, same order: chunk i is file i.
			for i, c := range chunks {
				if !files[i].Binary {
					files[i].Patch = c
				}
				budget -= len(c)
			}
		} else {
			// Type changes are split in two chunks: match by path.
			idx := make(map[string]int, len(files))
			for i, f := range files {
				idx[f.Path] = i
			}
			for _, c := range chunks {
				if i, ok := idx[chunkPath(c)]; ok && !files[i].Binary {
					files[i].Patch += c
				}
				budget -= len(c)
			}
		}
		if truncated {
			for i := range files {
				if files[i].Patch == "" && !files[i].Binary {
					files[i].Truncated = true
				}
			}
		}
		if truncated {
			budget = 0
		}
	}

	untracked, err := untrackedFiles(ctx, dir)
	if err != nil {
		return res, err
	}
	for i, path := range untracked {
		f := DiffFile{Path: path, Status: "A", Untracked: true}
		full := filepath.Join(dir, path)
		fi, err := os.Lstat(full)
		switch {
		case err != nil:
		case i >= maxUntracked || (fi.Mode().IsRegular() && fi.Size() > maxUntrackedSize):
			f.Truncated = true
			if fi.Mode().IsRegular() {
				f.Binary = isBinaryFile(full)
			}
			res.Truncated = res.Truncated || !f.Binary
		default:
			out, _, err := run(ctx, dir, runOpts{okCodes: []int{1}}, "diff", "--no-color", "--no-ext-diff", "--no-index", "--", "/dev/null", path)
			if err != nil {
				break
			}
			patch := string(out)
			if strings.Contains(patch, "\nBinary files ") || strings.HasPrefix(patch, "Binary files ") || strings.Contains(patch, "GIT binary patch") {
				f.Binary = true
				break
			}
			f.Additions = countAdded(patch)
			switch {
			case o.StatOnly:
			case len(patch) > budget:
				f.Truncated = true
				res.Truncated = true
				budget = 0
			default:
				f.Patch = patch
				budget -= len(patch)
			}
		}
		files = append(files, f)
	}

	for _, f := range files {
		res.Additions += f.Additions
		res.Deletions += f.Deletions
		res.PatchSize += len(f.Patch)
	}
	res.Files = files
	return res, nil
}

// ChangedFiles counts the paths WorkingDiff would list (cheaper: no
// numstat nor patch).
func ChangedFiles(ctx context.Context, dir, from string) (int, error) {
	if from == "" {
		from = "HEAD"
		if !HasCommits(ctx, dir) {
			from = EmptyTree
		}
	}
	out, _, err := run(ctx, dir, runOpts{}, "diff", "--no-color", "--no-ext-diff", "--find-renames", "--ignore-submodules=dirty", "--name-only", "-z", from, "--")
	if err != nil {
		return 0, err
	}
	n := len(splitZ(out))
	u, err := untrackedFiles(ctx, dir)
	return n + len(u), err
}

func untrackedFiles(ctx context.Context, dir string) ([]string, error) {
	out, _, err := run(ctx, dir, runOpts{}, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

func splitZ(b []byte) []string {
	var out []string
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}

// parseNameStatus parses `--name-status -z`: "M\0path\0" or
// "R100\0old\0new\0".
func parseNameStatus(b []byte) []DiffFile {
	f := splitZ(b)
	var out []DiffFile
	for i := 0; i < len(f); i++ {
		st := f[i]
		if st == "" {
			continue
		}
		d := DiffFile{Status: st[:1]}
		if (d.Status == "R" || d.Status == "C") && i+2 < len(f) {
			d.OldPath, d.Path = f[i+1], f[i+2]
			i += 2
		} else if i+1 < len(f) {
			d.Path = f[i+1]
			i++
		}
		out = append(out, d)
	}
	return out
}

// applyNumstat fills additions/deletions/binary from `--numstat -z`:
// "add\tdel\tpath\0" or, for renames, "add\tdel\t\0old\0new\0". Entries are
// matched by (new) path.
func applyNumstat(files []DiffFile, b []byte) {
	idx := make(map[string]int, len(files))
	for i, f := range files {
		idx[f.Path] = i
	}
	recs := bytes.Split(b, []byte{0})
	for i := 0; i < len(recs); i++ {
		r := string(recs[i])
		if r == "" {
			continue
		}
		parts := strings.SplitN(r, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		path := parts[2]
		if path == "" && i+2 < len(recs) { // rename: old\0new follow
			path = string(recs[i+2])
			i += 2
		}
		j, ok := idx[path]
		if !ok {
			continue
		}
		if parts[0] == "-" && parts[1] == "-" {
			files[j].Binary = true
			continue
		}
		files[j].Additions, _ = strconv.Atoi(parts[0])
		files[j].Deletions, _ = strconv.Atoi(parts[1])
	}
}

// splitPatch cuts a multi-file patch at each "diff --git " header (content
// lines always start with ' ', '+', '-' or '\', so this is unambiguous).
func splitPatch(p []byte) []string {
	if len(p) == 0 {
		return nil
	}
	var out []string
	s := string(p)
	start := 0
	for {
		i := strings.Index(s[start+1:], "\ndiff --git ")
		if i < 0 {
			out = append(out, s[start:])
			return out
		}
		end := start + 1 + i + 1
		out = append(out, s[start:end])
		start = end
	}
}

// chunkPath is the new path in a "diff --git a/x b/y" header.
func chunkPath(c string) string {
	head, _, _ := strings.Cut(c, "\n")
	if i := strings.LastIndex(head, " b/"); i >= 0 {
		return head[i+3:]
	}
	return ""
}

// countAdded counts "+" lines of a new-file patch (not the "+++" header).
func countAdded(patch string) int {
	n := 0
	inHunk := false
	for _, l := range strings.Split(patch, "\n") {
		if strings.HasPrefix(l, "@@") {
			inHunk = true
			continue
		}
		if inHunk && strings.HasPrefix(l, "+") {
			n++
		}
	}
	return n
}

// isBinaryFile applies git's heuristic: a NUL byte in the first 8000 bytes.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}
