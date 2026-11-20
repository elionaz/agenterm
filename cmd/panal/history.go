package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/protocol"
)

const historyUsage = `usage: panal history [--project P] [-n N] [words…]   search ended sessions (none: the latest)
       panal history show [--text] <id>
       panal history rm <id>
       panal history resume <id>`

// cmdHistory searches, shows, deletes or resumes archived sessions.
func cmdHistory(ctx context.Context, c *client.Client, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "show":
			return historyShow(ctx, c, args[1:])
		case "rm", "remove":
			if len(args) != 2 {
				return errors.New(historyUsage)
			}
			if err := c.HistoryDelete(ctx, args[1]); err != nil {
				return err
			}
			fmt.Println("removed from the history")
			return nil
		case "resume":
			if len(args) != 2 {
				return errors.New(historyUsage)
			}
			info, err := c.HistoryResume(ctx, protocol.HistoryResumeParams{ID: args[1]})
			if err != nil {
				return err
			}
			fmt.Printf("%s resumed as session %s (attach: panal attach %s)\n", args[1], info.ID, info.ID)
			return nil
		}
	}
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	project := fs.String("project", "", "only sessions of this project (id or name)")
	n := fs.Int("n", 20, "at most this many sessions")
	if err := parseInterspersed(fs, args); err != nil {
		return errors.New(historyUsage)
	}
	hits, err := c.HistorySearch(ctx, protocol.HistorySearchParams{Query: strings.Join(fs.Args(), " "), ProjectID: *project, Limit: *n})
	if err != nil {
		return err
	}
	if len(hits) == 0 {
		fmt.Println("no sessions found")
		return nil
	}
	// The table is aligned on its own; each row's matching lines go under
	// it, outside the columns.
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tENDED\tPROJECT\tAGENT\tNAME\tBRANCH")
	for _, h := range hits {
		e := h.Entry
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.ID, e.EndedAt.Local().Format("2006-01-02 15:04"), e.ProjectName, e.Agent, dash(e.Name), dash(first(e.Branches)))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	rows := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	fmt.Println(rows[0])
	for i, h := range hits {
		fmt.Println(rows[i+1])
		for _, sn := range h.Snippets {
			if sn.Source != "info" {
				fmt.Printf("    %s: %s\n", sn.Source, sn.Line)
			}
		}
	}
	return nil
}

func historyShow(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("history show", flag.ContinueOnError)
	text := fs.Bool("text", false, "also print the terminal text")
	if err := parseInterspersed(fs, args); err != nil || fs.NArg() != 1 {
		return errors.New(historyUsage)
	}
	d, err := c.HistoryGet(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	e := d.Entry
	fmt.Printf("%s  %s · %s", e.ID, e.ProjectName, e.Agent)
	if e.Name != "" {
		fmt.Printf(" · %s", e.Name)
	}
	if e.Model != "" {
		fmt.Printf(" · %s", e.Model)
	}
	fmt.Println()
	fmt.Printf("%s → %s (%s), exit %d\n", e.CreatedAt.Local().Format("2006-01-02 15:04"), e.EndedAt.Local().Format("15:04"),
		lasted(e.EndedAt.Sub(e.CreatedAt)), e.ExitCode)
	fmt.Printf("folder: %s\n", e.Cwd)
	if len(e.Branches) > 0 {
		fmt.Printf("branches: %s\n", strings.Join(e.Branches, ", "))
	}
	if d.CanResume {
		fmt.Printf("resume: panal history resume %s\n", e.ID)
	}
	if len(e.Prompts) > 0 {
		fmt.Printf("\nprompts (%d):\n", len(e.Prompts))
		for i, p := range e.Prompts {
			fmt.Printf("%3d. %s\n", i+1, indent(p))
		}
	}
	if *text {
		fmt.Printf("\n%s\n", d.Text)
	} else if e.TextBytes > 0 {
		fmt.Printf("\nterminal text: %s (--text to print it)\n", sizeLabel(e.TextBytes))
	}
	return nil
}

func indent(s string) string { return strings.ReplaceAll(s, "\n", "\n     ") }

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func first(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func sizeLabel(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KB", n/1024)
}

func lasted(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Minute).String()
}
