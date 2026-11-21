package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/history"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/protocol"
)

type fake struct {
	peers protocol.MsgPeers
	sent  []protocol.MsgSendParams
	take  protocol.MsgTakeResult
	wait  func(ctx context.Context, id string) (string, error)
	hits  []history.Hit
	query protocol.HistorySearchParams
	entry protocol.HistoryDetail
	diff  protocol.DiffResult
}

func (f *fake) MsgPeers(context.Context, string) (protocol.MsgPeers, error) { return f.peers, nil }
func (f *fake) MsgSend(_ context.Context, p protocol.MsgSendParams) ([]mailbox.Message, error) {
	f.sent = append(f.sent, p)
	return []mailbox.Message{{ID: 7, To: "b2", ToName: "beta", Held: true}}, nil
}
func (f *fake) MsgTake(context.Context, bool) (protocol.MsgTakeResult, error) { return f.take, nil }
func (f *fake) WaitIdle(ctx context.Context, id string) (string, error)       { return f.wait(ctx, id) }
func (f *fake) HistorySearch(_ context.Context, p protocol.HistorySearchParams) ([]history.Hit, error) {
	f.query = p
	return f.hits, nil
}
func (f *fake) HistoryGet(_ context.Context, id string) (protocol.HistoryDetail, error) {
	if id != f.entry.Entry.ID {
		return protocol.HistoryDetail{}, errors.New("no session " + id + " in the history")
	}
	return f.entry, nil
}
func (f *fake) SessionDiff(context.Context, string, bool) (protocol.DiffResult, error) {
	return f.diff, nil
}

// session drives a server over pipes, one request at a time.
type session struct {
	t   *testing.T
	in  io.WriteCloser
	out *bufio.Scanner
	id  int
}

func start(t *testing.T, b Backend) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- New(b).Serve(context.Background(), inR, outW)
		outW.Close()
	}()
	t.Cleanup(func() {
		inW.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	return &session{t: t, in: inW, out: sc}
}

func (s *session) write(line string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, line+"\n"); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) read() map[string]any {
	s.t.Helper()
	if !s.out.Scan() {
		s.t.Fatalf("no answer: %v", s.out.Err())
	}
	var m map[string]any
	if err := json.Unmarshal(s.out.Bytes(), &m); err != nil {
		s.t.Fatalf("answer %s: %v", s.out.Bytes(), err)
	}
	return m
}

func (s *session) call(method string, params any) map[string]any {
	s.t.Helper()
	s.id++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.id, "method": method, "params": params})
	s.write(string(b))
	m := s.read()
	if id, _ := m["id"].(float64); int(id) != s.id {
		s.t.Fatalf("answer for id %v, want %d: %v", m["id"], s.id, m)
	}
	return m
}

func (s *session) tool(name string, args any) (text string, isError bool, structured map[string]any) {
	s.t.Helper()
	m := s.call("tools/call", map[string]any{"name": name, "arguments": args})
	res, ok := m["result"].(map[string]any)
	if !ok {
		s.t.Fatalf("%s: no result: %v", name, m)
	}
	content := res["content"].([]any)
	text = content[0].(map[string]any)["text"].(string)
	isError, _ = res["isError"].(bool)
	structured, _ = res["structuredContent"].(map[string]any)
	return text, isError, structured
}

func peers() protocol.MsgPeers {
	return protocol.MsgPeers{
		ProjectID: "p1", ProjectName: "app", Enabled: true,
		Self:  &protocol.MsgPeer{ID: "a1", Name: "alpha", ProfileID: "claude", State: "working"},
		Peers: []protocol.MsgPeer{{ID: "b2", Name: "beta", ProfileID: "codex", State: "idle", Mode: protocol.ModeWorktree, Branch: "feat/b"}},
	}
}

func TestProtocol(t *testing.T) {
	s := start(t, &fake{peers: peers()})

	init := s.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t"}})
	res := init["result"].(map[string]any)
	if res["protocolVersion"] != ProtocolVersion || res["serverInfo"].(map[string]any)["name"] != "panal" {
		t.Fatalf("initialize: %v", init)
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("no tools capability: %v", res)
	}
	// A notification gets no answer: the next answer is the ping's.
	s.write(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if m := s.call("ping", nil); m["error"] != nil {
		t.Fatalf("ping: %v", m)
	}

	list := s.call("tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	want := []string{"peers", "msg_send", "msg_read", "wait", "session_status", "history_search", "history_get"}
	if len(list) != len(want) {
		t.Fatalf("%d tools, want %d", len(list), len(want))
	}
	for i, x := range list {
		tl := x.(map[string]any)
		if tl["name"] != want[i] || tl["description"] == "" {
			t.Fatalf("tool %d: %v", i, tl)
		}
		if sc := tl["inputSchema"].(map[string]any); sc["type"] != "object" {
			t.Fatalf("%s schema: %v", tl["name"], sc)
		}
	}

	if m := s.call("resources/list", nil); m["error"].(map[string]any)["code"].(float64) != codeMethodNotFound {
		t.Fatalf("unknown method: %v", m)
	}
	if m := s.call("tools/call", map[string]any{"name": "spawn"}); m["error"] == nil {
		t.Fatalf("unknown tool: %v", m)
	}
	s.write(`{not json`)
	if m := s.read(); m["error"].(map[string]any)["code"].(float64) != codeParse {
		t.Fatalf("parse error: %v", m)
	}
}

func TestMessagingTools(t *testing.T) {
	f := &fake{peers: peers(), take: protocol.MsgTakeResult{Enabled: true, Held: 2,
		Messages: []mailbox.Message{{ID: 3, From: "b2", FromName: "beta", To: "a1", Body: "done with the API"}}}}
	s := start(t, f)

	text, isErr, _ := s.tool("peers", nil)
	if isErr || !strings.Contains(text, `a1 "alpha"`) || !strings.Contains(text, `b2 "beta" [codex] idle, worktree on branch feat/b`) {
		t.Fatalf("peers: %q", text)
	}

	text, isErr, data := s.tool("msg_send", map[string]any{"to": "beta", "body": "ready?"})
	if isErr || !strings.Contains(text, "sent #7 to beta (b2): held") || len(f.sent) != 1 || f.sent[0].To != "beta" {
		t.Fatalf("msg_send: %q, %v", text, f.sent)
	}
	if sent := data["sent"].([]any); len(sent) != 1 || sent[0].(map[string]any)["held"] != true {
		t.Fatalf("msg_send data: %v", data)
	}
	if _, isErr, _ := s.tool("msg_send", map[string]any{"to": "beta", "body": " "}); !isErr {
		t.Fatal("an empty body is an error")
	}

	text, isErr, _ = s.tool("msg_read", nil)
	if isErr || !strings.Contains(text, "done with the API") || !strings.Contains(text, "2 more waiting for your user") {
		t.Fatalf("msg_read: %q", text)
	}
}

func TestWaitTool(t *testing.T) {
	f := &fake{peers: peers()}
	f.wait = func(ctx context.Context, id string) (string, error) {
		if id != "b2" {
			return "", errors.New("wrong id " + id)
		}
		return "idle", nil
	}
	s := start(t, f)
	text, isErr, data := s.tool("wait", map[string]any{"session": "beta"})
	if isErr || text != "beta (b2) is idle" || data["state"] != "idle" {
		t.Fatalf("wait: %q %v", text, data)
	}
	if text, isErr, _ := s.tool("wait", map[string]any{"session": "gamma"}); !isErr || !strings.Contains(text, `no session "gamma"`) {
		t.Fatalf("unknown session: %q", text)
	}

	// A timeout is an answer (still busy), not an error.
	f.wait = func(ctx context.Context, id string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	start := time.Now()
	text, isErr, data = s.tool("wait", map[string]any{"session": "b2", "timeoutSec": 1})
	if isErr || !strings.Contains(text, "still busy after 1s") || data["timedOut"] != true {
		t.Fatalf("timeout: %q %v", text, data)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("the timeout was not applied")
	}
}

func TestStatusAndHistoryTools(t *testing.T) {
	f := &fake{peers: peers(),
		diff: protocol.DiffResult{Branch: "feat/a", Base: "main", Additions: 3, Deletions: 1,
			Git: protocol.GitInfo{Ahead: 1, Uncommitted: 1, Merged: false}},
		hits: []history.Hit{{Entry: history.Entry{ID: "h1", Name: "fix login", Agent: "Claude Code", ProjectName: "app", EndedAt: time.Now()},
			Snippets: []history.Snippet{{Source: "prompt", Line: "fix the login form"}}}},
		entry: protocol.HistoryDetail{Entry: history.Entry{ID: "h1", Name: "fix login", Prompts: []string{"fix the login form"}},
			Text: strings.Repeat("line\n", MaxHistoryText/4)},
	}
	f.diff.Files = []gitx.DiffFile{{Path: "a.go", Status: "M", Additions: 3, Deletions: 1}}
	s := start(t, f)

	text, isErr, _ := s.tool("session_status", nil)
	if isErr || !strings.Contains(text, `a1 "alpha"`) || !strings.Contains(text, "branch feat/a (base main)") ||
		!strings.Contains(text, "1 files changed (+3 −1), 1 uncommitted, 1 commits ahead") || !strings.Contains(text, "M a.go") {
		t.Fatalf("session_status: %q", text)
	}

	text, isErr, _ = s.tool("history_search", map[string]any{"query": "login"})
	if isErr || !strings.Contains(text, `h1 "fix login"`) || !strings.Contains(text, "prompt: fix the login form") {
		t.Fatalf("history_search: %q", text)
	}
	if f.query.ProjectID != "p1" || f.query.Query != "login" || f.query.Limit != 10 {
		t.Fatalf("search query: %+v", f.query)
	}
	if s.tool("history_search", map[string]any{"allProjects": true, "limit": 500}); f.query.ProjectID != "" || f.query.Limit != maxHistoryHit {
		t.Fatalf("all projects: %+v", f.query)
	}

	text, isErr, data := s.tool("history_get", map[string]any{"id": "h1"})
	if isErr || !strings.Contains(text, "- fix the login form") || !strings.Contains(text, "bytes are left out") || data["truncated"] != true {
		t.Fatalf("history_get: %.300q", text)
	}
	if len(text) > MaxHistoryText+2000 {
		t.Fatalf("history_get text is %d bytes", len(text))
	}
	if _, isErr, _ := s.tool("history_get", map[string]any{"id": "zz"}); !isErr {
		t.Fatal("a missing entry is an error")
	}
}
