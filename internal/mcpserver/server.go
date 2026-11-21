// Package mcpserver is panal's MCP server for agents (`panal mcp`): the
// Model Context Protocol over stdio (JSON-RPC 2.0, one message per line),
// with tools to coordinate with the other sessions of the project and to
// read panal's state. It only reads and messages: it never starts or
// stops sessions.
//
// The daemon knows which session calls from the process ancestry (the
// server is a child of the agent), as for `panal msg`.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/elionaz/panal/internal/version"
)

// ProtocolVersion is the MCP revision the server speaks (it answers a
// client asking for another one with this one, as the spec allows).
const ProtocolVersion = "2025-06-18"

// maxLine bounds one incoming message.
const maxLine = 4 << 20

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Server answers one MCP client.
type Server struct {
	b     Backend
	tools []tool

	outMu sync.Mutex
	out   *json.Encoder
	wg    sync.WaitGroup
}

// New is a server whose tools use b.
func New(b Backend) *Server {
	return &Server{b: b, tools: toolset()}
}

// Serve reads requests from r and writes answers to w until r ends or ctx
// is done. Tool calls run concurrently (wait can block for minutes);
// everything else is answered in order.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.out = json.NewEncoder(w)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	defer s.wg.Wait()
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.send(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "parse error: " + err.Error()}})
			continue
		}
		s.handle(ctx, req)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (s *Server) handle(ctx context.Context, req request) {
	notification := len(req.ID) == 0
	reply := func(result any, err *rpcError) {
		if notification {
			return
		}
		s.send(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: err})
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		reply(nil, &rpcError{codeInvalidRequest, "invalid request"})
		return
	}
	switch req.Method {
	case "initialize":
		reply(map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "panal", "title": "panal", "version": version.Version},
			"instructions": "Tools to coordinate with the other agent sessions of this panal project " +
				"(peers, msg_send, msg_read, wait) and to read panal's state (session_status, history_search, history_get).",
		}, nil)
	case "ping":
		reply(map[string]any{}, nil)
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, map[string]any{
				"name": t.name, "title": t.title, "description": t.desc, "inputSchema": t.schema,
				"annotations": map[string]any{"readOnlyHint": t.readOnly, "openWorldHint": false},
			})
		}
		reply(map[string]any{"tools": list}, nil)
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, &rpcError{codeInvalidParams, "invalid params: " + err.Error()})
			return
		}
		t, ok := s.find(p.Name)
		if !ok {
			reply(nil, &rpcError{codeInvalidParams, "unknown tool: " + p.Name})
			return
		}
		s.wg.Go(func() {
			reply(s.call(ctx, t, p.Arguments), nil)
		})
	default:
		if notification {
			return // notifications/initialized, cancelled…: nothing to answer
		}
		reply(nil, &rpcError{codeMethodNotFound, "method not found: " + req.Method})
	}
}

func (s *Server) find(name string) (tool, bool) {
	for _, t := range s.tools {
		if t.name == name {
			return t, true
		}
	}
	return tool{}, false
}

// call runs a tool; its failures are results with isError (the model sees
// them), not protocol errors.
func (s *Server) call(ctx context.Context, t tool, args json.RawMessage) map[string]any {
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage("{}")
	}
	text, data, err := t.run(ctx, s.b, args)
	if err != nil {
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}
	}
	res := map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
	if data != nil {
		res["structuredContent"] = data
	}
	return res
}

func (s *Server) send(r response) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	// A failed write means the client quit: nothing more to say.
	_ = s.out.Encode(r)
}
