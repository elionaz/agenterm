package notifycmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/protocol"
)

func take(ms ...mailbox.Message) protocol.MsgTakeResult {
	return protocol.MsgTakeResult{Enabled: true, Self: "me", Messages: ms}
}

var msg = mailbox.Message{ID: 7, From: "ab12", FromName: "claude", To: "me", Body: "rebase", At: time.Now()}

func TestClaudeOutput(t *testing.T) {
	out, ids := claudeOutput(hooks.ClaudeHook{Event: evPostToolUse}, take(msg))
	var v struct {
		H struct {
			Event string `json:"hookEventName"`
			Ctx   string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out, &v); err != nil || v.H.Event != evPostToolUse || !strings.Contains(v.H.Ctx, `session="ab12"`) {
		t.Fatalf("PostToolUse: %s (%v)", out, err)
	}
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("ids %v", ids)
	}

	out, _ = claudeOutput(hooks.ClaudeHook{Event: evStop}, take(msg))
	if !strings.Contains(string(out), `"decision":"block"`) {
		t.Fatalf("Stop: %s", out)
	}
}

func TestClaudeOutputHoldsBack(t *testing.T) {
	for name, h := range map[string]hooks.ClaudeHook{
		"stop already continued": {Event: evStop, StopActive: true},
		"subagent tool call":     {Event: evPostToolUse, AgentID: "sub1"},
	} {
		if out, ids := claudeOutput(h, take(msg)); out != nil || ids != nil {
			t.Errorf("%s: printed %s (ids %v); the messages must stay pending", name, out, ids)
		}
	}
	if out, _ := claudeOutput(hooks.ClaudeHook{Event: evUserPromptSubmit}, protocol.MsgTakeResult{Messages: []mailbox.Message{msg}}); out != nil {
		t.Errorf("messages off: printed %s", out)
	}
	if out, _ := claudeOutput(hooks.ClaudeHook{Event: evPostToolUse}, take()); out != nil {
		t.Errorf("nothing pending: printed %s", out)
	}
}

func TestIntroOnlyAtStart(t *testing.T) {
	r := take()
	r.Peers = &protocol.MsgPeers{ProjectName: "demo", Peers: []protocol.MsgPeer{{ID: "x1", Name: "codex", ProfileID: "codex", State: "idle"}}}
	out, ids := claudeOutput(hooks.ClaudeHook{Event: evSessionStart}, r)
	if !strings.Contains(string(out), "x1") || len(ids) != 0 {
		t.Fatalf("intro: %s %v", out, ids)
	}
}

func TestClaudeOutputInstructions(t *testing.T) {
	const instr = "Standing instructions from your user: use pnpm."
	ctxOf := func(out []byte) string {
		var v struct {
			H struct {
				Ctx string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal(out, &v)
		return v.H.Ctx
	}
	// Messages off: the instructions still arrive at start, alone.
	off := protocol.MsgTakeResult{Self: "me", Instructions: instr, Peers: &protocol.MsgPeers{ProjectName: "web"},
		Messages: []mailbox.Message{msg}}
	out, ids := claudeOutput(hooks.ClaudeHook{Event: evSessionStart}, off)
	if got := ctxOf(out); got != instr || len(ids) != 0 {
		t.Fatalf("messages off: ctx %q ids %v", got, ids)
	}
	// Messages on: instructions first, then the introduction.
	on := take()
	on.Instructions, on.Peers = instr, &protocol.MsgPeers{ProjectName: "web"}
	got := ctxOf(func() []byte { o, _ := claudeOutput(hooks.ClaudeHook{Event: evSessionStart}, on); return o }())
	if !strings.HasPrefix(got, instr+"\n\n") || !strings.Contains(got, "panal session") {
		t.Fatalf("messages on: %q", got)
	}
	// Only at start, and never into a subagent.
	if out, _ := claudeOutput(hooks.ClaudeHook{Event: evPostToolUse}, on); out != nil {
		t.Fatalf("PostToolUse printed %s", out)
	}
	if out, _ := claudeOutput(hooks.ClaudeHook{Event: evSessionStart, AgentID: "sub"}, on); out != nil {
		t.Fatalf("subagent printed %s", out)
	}
}
