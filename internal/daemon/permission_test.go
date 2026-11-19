package daemon

import (
	"strings"
	"testing"
)

func TestPermissionMode(t *testing.T) {
	for _, c := range []struct {
		cmd, args, want string
	}{
		{"claude", "", ""},
		{"/opt/homebrew/bin/claude", "--settings /x/hooks.json --dangerously-skip-permissions", PermBypass},
		{"claude", "--permission-mode acceptEdits", PermAutoEdit},
		{"claude", "--permission-mode=plan", PermPlan},
		{"claude", "--permission-mode bypassPermissions", PermBypass},
		{"claude", "--permission-mode plan --dangerously-skip-permissions", PermBypass},
		{"claude", "--allow-dangerously-skip-permissions", ""},
		{"claude", "-p --permission-mode", ""},
		{"codex", "--full-auto", PermAutoEdit},
		{"codex", "-a never", PermBypass},
		{"codex", "--ask-for-approval=never", PermBypass},
		{"codex", "-a on-request", ""},
		{"codex", "-s danger-full-access", PermBypass},
		{"codex", "--sandbox read-only", PermPlan},
		{"codex", "--dangerously-bypass-approvals-and-sandbox", PermBypass},
		{"gemini", "--yolo", PermBypass},
		{"gemini", "--approval-mode auto_edit", PermAutoEdit},
		{"qwen", "-y", PermBypass},
		{"cursor-agent", "--force", PermBypass},
		{"cursor-agent", "-f", PermBypass},
		{"copilot", "--allow-all-tools", PermBypass},
		{"amp", "--dangerously-allow-all", PermBypass},
		{"crush", "--yolo", PermBypass},
		{"kiro-cli", "chat --trust-all-tools", PermBypass},
		{"aider", "--yes-always", PermBypass},
		// Flags of one agent mean nothing for another.
		{"opencode", "--yolo -f --dangerously-skip-permissions", ""},
		{"/bin/zsh", "-f", ""},
	} {
		var args []string
		if c.args != "" {
			args = strings.Fields(c.args)
		}
		if got := permissionMode(c.cmd, args); got != c.want {
			t.Errorf("%s %s = %q, want %q", c.cmd, c.args, got, c.want)
		}
	}
}
