package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/elionaz/panal/internal/history"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/protocol"
)

// Backend is what the tools ask the daemon (a *client.Client).
type Backend interface {
	MsgPeers(ctx context.Context, sessionID string) (protocol.MsgPeers, error)
	MsgSend(ctx context.Context, p protocol.MsgSendParams) ([]mailbox.Message, error)
	MsgTake(ctx context.Context, intro bool) (protocol.MsgTakeResult, error)
	WaitIdle(ctx context.Context, id string) (string, error)
	HistorySearch(ctx context.Context, p protocol.HistorySearchParams) ([]history.Hit, error)
	HistoryGet(ctx context.Context, id string) (protocol.HistoryDetail, error)
	SessionDiff(ctx context.Context, id string, stat bool) (protocol.DiffResult, error)
}

// Limits of the tools' answers.
const (
	defaultWait   = 5 * time.Minute
	maxWait       = 10 * time.Minute
	maxHistoryHit = 50
	// MaxHistoryText bounds the terminal text history_get returns (its end
	// is kept: that is where a session's outcome is).
	MaxHistoryText = 50 << 10
	maxListedFiles = 100
)

type tool struct {
	name, title, desc string
	schema            map[string]any
	readOnly          bool
	run               func(ctx context.Context, b Backend, args json.RawMessage) (text string, data any, err error)
}

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func decode[T any](args json.RawMessage) (T, error) {
	var v T
	if err := json.Unmarshal(args, &v); err != nil {
		return v, fmt.Errorf("invalid arguments: %w", err)
	}
	return v, nil
}

func toolset() []tool {
	return []tool{
		{
			name: "peers", title: "Other sessions", readOnly: true,
			desc:   "List the other agent sessions of this panal project: id, name, agent, state (working, idle…), what they are doing and their branch or folder. Use it to find who to message or wait for.",
			schema: object(map[string]any{}),
			run:    runPeers,
		},
		{
			name: "msg_send", title: "Send a message",
			desc: "Send a message to another session of this project (to: its id, its name, or \"all\"). " +
				"Message only when the other session has to act or you need its answer; no status updates or thanks. " +
				"In a project whose messages are manual, it waits until the user delivers it.",
			schema: object(map[string]any{
				"to":   str("a session id or name (see peers), or \"all\""),
				"body": str("the message"),
			}, "to", "body"),
			run: runSend,
		},
		{
			name: "msg_read", title: "Read your messages",
			desc:   "Read your new messages from the other sessions (and from the user) and mark them delivered. Messages also arrive by themselves between turns: use this when you are told you have some, or while waiting for an answer.",
			schema: object(map[string]any{}),
			run:    runRead,
		},
		{
			name: "wait", title: "Wait for a session", readOnly: true,
			desc: "Block until another session finishes its turn (idle), needs its user, or exits; then report its state. " +
				"Use it after asking a session for something, then msg_read for its answer.",
			schema: object(map[string]any{
				"session":    str("the session id or name"),
				"timeoutSec": map[string]any{"type": "integer", "minimum": 1, "maximum": int(maxWait / time.Second), "description": "give up after this many seconds (default 300, at most 600)"},
			}, "session"),
			run: runWait,
		},
		{
			name: "session_status", title: "Session status", readOnly: true,
			desc: "Show a session's state and its changes: branch, files changed, commits ahead of its base, uncommitted work, and whether its branch is already merged. Without session: your own.",
			schema: object(map[string]any{
				"session": str("a session id or name (default: yours)"),
			}),
			run: runStatus,
		},
		{
			name: "history_search", title: "Search past sessions", readOnly: true,
			desc: "Search the sessions that ended (their names, prompts and terminal text), in this project unless allProjects. Every word must appear; an empty query lists the most recent.",
			schema: object(map[string]any{
				"query":       str("words to look for"),
				"allProjects": map[string]any{"type": "boolean", "description": "search every project, not only this one"},
				"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxHistoryHit, "description": "at most this many sessions (default 10)"},
			}),
			run: runHistorySearch,
		},
		{
			name: "history_get", title: "Read a past session", readOnly: true,
			desc:   "Read one ended session (an id from history_search): what it was, its prompts, and the end of its terminal text.",
			schema: object(map[string]any{"id": str("the session id")}, "id"),
			run:    runHistoryGet,
		},
	}
}

func runPeers(ctx context.Context, b Backend, _ json.RawMessage) (string, any, error) {
	r, err := b.MsgPeers(ctx, "")
	if err != nil {
		return "", nil, err
	}
	return formatPeers(r), r, nil
}

func formatPeers(r protocol.MsgPeers) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "project %s (%s)\n", r.ProjectName, r.ProjectID)
	if r.Self != nil {
		fmt.Fprintf(&sb, "you: %s\n", peerLine(*r.Self))
	}
	if len(r.Peers) == 0 {
		sb.WriteString("no other sessions\n")
	}
	for _, p := range r.Peers {
		fmt.Fprintf(&sb, "- %s\n", peerLine(p))
	}
	if !r.Enabled {
		sb.WriteString("messages between sessions are off (panal Settings › Messages)\n")
	}
	return sb.String()
}

func peerLine(p protocol.MsgPeer) string {
	s := fmt.Sprintf("%s %q [%s] %s", p.ID, p.Name, p.ProfileID, p.State)
	switch {
	case p.Mode == protocol.ModeWorktree && p.Branch != "":
		s += ", worktree on branch " + p.Branch
	case p.SameFolder:
		s += ", same folder as you"
	}
	if p.Activity != "" {
		s += ", " + p.Activity
	}
	return s
}

func runSend(ctx context.Context, b Backend, args json.RawMessage) (string, any, error) {
	a, err := decode[struct {
		To   string `json:"to"`
		Body string `json:"body"`
	}](args)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(a.To) == "" || strings.TrimSpace(a.Body) == "" {
		return "", nil, errors.New("to and body are required")
	}
	ms, err := b.MsgSend(ctx, protocol.MsgSendParams{To: a.To, Body: a.Body})
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	type sent struct {
		ID     int    `json:"id"`
		To     string `json:"to"`
		ToName string `json:"toName"`
		Held   bool   `json:"held,omitempty"`
	}
	out := make([]sent, 0, len(ms))
	for _, m := range ms {
		fmt.Fprintf(&sb, "sent #%d to %s (%s)", m.ID, m.ToName, m.To)
		if m.Held {
			sb.WriteString(": held until your user delivers it (this project's messages are manual)")
		}
		sb.WriteString("\n")
		out = append(out, sent{m.ID, m.To, m.ToName, m.Held})
	}
	return sb.String(), map[string]any{"sent": out}, nil
}

func runRead(ctx context.Context, b Backend, _ json.RawMessage) (string, any, error) {
	r, err := b.MsgTake(ctx, false)
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	switch {
	case !r.Enabled:
		sb.WriteString("messages between sessions are off (panal Settings › Messages)\n")
	case len(r.Messages) == 0:
		sb.WriteString("no new messages\n")
	default:
		sb.WriteString(mailbox.Preamble + "\n\n" + mailbox.ForAgent(r.Messages))
	}
	if r.Enabled && r.Held > 0 {
		fmt.Fprintf(&sb, "%d more waiting for your user to deliver (this project's messages are manual)\n", r.Held)
	}
	return sb.String(), map[string]any{"messages": r.Messages, "held": r.Held, "enabled": r.Enabled}, nil
}

// resolve turns a session name into its id (ids pass through); "" is the
// calling session.
func resolve(ctx context.Context, b Backend, who string) (protocol.MsgPeer, error) {
	r, err := b.MsgPeers(ctx, "")
	if err != nil {
		return protocol.MsgPeer{}, err
	}
	if who == "" {
		if r.Self == nil {
			return protocol.MsgPeer{}, errors.New("this is not a panal session: name one")
		}
		return *r.Self, nil
	}
	all := r.Peers
	if r.Self != nil {
		all = append([]protocol.MsgPeer{*r.Self}, all...)
	}
	var byName []protocol.MsgPeer
	for _, p := range all {
		if p.ID == who {
			return p, nil
		}
		if strings.EqualFold(p.Name, who) {
			byName = append(byName, p)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return protocol.MsgPeer{}, fmt.Errorf("no session %q in this project (see peers)", who)
	}
	return protocol.MsgPeer{}, fmt.Errorf("several sessions are named %q: use its id (see peers)", who)
}

func runWait(ctx context.Context, b Backend, args json.RawMessage) (string, any, error) {
	a, err := decode[struct {
		Session    string `json:"session"`
		TimeoutSec int    `json:"timeoutSec"`
	}](args)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(a.Session) == "" {
		return "", nil, errors.New("session is required")
	}
	p, err := resolve(ctx, b, a.Session)
	if err != nil {
		return "", nil, err
	}
	d := defaultWait
	if a.TimeoutSec > 0 {
		d = min(time.Duration(a.TimeoutSec)*time.Second, maxWait)
	}
	wctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	st, err := b.WaitIdle(wctx, p.ID)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return fmt.Sprintf("%s (%s) is still busy after %s", p.Name, p.ID, d),
			map[string]any{"session": p.ID, "state": "busy", "timedOut": true}, nil
	}
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("%s (%s) is %s", p.Name, p.ID, st), map[string]any{"session": p.ID, "state": st}, nil
}

func runStatus(ctx context.Context, b Backend, args json.RawMessage) (string, any, error) {
	a, err := decode[struct {
		Session string `json:"session"`
	}](args)
	if err != nil {
		return "", nil, err
	}
	p, err := resolve(ctx, b, strings.TrimSpace(a.Session))
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", peerLine(p))
	data := map[string]any{"session": p}
	d, err := b.SessionDiff(ctx, p.ID, true)
	if err != nil {
		fmt.Fprintf(&sb, "no changes to show: %v\n", err)
		return sb.String(), data, nil
	}
	if d.Branch != "" {
		fmt.Fprintf(&sb, "branch %s", d.Branch)
		if d.Base != "" {
			fmt.Fprintf(&sb, " (base %s)", d.Base)
		}
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "%d files changed (+%d −%d), %d uncommitted, %d commits ahead",
		len(d.Files), d.Additions, d.Deletions, d.Git.Uncommitted, d.Git.Ahead)
	if d.Git.Merged {
		sb.WriteString(", already merged into its base")
	}
	sb.WriteString("\n")
	for i, f := range d.Files {
		if i == maxListedFiles {
			fmt.Fprintf(&sb, "  … %d more\n", len(d.Files)-i)
			break
		}
		fmt.Fprintf(&sb, "  %s %s (+%d −%d)\n", f.Status, f.Path, f.Additions, f.Deletions)
	}
	for _, c := range d.Commits {
		fmt.Fprintf(&sb, "  commit %s %s\n", c.Short, c.Subject)
	}
	files := make([]map[string]any, 0, min(len(d.Files), maxListedFiles))
	for _, f := range d.Files[:min(len(d.Files), maxListedFiles)] {
		files = append(files, map[string]any{"path": f.Path, "status": f.Status, "additions": f.Additions, "deletions": f.Deletions})
	}
	data["branch"], data["base"], data["git"], data["files"], data["commits"] = d.Branch, d.Base, d.Git, files, d.Commits
	return sb.String(), data, nil
}

func runHistorySearch(ctx context.Context, b Backend, args json.RawMessage) (string, any, error) {
	a, err := decode[struct {
		Query       string `json:"query"`
		AllProjects bool   `json:"allProjects"`
		Limit       int    `json:"limit"`
	}](args)
	if err != nil {
		return "", nil, err
	}
	q := protocol.HistorySearchParams{Query: a.Query, Limit: 10}
	if a.Limit > 0 {
		q.Limit = min(a.Limit, maxHistoryHit)
	}
	if !a.AllProjects {
		r, err := b.MsgPeers(ctx, "")
		if err != nil {
			return "", nil, err
		}
		q.ProjectID = r.ProjectID
	}
	hits, err := b.HistorySearch(ctx, q)
	if err != nil {
		return "", nil, err
	}
	var sb strings.Builder
	if len(hits) == 0 {
		sb.WriteString("no past sessions match\n")
	}
	for _, h := range hits {
		e := h.Entry
		name := e.Name
		if name == "" {
			name = e.Agent
		}
		fmt.Fprintf(&sb, "%s %q [%s] project %s, ended %s", e.ID, name, e.Agent, e.ProjectName, e.EndedAt.Local().Format("2006-01-02 15:04"))
		if len(e.Branches) > 0 {
			fmt.Fprintf(&sb, ", branch %s", e.Branches[0])
		}
		sb.WriteString("\n")
		for _, sn := range h.Snippets {
			fmt.Fprintf(&sb, "  %s: %s\n", sn.Source, sn.Line)
		}
	}
	return sb.String(), map[string]any{"hits": hits}, nil
}

func runHistoryGet(ctx context.Context, b Backend, args json.RawMessage) (string, any, error) {
	a, err := decode[struct {
		ID string `json:"id"`
	}](args)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(a.ID) == "" {
		return "", nil, errors.New("id is required")
	}
	d, err := b.HistoryGet(ctx, a.ID)
	if err != nil {
		return "", nil, err
	}
	e := d.Entry
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %q [%s] project %s\nfolder %s\n%s → %s, exit %d\n", e.ID, e.Name, e.Agent, e.ProjectName, e.Cwd,
		e.CreatedAt.Local().Format("2006-01-02 15:04"), e.EndedAt.Local().Format("2006-01-02 15:04"), e.ExitCode)
	if len(e.Branches) > 0 {
		fmt.Fprintf(&sb, "branches %s\n", strings.Join(e.Branches, ", "))
	}
	if len(e.Prompts) > 0 {
		sb.WriteString("\nprompts:\n")
		for _, p := range e.Prompts {
			fmt.Fprintf(&sb, "- %s\n", strings.ReplaceAll(p, "\n", " "))
		}
	}
	text, cut := tail(d.Text, MaxHistoryText)
	sb.WriteString("\nterminal:\n")
	if cut {
		fmt.Fprintf(&sb, "[… the first %d bytes are left out]\n", len(d.Text)-len(text))
	}
	sb.WriteString(text)
	return sb.String(), map[string]any{"entry": e, "truncated": cut}, nil
}

// tail is the last n bytes of s, cut at a line start (else a rune start).
func tail(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	t := s[len(s)-n:]
	if i := strings.IndexByte(t, '\n'); i >= 0 && i < len(t)-1 {
		return t[i+1:], true
	}
	for len(t) > 0 && !utf8.RuneStart(t[0]) {
		t = t[1:]
	}
	return t, true
}
