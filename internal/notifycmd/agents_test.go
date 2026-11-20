package notifycmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/hooks"
)

func TestGeminiOutput(t *testing.T) {
	hook := func(event string, stopActive bool) inboxHook {
		p, _ := json.Marshal(map[string]any{"session_id": "s1", "hook_event_name": event, "stop_hook_active": stopActive})
		h, ok := inboxHookFor(hooks.AgentGemini, p)
		if !ok {
			t.Fatalf("%s does not deliver", event)
		}
		return h
	}
	// BeforeAgent: added to the prompt, like Claude's UserPromptSubmit.
	out, ids := hookOutput(hook("BeforeAgent", false), take(msg))
	if !strings.Contains(string(out), `"hookEventName":"BeforeAgent"`) || !strings.Contains(string(out), `"additionalContext"`) || len(ids) != 1 {
		t.Fatalf("BeforeAgent: %s %v", out, ids)
	}
	// AfterAgent: blocked, with the messages as the next prompt.
	out, _ = hookOutput(hook("AfterAgent", false), take(msg))
	if !strings.Contains(string(out), `"decision":"block"`) || !strings.Contains(string(out), `"reason"`) {
		t.Fatalf("AfterAgent: %s", out)
	}
	if out, ids := hookOutput(hook("AfterAgent", true), take(msg)); out != nil || ids != nil {
		t.Fatalf("AfterAgent already continued: %s", out)
	}
	// SessionStart: the instructions.
	r := take()
	r.Instructions = "use pnpm"
	if out, _ := hookOutput(hook("SessionStart", false), r); !strings.Contains(string(out), "use pnpm") {
		t.Fatalf("SessionStart: %s", out)
	}
	if _, ok := inboxHookFor(hooks.AgentGemini, []byte(`{"hook_event_name":"BeforeTool"}`)); ok {
		t.Fatal("BeforeTool delivers")
	}
}

func TestCursorOutput(t *testing.T) {
	h, ok := inboxHookFor(hooks.AgentCursor, []byte(`{"conversation_id":"c1","hook_event_name":"sessionStart"}`))
	if !ok || !h.start {
		t.Fatalf("sessionStart: %+v %v", h, ok)
	}
	r := take()
	r.Instructions = "use pnpm"
	out, _ := hookOutput(h, r)
	var v struct {
		Ctx string `json:"additional_context"`
	}
	if err := json.Unmarshal(out, &v); err != nil || v.Ctx != "use pnpm" {
		t.Fatalf("sessionStart: %s (%v)", out, err)
	}
	for _, ev := range []string{"preToolUse", "stop", "beforeSubmitPrompt"} {
		if _, ok := inboxHookFor(hooks.AgentCursor, []byte(`{"hook_event_name":"`+ev+`"}`)); ok {
			t.Errorf("%s delivers", ev)
		}
	}
	if _, ok := inboxHookFor(hooks.AgentOpencode, []byte(`{"event":"session.created"}`)); ok {
		t.Fatal("opencode delivers")
	}
}
