package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var panalMCP = map[string]MCPServer{"panal": {Command: "/usr/local/bin/panal", Args: []string{"mcp", "--sock", "/s.sock"}}}

type stdioCfg struct {
	MCPServers map[string]struct {
		Command string
		Args    []string
	} `json:"mcpServers"`
}

func TestGeminiMCP(t *testing.T) {
	dir, sys := t.TempDir(), t.TempDir()
	real := filepath.Join(sys, "settings.json")
	t.Setenv(geminiSystemEnv, real)
	t.Setenv(geminiDefaultsEnv, "")
	// The system file's own server stays next to panal's.
	os.WriteFile(real, []byte(`{"mcpServers":{"corp":{"command":"corp-mcp"}}}`), 0o600)
	inj, err := Setup(AgentGemini, dir, "/usr/local/bin/panal", "/s.sock", WithMCP(panalMCP))
	if err != nil || inj.MCP == nil {
		t.Fatalf("inj %+v err %v", inj, err)
	}
	var cfg stdioCfg
	data, _ := os.ReadFile(filepath.Join(dir, "gemini-settings.json"))
	if json.Unmarshal(data, &cfg) != nil || cfg.MCPServers["corp"].Command != "corp-mcp" ||
		cfg.MCPServers["panal"].Command != "/usr/local/bin/panal" || strings.Join(cfg.MCPServers["panal"].Args, " ") != "mcp --sock /s.sock" {
		t.Fatalf("settings %s", data)
	}
	// Without the option: no server of ours.
	if _, err := Setup(AgentGemini, dir, "/usr/local/bin/panal", "/s.sock"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "gemini-settings.json"))
	var off stdioCfg
	if json.Unmarshal(data, &off) != nil || len(off.MCPServers) != 1 {
		t.Fatalf("settings without MCP %s", data)
	}
}

func TestOpencodeMCP(t *testing.T) {
	t.Setenv(opencodeConfigEnv, "")
	dir := t.TempDir()
	inj, err := Setup(AgentOpencode, dir, "/usr/local/bin/panal", "/s.sock", WithMCP(panalMCP))
	if err != nil || len(inj.Env) != 1 {
		t.Fatalf("inj %+v err %v", inj, err)
	}
	type mcpCfg struct {
		MCP map[string]struct {
			Type    string
			Command []string
			Enabled bool
		}
		Instructions []string
	}
	check := func(kv string) mcpCfg {
		t.Helper()
		_, v, _ := strings.Cut(kv, "=")
		var cfg mcpCfg
		if json.Unmarshal([]byte(v), &cfg) != nil || cfg.MCP["panal"].Type != "local" || !cfg.MCP["panal"].Enabled ||
			strings.Join(cfg.MCP["panal"].Command, " ") != "/usr/local/bin/panal mcp --sock /s.sock" {
			t.Fatalf("config %s", v)
		}
		return cfg
	}
	check(inj.Env[0])
	// The instructions document keeps the servers.
	if cfg := check(OpencodeInstructionsEnv(inj, dir, "/h/i.md")); len(cfg.Instructions) != 1 {
		t.Fatalf("instructions %v", cfg.Instructions)
	}
}

func TestCursorMCP(t *testing.T) {
	dir := t.TempDir()
	if _, err := Setup(AgentCursor, dir, "/usr/local/bin/panal", "/s.sock", WithMCP(panalMCP)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, cursorPluginDir, "mcp.json")
	var cfg stdioCfg
	data, _ := os.ReadFile(file)
	if json.Unmarshal(data, &cfg) != nil || cfg.MCPServers["panal"].Command != "/usr/local/bin/panal" {
		t.Fatalf("mcp.json %s", data)
	}
	// Turned off: the file goes.
	if _, err := Setup(AgentCursor, dir, "/usr/local/bin/panal", "/s.sock"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("mcp.json still there: %v", err)
	}
}
