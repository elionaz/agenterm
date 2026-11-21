package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/protocol"
)

// Agent sessions get panal's MCP server for that invocation only: Claude
// in its --mcp-config, codex as -c entries, Gemini in the system settings
// panal hands it, Cursor in the plugin's mcp.json; mcp.enabled: false
// takes it away.
func TestAgentMCPServer(t *testing.T) {
	t.Setenv("GEMINI_CLI_SYSTEM_SETTINGS_PATH", filepath.Join(t.TempDir(), "settings.json"))
	c, p, stop := startDaemon(t)
	defer stop()
	writeProfiles(t, p.Profiles(), "- id: hookedcodex\n  cmd: /bin/echo\n  setup_hooks: codex\n"+
		"- id: hookedgemini\n  cmd: /bin/echo\n  setup_hooks: gemini\n"+
		"- id: hookedcursor\n  cmd: /bin/echo\n  setup_hooks: cursor\n")
	ctx := context.Background()
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "app")
	if err != nil {
		t.Fatal(err)
	}
	spawn := func(profile string) protocol.SessionInfo {
		t.Helper()
		s, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: profile})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	read := func(file string) string {
		t.Helper()
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	geminiFile := filepath.Join(p.HooksDir(), "gemini-settings.json")
	cursorFile := filepath.Join(p.HooksDir(), "cursor-plugin", "mcp.json")

	cl := spawn("hookedclaude")
	if len(cl.Args) < 2 || cl.Args[0] != "--mcp-config" {
		t.Fatalf("claude args = %q", cl.Args)
	}
	if cfg := read(cl.Args[1]); !strings.Contains(cfg, `"panal":{"args":["mcp","--sock"`) {
		t.Fatalf("claude mcp config %s", cfg)
	}
	cx := spawn("hookedcodex")
	if !slices.Contains(cx.Args, "mcp_servers.panal.args=[\"mcp\",\"--sock\",\""+p.Socket()+"\"]") {
		t.Fatalf("codex args = %q", cx.Args)
	}
	spawn("hookedgemini")
	if cfg := read(geminiFile); !strings.Contains(cfg, `"mcpServers"`) || !strings.Contains(cfg, `"mcp",`) {
		t.Fatalf("gemini settings %s", cfg)
	}
	spawn("hookedcursor")
	if cfg := read(cursorFile); !strings.Contains(cfg, `"panal"`) {
		t.Fatalf("cursor mcp.json %s", cfg)
	}

	// Off: none of them gets it.
	if err := os.WriteFile(p.Settings(), []byte("mcp:\n  enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cl := spawn("hookedclaude"); slices.Contains(cl.Args, "--mcp-config") && strings.Contains(read(cl.Args[1]), `"panal":`) {
		t.Fatalf("claude args with MCP off = %q", cl.Args)
	}
	if cx := spawn("hookedcodex"); slices.ContainsFunc(cx.Args, func(a string) bool { return strings.HasPrefix(a, "mcp_servers.panal.") }) {
		t.Fatalf("codex args with MCP off = %q", cx.Args)
	}
	spawn("hookedgemini")
	if cfg := read(geminiFile); strings.Contains(cfg, `"mcpServers"`) {
		t.Fatalf("gemini settings with MCP off %s", cfg)
	}
	spawn("hookedcursor")
	if _, err := os.Stat(cursorFile); !os.IsNotExist(err) {
		t.Fatalf("cursor mcp.json with MCP off: %v", err)
	}
}

// WaitIdle (panal wait, the MCP wait tool) returns once the session is
// idle, and says when there is no such session.
func TestWaitIdle(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "app")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if st, err := c.WaitIdle(wctx, sh.ID); err != nil || !client.WaitDone(st) {
		t.Fatalf("wait = %q, %v", st, err)
	}
	if _, err := c.WaitIdle(wctx, "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("wait for an unknown session: %v", err)
	}
}
