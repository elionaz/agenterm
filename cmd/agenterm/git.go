package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/gitx"
	"github.com/elionaz/agenterm/internal/protocol"
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
		return errors.New("usage: agenterm diff [--stat] <sessionId|worktreeId>")
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
				fmt.Fprintf(os.Stderr, "agenterm: %s: sin contenido (límite de tamaño)\n", f.Path)
			}
		}
		if d.Truncated {
			fmt.Fprintln(os.Stderr, "agenterm: diff truncado (límite de 2 MB); usa git en", d.Dir)
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
			counts = "binario"
		}
		extra := ""
		if f.Untracked {
			extra = "(sin seguimiento)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.Status, name, counts, extra)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("%d %s, +%d -%d", len(d.Files), plural(len(d.Files), "archivo", "archivos"), d.Additions, d.Deletions)
	if d.Git.Ahead > 0 {
		fmt.Printf(", %d %s por delante", d.Git.Ahead, plural(d.Git.Ahead, "commit", "commits"))
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
		return errors.New("usage: agenterm worktree rm|merge|open <id>")
	}
	switch args[0] {
	case "rm", "remove":
		fs := flag.NewFlagSet("worktree rm", flag.ContinueOnError)
		force := fs.Bool("force", false, "remove even if uncommitted changes (or unmerged commits) would be lost")
		delBranch := fs.Bool("delete-branch", false, "also delete the branch")
		dry := fs.Bool("dry-run", false, "only report what would be lost")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: agenterm worktree rm [--force] [--delete-branch] [--dry-run] <id>")
		}
		res, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: fs.Arg(0), Force: *force, DeleteBranch: *delBranch, DryRun: *dry})
		if err != nil {
			return err
		}
		if *dry {
			if res.Loss == nil {
				fmt.Println("no se perdería nada")
			} else {
				printLoss(res.Loss)
			}
			return nil
		}
		fmt.Println("worktree eliminado")
		return nil
	case "merge":
		if len(args) != 2 {
			return errors.New("usage: agenterm worktree merge <id>")
		}
		res, err := c.WorktreeMerge(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%s mergeada en %s (%d %s)\n", res.Branch, res.Base, res.Commits, plural(res.Commits, "commit", "commits"))
		if res.Uncommitted > 0 {
			fmt.Printf("ojo: %d %s sin commitear en el worktree no se han incluido\n", res.Uncommitted, plural(res.Uncommitted, "cambio", "cambios"))
		}
		return nil
	case "open":
		fs := flag.NewFlagSet("worktree open", flag.ContinueOnError)
		finder := fs.Bool("finder", false, "open in Finder instead of the editor")
		if err := parseInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: agenterm worktree open [--finder] <id>")
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
		fmt.Printf("%d %s sin commitear:\n", len(l.Uncommitted), plural(len(l.Uncommitted), "archivo", "archivos"))
		for _, f := range l.Uncommitted {
			fmt.Println("  " + f)
		}
	}
	if len(l.Unmerged) > 0 {
		fmt.Printf("%d %s de %s sin mergear en %s:\n", len(l.Unmerged), plural(len(l.Unmerged), "commit", "commits"), l.Branch, l.Base)
		for _, cm := range l.Unmerged {
			fmt.Printf("  %s %s\n", cm.Short, cm.Subject)
		}
	}
}
