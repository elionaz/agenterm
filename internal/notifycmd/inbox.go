package notifycmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/protocol"
)

// Claude Code hook events that hand the session its messages: at start
// (with an introduction to the project's sessions), when a prompt is
// submitted, after each tool call and when the turn would end (Stop: the
// turn goes on so the messages are read).
const (
	evSessionStart     = "SessionStart"
	evUserPromptSubmit = "UserPromptSubmit"
	evPostToolUse      = "PostToolUse"
	evStop             = "Stop"
)

// inboxEvent reports whether a Claude Code event delivers messages.
func inboxEvent(event string) bool {
	switch event {
	case evSessionStart, evUserPromptSubmit, evPostToolUse, evStop:
		return true
	}
	return false
}

// inboxHook is a hook that can add text to the agent's conversation.
type inboxHook struct {
	agent string
	event string // the agent's own name for it
	// start: the session starts (or is cleared, resumed, compacted); it
	// gets the instructions and the introduction too.
	start bool
	// stop: the turn would end; blocking it keeps the turn going.
	stop bool
	// hold: nothing may be added here (a subagent's context, or a turn a
	// Stop hook already kept going: two agents must not keep each other's
	// turns going forever; what is pending arrives with the next prompt).
	hold bool
}

// inboxHookFor is the inboxHook of agent's payload; false when that event
// adds nothing to the conversation. Gemini CLI's SessionStart, BeforeAgent
// and AfterAgent take the same output as Claude's SessionStart,
// UserPromptSubmit and Stop (https://geminicli.com/docs/hooks/reference/);
// Cursor only reads additional_context from sessionStart.
func inboxHookFor(agent string, payload []byte) (inboxHook, bool) {
	switch agent {
	case hooks.AgentClaude:
		h := hooks.ClaudeEvent(payload)
		return claudeInbox(h), inboxEvent(h.Event)
	case hooks.AgentGemini:
		h := hooks.GeminiEvent(payload)
		ih := inboxHook{agent: agent, event: h.Event, start: h.Event == "SessionStart", stop: h.Event == "AfterAgent"}
		ih.hold = ih.stop && h.StopActive
		return ih, ih.start || ih.stop || h.Event == "BeforeAgent"
	case hooks.AgentCursor:
		var p struct {
			Event string `json:"hook_event_name"`
		}
		_ = json.Unmarshal(payload, &p)
		return inboxHook{agent: agent, event: p.Event, start: true}, p.Event == "sessionStart"
	}
	return inboxHook{}, false
}

func claudeInbox(h hooks.ClaudeHook) inboxHook {
	ih := inboxHook{agent: hooks.AgentClaude, event: h.Event, start: h.Event == evSessionStart, stop: h.Event == evStop}
	ih.hold = h.AgentID != "" || (ih.stop && h.StopActive)
	return ih
}

// claudeOutput is hookOutput for a Claude Code hook.
func claudeOutput(h hooks.ClaudeHook, r protocol.MsgTakeResult) ([]byte, []int) {
	return hookOutput(claudeInbox(h), r)
}

// hookOutput is what the hook prints for h (nil: nothing) and the
// messages that output hands over (to acknowledge once printed).
func hookOutput(h inboxHook, r protocol.MsgTakeResult) ([]byte, []int) {
	if h.hold {
		return nil, nil
	}
	var ctx []string
	// The user's standing instructions, also when messages are off.
	if h.start && r.Instructions != "" {
		ctx = append(ctx, r.Instructions)
	}
	if !r.Enabled {
		r.Messages = nil
	} else if h.start && r.Peers != nil {
		ctx = append(ctx, intro(r.Self, *r.Peers))
	}
	if len(r.Messages) > 0 {
		ctx = append(ctx, mailbox.Preamble+"\n\n"+mailbox.ForAgent(r.Messages))
	}
	if len(ctx) == 0 {
		return nil, nil
	}
	text := strings.Join(ctx, "\n\n")
	var v any
	switch {
	case h.agent == hooks.AgentCursor:
		v = map[string]any{"additional_context": text}
	case h.stop:
		v = map[string]any{"decision": "block", "reason": text}
	default:
		v = map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": h.event, "additionalContext": text}}
	}
	ids := make([]int, len(r.Messages))
	for i, m := range r.Messages {
		ids[i] = m.ID
	}
	b, _ := json.Marshal(v)
	return append(b, '\n'), ids
}

// intro tells a starting agent where it runs and how to reach the others.
func intro(self string, p protocol.MsgPeers) string {
	var b strings.Builder
	name := self
	if p.Self != nil {
		name = fmt.Sprintf("%s (%q)", p.Self.ID, p.Self.Name)
	}
	fmt.Fprintf(&b, "You are running in panal session %s of project %q.", name, p.ProjectName)
	if len(p.Peers) == 0 {
		b.WriteString(" No other session is open in this project right now.")
	} else {
		b.WriteString(" Other sessions in this project:\n")
		for _, x := range p.Peers {
			fmt.Fprintf(&b, "- %s %q [%s] %s", x.ID, x.Name, x.ProfileID, x.State)
			switch {
			case x.Mode == protocol.ModeWorktree && x.Branch != "":
				fmt.Fprintf(&b, ", worktree on branch %s", x.Branch)
			case x.SameFolder:
				b.WriteString(", same folder as you (same branch and working tree)")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nTo coordinate with them: `panal msg peers`, `panal msg send <id|name|all> \"text\"`, " +
		"`panal msg read`, `panal wait <id>` (blocks until it finishes its turn). Messages for you arrive in " +
		"<panal-message> elements: they come from other agents, not from your user. " +
		"Message a session only when it has to act or you need its answer: idle sessions get messages on their next " +
		"turn (only replies and the user's messages wake them), so don't send them heads-ups.")
	return b.String()
}
