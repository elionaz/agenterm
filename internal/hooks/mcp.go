package hooks

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// MCPServer is a stdio MCP server an agent session gets (panal's own,
// `panal mcp`, or the browser one).
type MCPServer struct {
	Command string
	Args    []string
}

// Option adjusts Setup.
type Option func(*options)

type options struct {
	mcp map[string]MCPServer
}

// WithMCP gives the session these MCP servers (by name), for the agents
// whose integration carries them: Gemini CLI (its system settings),
// opencode (its inline config) and Cursor CLI (the plugin's mcp.json).
// Claude Code and codex get theirs as arguments from the daemon.
func WithMCP(servers map[string]MCPServer) Option {
	return func(o *options) { o.mcp = servers }
}

func setupOptions(opts []Option) options {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return o
}

// mcpNames are the server names in a stable order.
func mcpNames(m map[string]MCPServer) []string {
	return slices.Sorted(maps.Keys(m))
}

// stdioServers is the {"name": {"command": …, "args": […]}} map that
// Gemini CLI and Cursor CLI read (mcpServers).
func stdioServers(m map[string]MCPServer) map[string]any {
	out := make(map[string]any, len(m))
	for _, n := range mcpNames(m) {
		out[n] = map[string]any{"command": m[n].Command, "args": append([]string{}, m[n].Args...)}
	}
	return out
}

// opencodeMCP is opencode's "mcp" config section.
func opencodeMCP(m map[string]MCPServer) map[string]any {
	out := make(map[string]any, len(m))
	for _, n := range mcpNames(m) {
		out[n] = map[string]any{"type": "local", "command": append([]string{m[n].Command}, m[n].Args...), "enabled": true}
	}
	return out
}

// writeCursorMCP writes (or, with no servers, removes) the plugin's
// mcp.json, which Cursor discovers in a plugin's root.
func writeCursorMCP(pdir string, m map[string]MCPServer) error {
	file := filepath.Join(pdir, "mcp.json")
	if len(m) == 0 {
		if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(map[string]any{"mcpServers": stdioServers(m)}, "", "  ")
	if err != nil {
		return err
	}
	return writeIfChanged(file, append(b, '\n'))
}
