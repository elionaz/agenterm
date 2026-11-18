package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// gitlab drives glab. Commands name the project by its full URL (-R
// https://host/group/project), which also selects the host, and API calls
// pass --hostname for the same reason.
type gitlab struct{ *cli }

func (g *gitlab) Kind() string { return GitLab }
func (g *gitlab) Tool() string { return "glab" }
func (g *gitlab) repo() string { return g.remote.Web }

// project is the API path of the project (URL-encoded full path).
func (g *gitlab) project() string { return "projects/" + url.PathEscape(g.remote.Path) }

type glMR struct {
	IID                 int    `json:"iid"`
	Title               string `json:"title"`
	State               string `json:"state"` // opened | closed | merged | locked
	Draft               bool   `json:"draft"`
	WorkInProgress      bool   `json:"work_in_progress"`
	WebURL              string `json:"web_url"`
	TargetBranch        string `json:"target_branch"`
	SourceBranch        string `json:"source_branch"`
	HasConflicts        bool   `json:"has_conflicts"`
	DetailedMergeStatus string `json:"detailed_merge_status"`
	HeadPipeline        *struct {
		Status string `json:"status"`
	} `json:"head_pipeline"`
}

func (m glMR) toPR() *PR {
	pr := &PR{Number: m.IID, Title: m.Title, URL: m.WebURL, Base: m.TargetBranch, Head: m.SourceBranch,
		Conflicts: m.HasConflicts || m.DetailedMergeStatus == "conflict"}
	switch {
	case m.State == "merged":
		pr.State = StateMerged
	case m.State == "closed" || m.State == "locked":
		pr.State = StateClosed
	case m.Draft || m.WorkInProgress:
		pr.State = StateDraft
	default:
		pr.State = StateOpen
	}
	switch m.DetailedMergeStatus {
	case "not_approved", "requested_changes":
		pr.Blocked = true
		pr.Review = ReviewRequired
		if m.DetailedMergeStatus == "requested_changes" {
			pr.Review = ReviewChangesRequested
		}
	case "discussions_not_resolved", "blocked_status", "external_status_checks", "jira_association_missing", "need_rebase":
		pr.Blocked = true
	}
	var rs []checkResult
	if m.HeadPipeline != nil && m.HeadPipeline.Status != "" {
		state := ChecksPending
		switch m.HeadPipeline.Status {
		case "success", "skipped", "manual":
			state = ChecksPassing
		case "failed", "canceled":
			state = ChecksFailing
		}
		rs = append(rs, checkResult{"pipeline", state})
	}
	pr.Checks = summarize(rs)
	return pr
}

func (g *gitlab) Status(ctx context.Context, branch string) (*PR, error) {
	q := url.Values{"source_branch": {branch}, "state": {"all"}, "order_by": {"created_at"}, "sort": {"desc"}, "per_page": {"10"}}
	out, err := g.api(ctx, g.project()+"/merge_requests?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var mrs []glMR
	if err := json.Unmarshal(out, &mrs); err != nil {
		return nil, fmt.Errorf("glab api merge_requests: %w", err)
	}
	if len(mrs) == 0 {
		return nil, nil
	}
	// The list leaves out the pipeline and merge status details: ask for the
	// chosen one.
	pick := pickPR(mrs, func(m glMR) bool { return m.State == "opened" }, func(m glMR) *PR { return &PR{Number: m.IID} })
	one, err := g.api(ctx, g.project()+"/merge_requests/"+strconv.Itoa(pick.Number))
	if err != nil {
		return nil, err
	}
	var m glMR
	if err := json.Unmarshal(one, &m); err != nil {
		return nil, fmt.Errorf("glab api merge_request: %w", err)
	}
	return m.toPR(), nil
}

func (g *gitlab) api(ctx context.Context, path string, extra ...string) ([]byte, error) {
	return g.run(ctx, append([]string{"api", "--hostname", g.remote.Host}, append(extra, path)...)...)
}

func (g *gitlab) Comments(ctx context.Context, number int) ([]Comment, error) {
	out, err := g.api(ctx, g.project()+"/merge_requests/"+strconv.Itoa(number)+"/discussions?per_page=100")
	if err != nil {
		return nil, err
	}
	var ds []struct {
		Notes []struct {
			ID        int                       `json:"id"`
			Author    struct{ Username string } `json:"author"`
			Body      string                    `json:"body"`
			System    bool                      `json:"system"`
			CreatedAt time.Time                 `json:"created_at"`
			Position  *struct {
				NewPath string `json:"new_path"`
				NewLine int    `json:"new_line"`
				OldPath string `json:"old_path"`
				OldLine int    `json:"old_line"`
			} `json:"position"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(out, &ds); err != nil {
		return nil, fmt.Errorf("glab api discussions: %w", err)
	}
	web := g.remote.Web + "/-/merge_requests/" + strconv.Itoa(number)
	cs := []Comment{}
	for _, d := range ds {
		for _, n := range d.Notes {
			if n.System {
				continue
			}
			c := Comment{Kind: "comment", Author: n.Author.Username, Body: n.Body, At: n.CreatedAt, URL: web + "#note_" + strconv.Itoa(n.ID)}
			if p := n.Position; p != nil {
				c.Kind, c.Path, c.Line = "line", cmpOr(p.NewPath, p.OldPath), p.NewLine
				if c.Line == 0 {
					c.Line = p.OldLine
				}
			}
			cs = append(cs, c)
		}
	}
	sortComments(cs)
	return cs, nil
}

func (g *gitlab) MergeMethods(ctx context.Context) ([]string, error) {
	out, err := g.api(ctx, g.project())
	if err != nil {
		return nil, err
	}
	var v struct {
		SquashOption string `json:"squash_option"` // never | always | default_on | default_off
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("glab api project: %w", err)
	}
	switch v.SquashOption {
	case "never":
		return []string{"merge"}, nil
	case "always":
		return []string{"squash"}, nil
	case "default_on":
		return []string{"squash", "merge"}, nil
	}
	return []string{"merge", "squash"}, nil
}

func (g *gitlab) Merge(ctx context.Context, number int, method, _ string, deleteBranch bool) error {
	args := []string{"mr", "merge", strconv.Itoa(number), "-R", g.repo(), "--yes"}
	switch method {
	case "squash":
		args = append(args, "--squash")
	case "rebase":
		args = append(args, "--rebase")
	case "merge":
	default:
		return fmt.Errorf("unknown merge method %q (squash, merge or rebase)", method)
	}
	if deleteBranch {
		args = append(args, "--remove-source-branch")
	}
	_, err := g.run(ctx, args...)
	return err
}

func (g *gitlab) Create(ctx context.Context, p CreateParams) error {
	args := []string{"mr", "create", "-R", g.repo(), "--source-branch", p.Head, "--target-branch", p.Base,
		"--title", p.Title, "--description", p.Body, "--yes"}
	if p.Draft {
		args = append(args, "--draft")
	}
	_, err := g.run(ctx, args...)
	return err
}

// sortComments orders comments oldest first.
func sortComments(cs []Comment) {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].At.Before(cs[j].At) })
}
