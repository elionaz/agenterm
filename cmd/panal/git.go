package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/protocol"
)

// gitCol renders "agent/x ±3 ↑1" (branch, changed files, commits ahead).
func gitCol(branch string, g *protocol.GitInfo) string {
	var parts []string
	if branch != "" {
		parts = append(parts, branch)
	}
	if g != nil {
		parts = append(parts, fmt.Sprintf("±%d", g.Files))
		if g.Ahead > 0 {
			parts = append(parts, fmt.Sprintf("↑%d", g.Ahead))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

// cmdDiff prints a session's (or worktree's) diff: the unified patch, or a
// per-file summary with --stat.
func cmdDiff(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	stat := fs.Bool("stat", false, "only the per-file summary")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: panal diff [--stat] <sessionId|worktreeId>")
	}
	id := fs.Arg(0)
	d, err := c.SessionDiff(ctx, id, *stat)
	if err != nil && client.IsRemote(err) && strings.Contains(err.Error(), "session not found") {
		d, err = c.WorktreeDiff(ctx, id, *stat)
	}
	if err != nil {
		return err
	}
	if !*stat {
		fmt.Print(d.Patch())
		for _, f := range d.Files {
			if f.Truncated {
				fmt.Fprintf(os.Stderr, "panal: %s: no content (size limit)\n", f.Path)
			}
		}
		if d.Truncated {
			fmt.Fprintln(os.Stderr, "panal: diff truncated (2 MB limit); use git in", d.Dir)
		}
		return nil
	}
	head := d.Dir
	if d.Branch != "" {
		head += " (" + d.Branch
		if d.Base != "" {
			head += " ← " + d.Base
		}
		head += ")"
	}
	fmt.Println(head)
	for _, cm := range d.Commits {
		fmt.Printf("  %s %s\n", cm.Short, cm.Subject)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, f := range d.Files {
		name := f.Path
		if f.OldPath != "" {
			name = f.OldPath + " → " + f.Path
		}
		counts := fmt.Sprintf("+%d -%d", f.Additions, f.Deletions)
		if f.Binary {
			counts = "binary"
		}
		extra := ""
		if f.Untracked {
			extra = "(untracked)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Status, name, counts, extra)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("%d %s, +%d -%d", len(d.Files), plural(len(d.Files), "file", "files"), d.Additions, d.Deletions)
	if d.Git.Ahead > 0 {
		fmt.Printf(", %d %s ahead", d.Git.Ahead, plural(d.Git.Ahead, "commit", "commits"))
	}
	fmt.Println()
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func cmdWorktrees(ctx context.Context, c *client.Client, project string) error {
	wts, err := c.Worktrees(ctx, project)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tBRANCH\tBASE\tSTATE\tGIT\tSESSIONS\tPATH")
	for _, wt := range wts {
		state := "exited"
		switch {
		case wt.Live:
			state = "live"
		case wt.Orphan:
			state = "orphan"
		}
		sessions := strings.Join(wt.SessionIDs, ",")
		if sessions == "" {
			sessions = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", wt.ID, wt.Branch, wt.Base, state, gitCol("", wt.Git), sessions, wt.Path)
	}
	return w.Flush()
}

func cmdWorktree(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: panal worktree rm|merge|pr|open <id>")
	}
	switch args[0] {
	case "rm", "remove":
		fs := flag.NewFlagSet("worktree rm", flag.ContinueOnError)
		force := fs.Bool("force", false, "remove even if uncommitted changes (or unmerged commits) would be lost")
		delBranch := fs.Bool("delete-branch", false, "also delete the branch")
		dry := fs.Bool("dry-run", false, "only report what would be lost (with --merged: what would be removed)")
		merged := fs.Bool("merged", false, "remove a project's orphaned worktrees already merged, with their branches")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if *merged {
			if fs.NArg() != 1 || *force || *delBranch {
				return errors.New("usage: panal worktree rm --merged [--dry-run] <projectId|name>")
			}
			return removeMerged(ctx, c, fs.Arg(0), *dry)
		}
		if fs.NArg() != 1 {
			return errors.New("usage: panal worktree rm [--force] [--delete-branch] [--dry-run] <id>\n       panal worktree rm --merged [--dry-run] <projectId|name>")
		}
		res, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: fs.Arg(0), Force: *force, DeleteBranch: *delBranch, DryRun: *dry})
		if err != nil {
			return err
		}
		if *dry {
			if res.Loss == nil {
				fmt.Println("nothing would be lost")
			} else {
				printLoss(res.Loss)
			}
			return nil
		}
		fmt.Println("worktree removed")
		return nil
	case "merge":
		if len(args) != 2 {
			return errors.New("usage: panal worktree merge <id>")
		}
		res, err := c.WorktreeMerge(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%s merged into %s (%d %s)\n", res.Branch, res.Base, res.Commits, plural(res.Commits, "commit", "commits"))
		if res.Uncommitted > 0 {
			fmt.Printf("note: %d uncommitted %s in the worktree were not included\n", res.Uncommitted, plural(res.Uncommitted, "change", "changes"))
		}
		return nil
	case "pr":
		fs := flag.NewFlagSet("worktree pr", flag.ContinueOnError)
		noOpen := fs.Bool("no-open", false, "print the pull request URL without opening it")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: panal worktree pr [--no-open] <id>")
		}
		res, err := c.WorktreePR(ctx, fs.Arg(0), false)
		if err != nil {
			return err
		}
		fmt.Printf("%s pushed to %s (%d %s ahead of %s)\n", res.Branch, res.Remote, res.Commits, plural(res.Commits, "commit", "commits"), res.Base)
		if res.Uncommitted > 0 {
			fmt.Printf("note: %d uncommitted %s in the worktree were not pushed\n", res.Uncommitted, plural(res.Uncommitted, "change", "changes"))
		}
		if res.URL == "" {
			fmt.Printf("open a pull request for %s on your git host\n", res.Branch)
			return nil
		}
		fmt.Println(res.URL)
		if !*noOpen {
			if err := exec.Command("open", res.URL).Run(); err != nil {
				fmt.Fprintln(os.Stderr, "panal: could not open the browser:", err)
			}
		}
		return nil
	case "open":
		fs := flag.NewFlagSet("worktree open", flag.ContinueOnError)
		finder := fs.Bool("finder", false, "open in Finder instead of the editor")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: panal worktree open [--finder] <id>")
		}
		target := gitx.OpenEditor
		if *finder {
			target = gitx.OpenFinder
		}
		return c.WorktreeOpen(ctx, protocol.WorktreeOpenParams{ID: fs.Arg(0), Target: target})
	}
	return fmt.Errorf("unknown worktree subcommand %q", args[0])
}

func printLoss(l *gitx.LossError) {
	if len(l.Uncommitted) > 0 {
		fmt.Printf("%d uncommitted %s:\n", len(l.Uncommitted), plural(len(l.Uncommitted), "file", "files"))
		for _, f := range l.Uncommitted {
			fmt.Println("  " + f)
		}
	}
	if len(l.Unmerged) > 0 {
		fmt.Printf("%d %s on %s not merged into %s:\n", len(l.Unmerged), plural(len(l.Unmerged), "commit", "commits"), l.Branch, l.Base)
		for _, cm := range l.Unmerged {
			fmt.Printf("  %s %s\n", cm.Short, cm.Subject)
		}
	}
}

// removeMerged removes (or, dry, lists) a project's orphaned worktrees
// whose branch is already merged.
func removeMerged(ctx context.Context, c *client.Client, project string, dry bool) error {
	res, err := c.WorktreeRemoveMerged(ctx, protocol.WorktreeRemoveMergedParams{ProjectID: project, DryRun: dry})
	if err != nil {
		return err
	}
	verb := "removed"
	if dry {
		verb = "would remove"
	}
	for _, w := range res.Removed {
		fmt.Printf("%s %s (%s)\n", verb, w.ID, w.Branch)
	}
	for _, sk := range res.Skipped {
		fmt.Printf("kept %s (%s): %s\n", sk.Worktree.ID, sk.Worktree.Branch, sk.Reason)
	}
	if len(res.Removed) == 0 && len(res.Skipped) == 0 {
		fmt.Println("no merged orphaned worktrees")
	}
	return nil
}
