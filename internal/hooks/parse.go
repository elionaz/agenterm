// Package hooks integrates agent CLIs with agenterm's attention tracking:
// it generates per-session hook configuration (never touching the user's
// own agent config or the project) and translates hook payloads into
// status events.
package hooks

import (
	"encoding/json"
	"fmt"
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
	case "UserPromptSubmit", "PostToolUse", "PostToolUseFailure":
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
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxMessage {
		return string(r[:maxMessage-1]) + "…"
	}
	return s
}
