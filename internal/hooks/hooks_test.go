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
		true, status.Event{State: status.Running, Turn: true, Activity: "Thinking…", ActivityKind: "thinking"},
	},
	"permission request": {
		`{"session_id":"abc123","transcript_path":"/Users/x/t.jsonl","cwd":"/Users/x","permission_mode":"default","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf node_modules","description":"Remove node_modules directory"},"permission_suggestions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"rm -rf node_modules"}],"behavior":"allow","destination":"localSettings"}]}`,
		true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Bash: rm -rf node_modules"},
	},
	"post tool use": {
		`{"session_id":"abc123","cwd":"/x","hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"/x/a.txt"},"tool_response":{"success":true}}`,
		true, status.Event{State: status.Running},
	},
	"pre tool use (doc sample)": {
		`{"session_id":"abc123","prompt_id":"550e8400-e29b-41d4-a716-446655440000","transcript_path":"/home/user/.claude/projects/.../transcript.jsonl","cwd":"/home/user/my-project","scratchpad_dir":"/tmp/claude-1000/-home-user-my-project/abc123/scratchpad","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"npm test","description":"Run test suite","timeout":120000,"run_in_background":false},"tool_use_id":"toolu_01ABC123..."}`,
		true, status.Event{State: status.Running, Activity: "Bash: npm test", ActivityKind: "bash", ActivityTarget: "npm test"},
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
	inj, err := Setup("claude", dir, exe, "/tmp/agt x/d.sock")
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
			g[0].Hooks[0].Command != `'`+exe+`' notify --sock '/tmp/agt x/d.sock' --from-hook claude` {
			t.Errorf("%s: %+v", ev, g)
		}
	}
}

func TestSetupCodexArgs(t *testing.T) {
	inj, err := Setup("codex", t.TempDir(), "/usr/local/bin/agenterm", "/tmp/a/d.sock")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(inj.Args, " ")
	if !strings.Contains(joined, `notify=["/usr/local/bin/agenterm","notify","--sock","/tmp/a/d.sock","--from-hook","codex"]`) ||
		!strings.Contains(joined, `tui.notification_method="osc9"`) {
		t.Fatalf("args %v", inj.Args)
	}
	if inj, _ := Setup("", "", "", ""); len(inj.Args) != 0 {
		t.Fatal("no integration expected")
	}
	if _, err := Setup("bogus", "", "", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestToolActivity(t *testing.T) {
	long := strings.Repeat("x", 100)
	cases := []struct{ tool, input, want string }{
		{"Edit", `{"file_path":"/home/user/proj/internal/main.go","old_string":"a","new_string":"b","replace_all":false}`, "Editing main.go"},
		{"Write", `{"file_path":"/home/user/proj/README.md","content":"# hi"}`, "Editing README.md"},
		{"MultiEdit", `{"file_path":"/p/a.ts","edits":[]}`, "Editing a.ts"},
		{"NotebookEdit", `{"notebook_path":"/p/n.ipynb","new_source":"x"}`, "Editing n.ipynb"},
		{"Read", `{"file_path":"/home/user/proj/go.mod","offset":1,"limit":20}`, "Reading go.mod"},
		{"Bash", `{"command":"npm test","description":"Run test suite","timeout":120000,"run_in_background":false}`, "Bash: npm test"},
		{"Bash", `{"command":"echo  a\n  b"}`, "Bash: echo a b"},
		{"Bash", `{"command":"` + long + `"}`, "Bash: " + strings.Repeat("x", 59) + "…"},
		{"Grep", `{"pattern":"TODO.*","path":"/p","glob":"*.go","output_mode":"content"}`, "Searching TODO.*"},
		{"Glob", `{"pattern":"**/*.ts","path":"/p"}`, "Searching **/*.ts"},
		{"WebFetch", `{"url":"https://example.com/api/docs?x=1","prompt":"Extract the endpoints"}`, "Web: example.com"},
		{"WebSearch", `{"query":"react hooks best practices","allowed_domains":["docs.example.com"]}`, "Web: react hooks best practices"},
		{"Agent", `{"prompt":"Find all API endpoints","description":"Find API endpoints","subagent_type":"Explore","model":"sonnet"}`, "Subagent: Find API endpoints"},
		{"Task", `{"prompt":"x","subagent_type":"general-purpose"}`, "Subagent: general-purpose"},
		{"TodoWrite", `{"todos":[{"content":"a","status":"pending"}]}`, "Updating tasks"},
		{"mcp__memory__create_entities", `{"entities":[]}`, "MCP: memory.create_entities"},
		{"mcp__github__search_repositories", `{}`, "MCP: github.search_repositories"},
		{"ExitPlanMode", `{}`, "ExitPlanMode"},
		{"Read", `null`, "Reading"},
		{"Edit", ``, "Editing"},
	}
	for _, c := range cases {
		if got := ToolActivity(c.tool, json.RawMessage(c.input)); got != c.want {
			t.Errorf("%s %s: got %q want %q", c.tool, c.input, got, c.want)
		}
	}
}

func TestToolActivityParts(t *testing.T) {
	cases := []struct{ tool, input, kind, target string }{
		{"Edit", `{"file_path":"/p/main.go"}`, "edit", "main.go"},
		{"Read", `{"file_path":"/p/go.mod"}`, "read", "go.mod"},
		{"Bash", `{"command":"npm test"}`, "bash", "npm test"},
		{"Grep", `{"pattern":"TODO"}`, "search", "TODO"},
		{"WebFetch", `{"url":"https://example.com/x"}`, "web", "example.com"},
		{"Task", `{"subagent_type":"Explore"}`, "subagent", "Explore"},
		{"TodoWrite", `{}`, "todo", ""},
		{"mcp__github__search", `{}`, "mcp", "github.search"},
		{"ExitPlanMode", `{}`, "tool", "ExitPlanMode"},
	}
	for _, c := range cases {
		if k, tg := ToolActivityParts(c.tool, json.RawMessage(c.input)); k != c.kind || tg != c.target {
			t.Errorf("%s: got %q %q, want %q %q", c.tool, k, tg, c.kind, c.target)
		}
	}
}

func TestClaudeSettingsPreToolUse(t *testing.T) {
	var cfg struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct {
				Type    string
				Timeout int
			}
		}
	}
	if err := json.Unmarshal(ClaudeSettings("/x/agenterm", ""), &cfg); err != nil {
		t.Fatal(err)
	}
	g := cfg.Hooks["PreToolUse"]
	if len(g) != 1 || g[0].Matcher != "" || g[0].Hooks[0].Timeout != preToolTimeout || preToolTimeout > 5 {
		t.Fatalf("PreToolUse hook %+v", g)
	}
}

func TestSessionRef(t *testing.T) {
	// Every Claude Code hook payload carries session_id (+ transcript_path).
	for name, c := range claudeSamples {
		id, _ := SessionRef(AgentClaude, []byte(c.payload))
		if id != "abc123" {
			t.Errorf("%s: session id %q", name, id)
		}
	}
	id, tr := SessionRef(AgentClaude, []byte(`{"session_id":"0b5e7f2a-1111-2222-3333-444455556666","transcript_path":"/u/.claude/projects/p/t.jsonl","hook_event_name":"SessionStart","source":"resume"}`))
	if id != "0b5e7f2a-1111-2222-3333-444455556666" || tr != "/u/.claude/projects/p/t.jsonl" {
		t.Fatalf("claude: %q %q", id, tr)
	}
	// codex notify (legacy_notify) payload: thread-id.
	id, _ = SessionRef(AgentCodex, []byte(`{"type":"agent-turn-complete","thread-id":"019a-thread","turn-id":"t1","cwd":"/x","input-messages":["hi"],"last-assistant-message":"done"}`))
	if id != "019a-thread" {
		t.Fatalf("codex: %q", id)
	}
	for _, bad := range []string{`{}`, `not json`, `{"session_id":"--dangerous"}`, `{"session_id":"a b"}`, `{"session_id":"x;rm"}`} {
		if id, _ := SessionRef(AgentClaude, []byte(bad)); id != "" {
			t.Errorf("%s: accepted %q", bad, id)
		}
	}
	if id, _ := SessionRef("other", []byte(`{"session_id":"abc"}`)); id != "" {
		t.Fatal("unknown agent")
	}
}

func TestClaudeSettingsListenToSessionStart(t *testing.T) {
	var cfg struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(ClaudeSettings("/x/agenterm", "/s.sock"), &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Hooks["SessionStart"]; !ok {
		t.Fatalf("no SessionStart hook: %v", cfg.Hooks)
	}
	// SessionStart only reports the session id: no state change.
	if _, ok, err := Parse(AgentClaude, []byte(`{"session_id":"abc","hook_event_name":"SessionStart","source":"startup"}`)); ok || err != nil {
		t.Fatalf("SessionStart parsed as an event: %v %v", ok, err)
	}
}
