package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Injection is what a session needs to report to agenterm: extra arguments
// placed before the profile's own and extra environment variables.
type Injection struct {
	Args []string
	Env  []string
}

// Agents with built-in hook integration (profile `setup_hooks`).
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// claudeEvents are the Claude Code hook events agenterm listens to; see
// parseClaude for what each one means.
var claudeEvents = []string{"UserPromptSubmit", "PermissionRequest", "Notification", "PostToolUse", "Stop", "StopFailure"}

// Setup prepares the hook integration for agent. exe is the absolute path
// of a binary implementing `notify --from-hook` (the daemon's own
// executable); dir is where generated files go (paths.HooksDir). Nothing is
// written outside dir: the user's agent configuration and the project are
// left alone, and the integration only applies to sessions agenterm spawns.
// An empty agent (or "none") means no integration.
func Setup(agent, dir, exe string) (Injection, error) {
	switch agent {
	case "", "none":
		return Injection{}, nil
	case AgentClaude:
		// `claude --settings <file>` loads additional settings for this
		// invocation only; hook lists merge with the user's own hooks.
		file := filepath.Join(dir, "claude-settings.json")
		if err := writeIfChanged(file, ClaudeSettings(exe)); err != nil {
			return Injection{}, err
		}
		return Injection{Args: []string{"--settings", file}}, nil
	case AgentCodex:
		// `codex -c key=value` overrides config.toml for this invocation.
		// notify runs our binary with the turn-complete JSON as last argv;
		// the TUI's own OSC 9 notifications cover approvals and questions
		// (picked up by the terminal-signal parser).
		notify, _ := json.Marshal([]string{exe, "notify", "--from-hook", AgentCodex})
		return Injection{Args: []string{
			"-c", "notify=" + string(notify),
			"-c", `tui.notifications=["approval-requested","plan-mode-prompt","async-question"]`,
			"-c", `tui.notification_method="osc9"`,
			"-c", `tui.notification_condition="always"`,
		}}, nil
	}
	return Injection{}, fmt.Errorf("unknown setup_hooks %q (want claude, codex or none)", agent)
}

// ClaudeSettings returns the --settings JSON wiring every relevant Claude
// Code hook event to `<exe> notify --from-hook claude`, which reads the
// event from stdin, prints nothing and always exits 0.
func ClaudeSettings(exe string) []byte {
	cmd := shellQuote(exe) + " notify --from-hook claude"
	type handler struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	type group struct {
		Hooks []handler `json:"hooks"`
	}
	hooks := make(map[string][]group, len(claudeEvents))
	for _, ev := range claudeEvents {
		hooks[ev] = []group{{Hooks: []handler{{Type: "command", Command: cmd, Timeout: 5}}}}
	}
	b, _ := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return append(b, '\n')
}

// shellQuote quotes s for POSIX sh (Claude runs hook commands via a shell).
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
