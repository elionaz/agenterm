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

	"github.com/elionaz/agenterm/internal/i18n"
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
		return withActivity(status.Event{State: status.Running, Turn: true}, i18n.ActThinking, ""), true, nil
	case "PreToolUse":
		kind, target := ToolActivityParts(p.ToolName, p.ToolInput)
		return withActivity(status.Event{State: status.Running}, kind, target), true, nil
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
	maxTarget   = maxActivity - 16 // room for the verb
)

// ToolActivity describes a Claude Code tool call for the activity line,
// in English (the canonical text): "Editing main.go", "Bash: go test ./...".
func ToolActivity(tool string, input json.RawMessage) string {
	kind, target := ToolActivityParts(tool, input)
	return i18n.ActivityText(i18n.EN, kind, target)
}

// ToolActivityParts is the structured form of ToolActivity: an activity
// kind (i18n.Act*) and its target (file name, command, pattern...), which
// UIs render in their own language. Field names follow the tool_input
// shapes of the hooks reference
// (https://code.claude.com/docs/en/hooks#pretooluse-input).
func ToolActivityParts(tool string, input json.RawMessage) (kind, target string) {
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
	arg := func(s string) string { return clipTo(s, maxTarget) }
	switch tool {
	case "Edit", "Write", "MultiEdit":
		return i18n.ActEdit, arg(base(str("file_path", "path")))
	case "NotebookEdit":
		return i18n.ActEdit, arg(base(str("notebook_path", "file_path")))
	case "Read":
		return i18n.ActRead, arg(base(str("file_path", "path")))
	case "Bash":
		return i18n.ActBash, clipTo(str("command"), maxCommand)
	case "Grep", "Glob":
		return i18n.ActSearch, arg(str("pattern"))
	case "WebFetch":
		u := str("url")
		if pu, err := url.Parse(u); err == nil && pu.Host != "" {
			u = pu.Host
		}
		return i18n.ActWeb, arg(u)
	case "WebSearch":
		return i18n.ActWeb, arg(str("query"))
	case "Task", "Agent":
		return i18n.ActSubagent, arg(str("description", "subagent_type"))
	case "TodoWrite":
		return i18n.ActTodo, ""
	}
	if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
		server, name, _ := strings.Cut(rest, "__")
		if name != "" {
			return i18n.ActMCP, arg(server + "." + name)
		}
		return i18n.ActMCP, arg(server)
	}
	return i18n.ActTool, clipTo(tool, maxActivity)
}

// withActivity sets ev's activity from its structured form.
func withActivity(ev status.Event, kind, target string) status.Event {
	ev.Activity = i18n.ActivityText(i18n.EN, kind, target)
	ev.ActivityKind, ev.ActivityTarget = kind, target
	return ev
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
		return clip(i18n.T(i18n.EN, i18n.PermissionFor, tool))
	}
	return ""
}

// SessionRef extracts the agent's own session id from a hook payload:
// what `claude --resume <id>` / `codex resume <id>` need to continue
// that conversation after a daemon restart. Claude Code sends session_id
// (and transcript_path) with every hook event; codex's notify payload has
// thread-id. Empty when the payload has none (or does not parse).
func SessionRef(agent string, payload []byte) (id, transcript string) {
	switch agent {
	case AgentClaude:
		var p struct {
			SessionID      string `json:"session_id"`
			TranscriptPath string `json:"transcript_path"`
		}
		if json.Unmarshal(payload, &p) == nil && validRef(p.SessionID) {
			return p.SessionID, p.TranscriptPath
		}
	case AgentCodex:
		var p struct {
			ThreadID string `json:"thread-id"`
		}
		if json.Unmarshal(payload, &p) == nil && validRef(p.ThreadID) {
			return p.ThreadID, ""
		}
	}
	return "", ""
}

// validRef accepts what agents use as session ids (UUIDs, slugs): it ends
// up as a command-line argument, so nothing that could pass for a flag.
func validRef(s string) bool {
	if s == "" || len(s) > 128 || s[0] == '-' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
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

// ClaudeEvent is the hook_event_name of a Claude Code payload and, for
// Stop, whether Claude is already continuing because of a Stop hook
// (stop_hook_active: blocking again could loop).
func ClaudeEvent(payload []byte) (event string, stopActive bool) {
	var p struct {
		HookEventName  string `json:"hook_event_name"`
		StopHookActive bool   `json:"stop_hook_active"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return "", false
	}
	return p.HookEventName, p.StopHookActive
}
