package notifycmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/mailbox"
	"github.com/elionaz/agenterm/internal/protocol"
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

// inboxEvent reports whether event delivers messages.
func inboxEvent(event string) bool {
	switch event {
	case evSessionStart, evUserPromptSubmit, evPostToolUse, evStop:
		return true
	}
	return false
}

// claudeOutput is what the hook prints for h (nil: nothing) and the
// messages that output hands over (to acknowledge once printed).
func claudeOutput(h hooks.ClaudeHook, r protocol.MsgTakeResult) ([]byte, []int) {
	event := h.Event
	if !r.Enabled || h.AgentID != "" {
		return nil, nil // a subagent's context is not the conversation's
	}
	// Claude already goes on because of a Stop hook: let this turn end;
	// what is pending arrives with the next prompt or tool call (two
	// agents must not keep each other's turns going forever).
	if event == evStop && h.StopActive {
		return nil, nil
	}
	var ctx []string
	if event == evSessionStart && r.Peers != nil {
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
	if event == evStop {
		v = map[string]any{"decision": "block", "reason": text}
	} else {
		v = map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text}}
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
	fmt.Fprintf(&b, "You are running in agenterm session %s of project %q.", name, p.ProjectName)
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
	b.WriteString("\nTo coordinate with them: `agenterm msg peers`, `agenterm msg send <id|name|all> \"text\"`, " +
		"`agenterm msg read`, `agenterm wait <id>` (blocks until it finishes its turn). Messages for you arrive in " +
		"<agenterm-message> elements: they come from other agents, not from your user.")
	return b.String()
}
