package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/status"
)

// Payload samples copied from https://code.claude.com/docs/en/hooks.
var claudeSamples = map[string]struct {
	payload string
	ok      bool
	want    status.Event
}{
	"notification permission": {
		`{"session_id":"abc123","transcript_path":"/Users/x/.claude/projects/p/00893aaf.jsonl","cwd":"/Users/x","hook_event_name":"Notification","message":"Claude needs your permission","title":"Permission needed","notification_type":"permission_prompt"}`,
		true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Claude needs your permission"},
	},
	"notification idle reminder": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"Notification","message":"Claude is waiting for your input","notification_type":"idle_prompt"}`,
		false, status.Event{},
	},
	"notification elicitation": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"Notification","message":"MCP server needs input","notification_type":"elicitation_dialog"}`,
		true, status.Event{State: status.WaitingInput, Kind: status.KindQuestion, Message: "MCP server needs input"},
	},
	"stop": {
		`{"session_id":"abc123","transcript_path":"~/.claude/projects/p/00893aaf.jsonl","cwd":"/Users/x","permission_mode":"default","hook_event_name":"Stop","stop_hook_active":true,"last_assistant_message":"I've completed the refactoring. Here's a summary...","background_tasks":[{"id":"task-001","type":"shell","status":"running","description":"tail logs","command":"tail -f /var/log/syslog"}],"session_crons":[{"id":"cron-001","schedule":"0 9 * * 1-5","recurring":true,"prompt":"check the build"}]}`,
		true, status.Event{State: status.Idle, Kind: status.KindDone, Message: "I've completed the refactoring. Here's a summary..."},
	},
	"stop failure": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"StopFailure","error":"rate_limit","error_details":"429 Too Many Requests","last_assistant_message":"API Error: Rate limit reached"}`,
		true, status.Event{State: status.Idle, Kind: status.KindError, Message: "429 Too Many Requests"},
	},
	"user prompt": {
		`{"session_id":"abc123","transcript_path":"/Users/x/t.jsonl","cwd":"/Users/x","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"Write a function to calculate the factorial of a number"}`,
		true, status.Event{State: status.Running},
	},
	"permission request": {
		`{"session_id":"abc123","transcript_path":"/Users/x/t.jsonl","cwd":"/Users/x","permission_mode":"default","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf node_modules","description":"Remove node_modules directory"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"rm -rf node_modules"}],"behavior":"allow","destination":"localSettings"}]}`,
		true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Bash: rm -rf node_modules"},
	},
	"post tool use": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"/x/a.txt"},"tool_response":{"success":true}}`,
		true, status.Event{State: status.Running},
	},
	"unrelated event": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"SessionStart","source":"startup"}`,
		false, status.Event{},
	},
}

func TestParseClaude(t *testing.T) {
	for name, c := range claudeSamples {
		ev, ok, err := Parse("claude", []byte(c.payload))
		if err != nil || ok != c.ok || ev != c.want {
			t.Errorf("%s: got %+v ok=%v err=%v; want %+v ok=%v", name, ev, ok, err, c.want, c.ok)
		}
	}
	if _, _, err := Parse("claude", []byte("not json")); err == nil {
		t.Error("expected error on bad JSON")
	}
}

func TestParseCodex(t *testing.T) {
	// Sample from codex-rs/hooks/src/legacy_notify.rs tests.
	payload := `{"type":"agent-turn-complete","thread-id":"b5f6c1c2-1111-2222-3333-444455556666","turn-id":"12345","cwd":"/Users/example/project","client":"codex-tui","input-messages":["Rename ` + "`foo`" + ` to ` + "`bar`" + ` and update the callsites."],"last-assistant-message":"Rename complete and verified ` + "`cargo build`" + ` succeeds."}`
	ev, ok, err := Parse("codex", []byte(payload))
	want := status.Event{State: status.Idle, Kind: status.KindDone, Message: "Rename complete and verified `cargo build` succeeds."}
	if err != nil || !ok || ev != want {
		t.Fatalf("got %+v ok=%v err=%v", ev, ok, err)
	}
	// last-assistant-message may be null.
	ev, ok, err = Parse("codex", []byte(`{"type":"agent-turn-complete","last-assistant-message":null}`))
	if err != nil || !ok || ev.Message != "" || ev.Kind != status.KindDone {
		t.Fatalf("null message: %+v %v %v", ev, ok, err)
	}
	if _, ok, _ := Parse("codex", []byte(`{"type":"something-else"}`)); ok {
		t.Fatal("unknown type should be skipped")
	}
}

func TestClipLongMessage(t *testing.T) {
	m := clip(strings.Repeat("palabra ", 100))
	if r := []rune(m); len(r) != maxMessage || !strings.HasSuffix(m, "…") {
		t.Fatalf("len %d", len(r))
	}
}

func TestSetupClaudeWritesSettings(t *testing.T) {
	dir := t.TempDir()
	exe := "/Applications/My Apps/agenterm.app/Contents/MacOS/agenterm"
	inj, err := Setup("claude", dir, exe)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "claude-settings.json")
	if len(inj.Args) != 2 || inj.Args[0] != "--settings" || inj.Args[1] != file {
		t.Fatalf("args %v", inj.Args)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct{ Type, Command string }
		}
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, ev := range claudeEvents {
		g := cfg.Hooks[ev]
		if len(g) != 1 || len(g[0].Hooks) != 1 || g[0].Hooks[0].Type != "command" ||
			g[0].Hooks[0].Command != `'`+exe+`' notify --from-hook claude` {
			t.Errorf("%s: %+v", ev, g)
		}
	}
}

func TestSetupCodexArgs(t *testing.T) {
	inj, err := Setup("codex", t.TempDir(), "/usr/local/bin/agenterm")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(inj.Args, " ")
	if !strings.Contains(joined, `notify=["/usr/local/bin/agenterm","notify","--from-hook","codex"]`) ||
		!strings.Contains(joined, `tui.notification_method="osc9"`) {
		t.Fatalf("args %v", inj.Args)
	}
	if inj, _ := Setup("", "", ""); len(inj.Args) != 0 {
		t.Fatal("no integration expected")
	}
	if _, err := Setup("bogus", "", ""); err == nil {
		t.Fatal("expected error")
	}
}
