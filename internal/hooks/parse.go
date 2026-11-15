// Package hooks integrates agent CLIs with agenterm's attention tracking:
// it generates per-session hook configuration (never touching the user's
// own agent config or the project) and translates hook payloads into
// status events.
package hooks

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/elionaz/agenterm/internal/status"
)

// maxMessage bounds messages taken from payloads (they end up in
// notifications and tooltips).
const maxMessage = 240

// Parse translates the payload of agent's hook into an event. ok is false
// when the payload carries nothing worth reporting (the caller then does
// nothing and exits 0).
func Parse(agent string, payload []byte) (ev status.Event, ok bool, err error) {
	switch agent {
	case "claude":
		return parseClaude(payload)
	case "codex":
		return parseCodex(payload)
	}
	return status.Event{}, false, fmt.Errorf("unknown hook agent %q", agent)
}

// claudePayload holds the fields we use from Claude Code hook input
// (https://code.claude.com/docs/en/hooks). All events share session_id,
// cwd and hook_event_name.
type claudePayload struct {
	HookEventName        string          `json:"hook_event_name"`
	Message              string          `json:"message"`
	Title                string          `json:"title"`
	NotificationType     string          `json:"notification_type"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	Error                string          `json:"error"`
	ErrorDetails         string          `json:"error_details"`
}

func parseClaude(data []byte) (status.Event, bool, error) {
	var p claudePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return status.Event{}, false, fmt.Errorf("claude hook payload: %w", err)
	}
	switch p.HookEventName {
	case "UserPromptSubmit":
		return status.Event{State: status.Running, Turn: true, Activity: "Pensando…"}, true, nil
	case "PreToolUse":
		return status.Event{State: status.Running, Activity: ToolActivity(p.ToolName, p.ToolInput)}, true, nil
	case "PostToolUse", "PostToolUseFailure":
		return status.Event{State: status.Running}, true, nil
	case "PermissionRequest":
		return status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: permissionMessage(p.ToolName, p.ToolInput)}, true, nil
	case "Notification":
		switch p.NotificationType {
		case "permission_prompt":
			return status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: clip(p.Message)}, true, nil
		case "elicitation_dialog", "elicitation_url_dialog", "agent_needs_input":
			return status.Event{State: status.WaitingInput, Kind: status.KindQuestion, Message: clip(p.Message)}, true, nil
		}
		// idle_prompt is a 60 s reminder of a turn we already reported as
		// done; auth_success and the rest are not requests for attention.
		return status.Event{}, false, nil
	case "Stop":
		return status.Event{State: status.Idle, Kind: status.KindDone, Message: clip(p.LastAssistantMessage)}, true, nil
	case "StopFailure":
		msg := p.ErrorDetails
		if msg == "" {
			msg = p.Error
		}
		return status.Event{State: status.Idle, Kind: status.KindError, Message: clip(msg)}, true, nil
	}
	return status.Event{}, false, nil
}

// maxActivity bounds activity strings (sidebar rows, cards); Bash commands
// get a tighter limit so the verb stays readable.
const (
	maxActivity = 120
	maxCommand  = 60
)

// ToolActivity describes a Claude Code tool call in Spanish for the
// activity line: "Editando main.go", "Bash: go test ./...". Field names follow
// the tool_input shapes of the hooks reference
// (https://code.claude.com/docs/en/hooks#pretooluse-input).
func ToolActivity(tool string, input json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := in[k].(string); ok && strings.TrimSpace(v) != "" {
				return oneLine(v)
			}
		}
		return ""
	}
	with := func(verb, arg string) string {
		if arg == "" {
			return verb
		}
		return clipTo(verb+" "+arg, maxActivity)
	}
	switch tool {
	case "Edit", "Write", "MultiEdit":
		return with("Editando", base(str("file_path", "path")))
	case "NotebookEdit":
		return with("Editando", base(str("notebook_path", "file_path")))
	case "Read":
		return with("Leyendo", base(str("file_path", "path")))
	case "Bash":
		if c := str("command"); c != "" {
			return "Bash: " + clipTo(c, maxCommand)
		}
		return "Bash"
	case "Grep", "Glob":
		return with("Buscando", str("pattern"))
	case "WebFetch":
		u := str("url")
		if pu, err := url.Parse(u); err == nil && pu.Host != "" {
			u = pu.Host
		}
		return with("Web:", u)
	case "WebSearch":
		return with("Web:", str("query"))
	case "Task", "Agent":
		return with("Subagente:", str("description", "subagent_type"))
	case "TodoWrite":
		return "Actualizando tareas"
	}
	if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
		server, name, _ := strings.Cut(rest, "__")
		if name != "" {
			return clipTo("MCP: "+server+"."+name, maxActivity)
		}
		return clipTo("MCP: "+server, maxActivity)
	}
	return clipTo(tool, maxActivity)
}

func base(p string) string {
	if p == "" {
		return ""
	}
	return path.Base(strings.TrimRight(p, "/"))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// permissionMessage summarises a tool call: "Bash: rm -rf node_modules".
func permissionMessage(tool string, input json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	for _, k := range []string{"command", "file_path", "path", "url", "pattern", "description"} {
		if v, ok := in[k].(string); ok && v != "" {
			if tool == "" {
				return clip(v)
			}
			return clip(tool + ": " + v)
		}
	}
	if tool != "" {
		return clip("Permiso para " + tool)
	}
	return ""
}

// codexPayload is the JSON codex passes as the last argument of its
// `notify` program.
type codexPayload struct {
	Type                 string   `json:"type"`
	LastAssistantMessage string   `json:"last-assistant-message"`
	InputMessages        []string `json:"input-messages"`
}

func parseCodex(data []byte) (status.Event, bool, error) {
	var p codexPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return status.Event{}, false, fmt.Errorf("codex notify payload: %w", err)
	}
	switch p.Type {
	case "agent-turn-complete":
		return status.Event{State: status.Idle, Kind: status.KindDone, Message: clip(p.LastAssistantMessage)}, true, nil
	}
	return status.Event{}, false, nil
}

// clip flattens whitespace and truncates to maxMessage runes.
func clip(s string) string { return clipTo(s, maxMessage) }

// clipTo flattens whitespace and truncates to n runes (with an ellipsis).
func clipTo(s string, n int) string {
	s = oneLine(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
