package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/elionaz/panal/internal/i18n"
	"github.com/elionaz/panal/internal/status"
)

// Gemini CLI (https://geminicli.com/docs/hooks/reference/) reads hooks from
// its settings files. It has no --settings flag, but the system settings
// file can be moved with GEMINI_CLI_SYSTEM_SETTINGS_PATH, and hook lists
// from every scope are concatenated (settingsSchema.ts: hooks.* use
// MergeStrategy.CONCAT): panal points that variable at a generated file in
// the hooks dir, so its hooks are added to the user's own and nothing of
// theirs is edited. A real system settings file, if there is one, is copied
// into the generated file (it would otherwise be hidden); the system
// defaults file next to it keeps being read through
// GEMINI_CLI_SYSTEM_DEFAULTS_PATH.

// Environment variables Gemini CLI reads its system settings paths from.
const (
	geminiSystemEnv   = "GEMINI_CLI_SYSTEM_SETTINGS_PATH"
	geminiDefaultsEnv = "GEMINI_CLI_SYSTEM_DEFAULTS_PATH"
)

// geminiEvents are the Gemini CLI hook events panal listens to (see
// parseGemini). Gemini's own timeouts are in milliseconds.
var geminiEvents = []string{"SessionStart", "BeforeAgent", "BeforeTool", "AfterTool", "Notification", "AfterAgent"}

// geminiSystemPath is where Gemini CLI looks for its system settings when
// the environment does not say (cli/src/config/settings.ts).
func geminiSystemPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/GeminiCli/settings.json"
	case "windows":
		return `C:\ProgramData\gemini-cli\settings.json`
	}
	return "/etc/gemini-cli/settings.json"
}

// setupGemini writes dir/gemini-settings.json and returns the environment
// that makes Gemini CLI read it as its system settings.
func setupGemini(dir, exe, sock string) (Injection, error) {
	file := filepath.Join(dir, "gemini-settings.json")
	real := os.Getenv(geminiSystemEnv)
	if real == "" || real == file {
		real = geminiSystemPath()
	}
	base, err := readGeminiBase(real, file)
	if err != nil {
		// A system file panal cannot read back as JSON stays in charge:
		// replacing it would drop the administrator's settings.
		return Injection{Note: fmt.Sprintf("gemini hooks off: %v", err)}, nil
	}
	data, err := GeminiSettings(exe, sock, base)
	if err != nil {
		return Injection{}, err
	}
	if err := writeIfChanged(file, data); err != nil {
		return Injection{}, err
	}
	env := []string{geminiSystemEnv + "=" + file}
	if os.Getenv(geminiDefaultsEnv) == "" {
		// Gemini looks for system-defaults.json next to the system
		// settings: keep it pointing at the real one.
		env = append(env, geminiDefaultsEnv+"="+filepath.Join(filepath.Dir(real), "system-defaults.json"))
	}
	return Injection{Env: env}, nil
}

// readGeminiBase reads the real system settings file (nil if there is
// none). Gemini accepts comments in it; panal only plain JSON.
func readGeminiBase(path, own string) (map[string]any, error) {
	if path == own {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var base map[string]any
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("%s is not plain JSON: %w", path, err)
	}
	return base, nil
}

// GeminiSettings returns base (the real system settings, may be nil) with
// a hook per geminiEvents entry calling `<exe> notify [--sock S]
// --from-hook gemini` added to its own hooks.
func GeminiSettings(exe, sock string, base map[string]any) ([]byte, error) {
	out := make(map[string]any, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	hooks := map[string]any{}
	if h, ok := base["hooks"].(map[string]any); ok {
		for k, v := range h {
			hooks[k] = v
		}
	}
	cmd := hookCommand(exe, sock, AgentGemini)
	for _, ev := range geminiEvents {
		to := hookTimeout * 1000
		if ev == "BeforeTool" {
			to = preToolTimeout * 1000
		}
		var list []any
		if l, ok := hooks[ev].([]any); ok {
			list = append(list, l...)
		}
		list = append(list, map[string]any{"hooks": []any{map[string]any{
			"name": "panal", "type": "command", "command": cmd, "timeout": to,
		}}})
		hooks[ev] = list
	}
	out["hooks"] = hooks
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// hookCommand is the shell command line `<exe> notify [--sock S]
// --from-hook <agent>`.
func hookCommand(exe, sock, agent string) string {
	var parts []string
	for _, a := range notifyArgv(exe, sock) {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ") + " --from-hook " + agent
}

// geminiPayload holds the fields we use from Gemini CLI hook input. All
// events share session_id, transcript_path, cwd and hook_event_name.
type geminiPayload struct {
	HookEventName    string          `json:"hook_event_name"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	NotificationType string          `json:"notification_type"`
	Message          string          `json:"message"`
	Details          json.RawMessage `json:"details"`
	PromptResponse   string          `json:"prompt_response"`
}

// parseGemini maps Gemini CLI hook events to states. Gemini has no hook
// for a turn that fails or is cancelled (AfterAgent only follows a turn
// that completed), which is why its profile keeps the heuristic: a turn
// that ends without AfterAgent goes idle once the output is quiet.
func parseGemini(data []byte) (status.Event, bool, error) {
	var p geminiPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return status.Event{}, false, fmt.Errorf("gemini hook payload: %w", err)
	}
	switch p.HookEventName {
	case "BeforeAgent":
		return withActivity(status.Event{State: status.Running, Turn: true}, i18n.ActThinking, ""), true, nil
	case "BeforeTool":
		kind, target := ToolActivityParts(canonicalTool(AgentGemini, p.ToolName), p.ToolInput)
		return withActivity(status.Event{State: status.Running}, kind, target), true, nil
	case "AfterTool":
		return status.Event{State: status.Running}, true, nil
	case "Notification":
		// ToolPermission is the only notification type: a tool call
		// waits for the user's confirmation.
		if p.NotificationType == "ToolPermission" {
			msg := geminiPermission(p.Details)
			if msg == "" {
				msg = clip(p.Message)
			}
			return status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: msg}, true, nil
		}
		return status.Event{}, false, nil
	case "AfterAgent":
		return status.Event{State: status.Idle, Kind: status.KindDone, Message: clip(p.PromptResponse)}, true, nil
	}
	return status.Event{}, false, nil
}

// geminiPermission summarises a ToolPermission notification's details
// (the confirmation it waits for): "Shell: rm -rf build", "Edit: main.go",
// "MCP: github.create_issue". "" when they say nothing usable.
func geminiPermission(details json.RawMessage) string {
	var d struct {
		Title      string `json:"title"`
		Command    string `json:"command"`
		FileName   string `json:"fileName"`
		FilePath   string `json:"filePath"`
		ServerName string `json:"serverName"`
		ToolName   string `json:"toolName"`
	}
	if json.Unmarshal(details, &d) != nil {
		return ""
	}
	switch {
	case d.Command != "":
		return clip("Shell: " + d.Command)
	case d.FileName != "":
		return clip("Edit: " + d.FileName)
	case d.FilePath != "":
		return clip("Edit: " + base(d.FilePath))
	case d.ServerName != "" && d.ToolName != "":
		return clip("MCP: " + d.ServerName + "." + d.ToolName)
	}
	return clip(d.Title)
}

// GeminiHook is what a Gemini CLI payload says about where it fired.
type GeminiHook struct {
	Event string `json:"hook_event_name"`
	// StopActive (AfterAgent): this turn already follows a hook that
	// blocked the previous one.
	StopActive bool `json:"stop_hook_active"`
}

// GeminiEvent decodes the fields of GeminiHook from a payload.
func GeminiEvent(payload []byte) GeminiHook {
	var h GeminiHook
	_ = json.Unmarshal(payload, &h)
	return h
}
