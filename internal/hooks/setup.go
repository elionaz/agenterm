package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Injection is what a session needs to report to panal: extra arguments
// placed before the profile's own and extra environment variables.
type Injection struct {
	Args []string
	Env  []string
	// Note says why an agent's integration is off for this launch (the
	// session then runs without it); "" normally.
	Note string
}

// Agents with built-in hook integration (profile `setup_hooks`).
const (
	AgentClaude   = "claude"
	AgentCodex    = "codex"
	AgentGemini   = "gemini"
	AgentOpencode = "opencode"
	AgentCursor   = "cursor"
)

// Agents lists the setup_hooks values with an integration.
var Agents = []string{AgentClaude, AgentCodex, AgentGemini, AgentOpencode, AgentCursor}

// Known reports whether agent is a valid setup_hooks value ("" and "none"
// included: no integration).
func Known(agent string) bool {
	return agent == "" || agent == "none" || slices.Contains(Agents, agent)
}

// claudeEvents are the Claude Code hook events panal listens to; see
// parseClaude for what each one means. SessionStart carries no state: it
// reports the (new) session_id early, so a daemon restart can resume the
// conversation even before the first prompt (SessionRef).
var claudeEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Notification", "PostToolUse", "Stop", "StopFailure"}

// Hook timeouts (seconds). PreToolUse runs before every tool call, so it
// gets the tightest bound; `notify --from-hook` gives up after 2 s anyway and
// never prints a permission decision (exit 0, empty stdout = "no opinion":
// the call goes through Claude's normal permission flow).
const (
	hookTimeout    = 5
	preToolTimeout = 3
)

// Setup prepares the hook integration for agent. exe is the absolute path
// of a binary implementing `notify --from-hook` (the daemon's own
// executable); sock is the daemon's socket, passed explicitly (`--sock`) so
// hooks reach that daemon whatever PANAL_HOME / PANAL_SOCK the
// agent's environment ends up with; dir is where generated files go
// (paths.HooksDir, which belongs to that same daemon). Nothing is
// written outside dir: the user's agent configuration and the project are
// left alone, and the integration only applies to sessions panal spawns.
// An empty agent (or "none") means no integration.
func Setup(agent, dir, exe, sock string) (Injection, error) {
	switch agent {
	case "", "none":
		return Injection{}, nil
	case AgentClaude:
		// `claude --settings <file>` loads additional settings for this
		// invocation only; hook lists merge with the user's own hooks.
		file := filepath.Join(dir, "claude-settings.json")
		if err := writeIfChanged(file, ClaudeSettings(exe, sock)); err != nil {
			return Injection{}, err
		}
		return Injection{Args: []string{"--settings", file}}, nil
	case AgentCodex:
		// `codex -c key=value` overrides config.toml for this invocation.
		// notify runs our binary with the turn-complete JSON as last argv;
		// the TUI's own OSC 9 notifications cover approvals and questions
		// (picked up by the terminal-signal parser).
		notify, _ := json.Marshal(append(notifyArgv(exe, sock), "--from-hook", AgentCodex))
		return Injection{Args: []string{
			"-c", "notify=" + string(notify),
			"-c", `tui.notifications=["approval-requested","plan-mode-prompt","async-question"]`,
			"-c", `tui.notification_method="osc9"`,
			"-c", `tui.notification_condition="always"`,
		}}, nil
	case AgentGemini:
		return setupGemini(dir, exe, sock)
	case AgentOpencode:
		return setupOpencode(dir, exe, sock)
	case AgentCursor:
		return setupCursor(dir, exe, sock)
	}
	return Injection{}, fmt.Errorf("unknown setup_hooks %q (want %s or none)", agent, strings.Join(Agents, ", "))
}

// notifyArgv is `<exe> notify [--sock <sock>]`.
func notifyArgv(exe, sock string) []string {
	argv := []string{exe, "notify"}
	if sock != "" {
		argv = append(argv, "--sock", sock)
	}
	return argv
}

// ClaudeSettings returns the --settings JSON wiring every relevant Claude
// Code hook event to `<exe> notify [--sock S] --from-hook claude`, which
// reads the event from stdin, prints nothing and always exits 0.
func ClaudeSettings(exe, sock string) []byte {
	cmd := hookCommand(exe, sock, AgentClaude)
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
		to := hookTimeout
		if ev == "PreToolUse" {
			to = preToolTimeout
		}
		hooks[ev] = []group{{Hooks: []handler{{Type: "command", Command: cmd, Timeout: to}}}}
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

// WriteIfChanged writes data to path (atomically, private) unless it
// already holds it.
func WriteIfChanged(path string, data []byte) error { return writeIfChanged(path, data) }

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
