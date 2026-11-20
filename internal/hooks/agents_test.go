package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/status"
)

type sample struct {
	payload string
	ok      bool
	want    status.Event
}

func checkSamples(t *testing.T, agent string, samples map[string]sample) {
	t.Helper()
	for name, c := range samples {
		ev, ok, err := Parse(agent, []byte(c.payload))
		if err != nil || ok != c.ok || ev != c.want {
			t.Errorf("%s %s: got %+v ok=%v err=%v; want %+v ok=%v", agent, name, ev, ok, err, c.want, c.ok)
		}
	}
	if _, _, err := Parse(agent, []byte("not json")); err == nil {
		t.Errorf("%s: expected error on bad JSON", agent)
	}
}

// Payload shapes from https://geminicli.com/docs/hooks/reference/ (base
// fields: session_id, transcript_path, cwd, hook_event_name, timestamp).
func TestParseGemini(t *testing.T) {
	const b = `"session_id":"6c1f0d3e-5a7b-4c2d-9e8f-0123456789ab","transcript_path":"/u/.gemini/tmp/p/chats/session-x.jsonl","cwd":"/p","timestamp":"2026-11-20T10:00:00Z",`
	checkSamples(t, AgentGemini, map[string]sample{
		"before agent": {`{` + b + `"hook_event_name":"BeforeAgent","prompt":"fix the tests"}`,
			true, status.Event{State: status.Running, Turn: true, Activity: "Thinking…", ActivityKind: "thinking"}},
		"before tool shell": {`{` + b + `"hook_event_name":"BeforeTool","tool_name":"run_shell_command","tool_input":{"command":"npm test","description":"run tests"}}`,
			true, status.Event{State: status.Running, Activity: "Bash: npm test", ActivityKind: "bash", ActivityTarget: "npm test"}},
		"before tool write": {`{` + b + `"hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"file_path":"/p/src/a.ts","content":"x"}}`,
			true, status.Event{State: status.Running, Activity: "Editing a.ts", ActivityKind: "edit", ActivityTarget: "a.ts"}},
		"before tool mcp": {`{` + b + `"hook_event_name":"BeforeTool","tool_name":"mcp_github_create_issue","tool_input":{},"mcp_context":{"server_name":"github","tool_name":"create_issue"}}`,
			true, status.Event{State: status.Running, Activity: "mcp_github_create_issue", ActivityKind: "tool", ActivityTarget: "mcp_github_create_issue"}},
		"after tool": {`{` + b + `"hook_event_name":"AfterTool","tool_name":"read_file","tool_input":{},"tool_response":{"llmContent":"x","returnDisplay":"x"}}`,
			true, status.Event{State: status.Running}},
		"permission exec": {`{` + b + `"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool Shell requires execution","details":{"type":"exec","title":"Shell","command":"rm -rf build","rootCommand":"rm"}}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Shell: rm -rf build"}},
		"permission edit": {`{` + b + `"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool Edit requires editing","details":{"type":"edit","title":"Edit","fileName":"main.go","filePath":"/p/main.go"}}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Edit: main.go"}},
		"permission mcp": {`{` + b + `"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool requires MCP","details":{"type":"mcp","title":"x","serverName":"github","toolName":"create_issue"}}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "MCP: github.create_issue"}},
		"permission bare": {`{` + b + `"hook_event_name":"Notification","notification_type":"ToolPermission","message":"Tool X requires information"}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Tool X requires information"}},
		"other notification": {`{` + b + `"hook_event_name":"Notification","notification_type":"Other","message":"x"}`, false, status.Event{}},
		"after agent": {`{` + b + `"hook_event_name":"AfterAgent","prompt":"fix","prompt_response":"All tests pass now.","stop_hook_active":false}`,
			true, status.Event{State: status.Idle, Kind: status.KindDone, Message: "All tests pass now."}},
		"session start": {`{` + b + `"hook_event_name":"SessionStart","source":"startup"}`, false, status.Event{}},
	})
	id, tr := SessionRef(AgentGemini, []byte(`{`+b+`"hook_event_name":"SessionStart","source":"resume"}`))
	if id != "6c1f0d3e-5a7b-4c2d-9e8f-0123456789ab" || tr != "/u/.gemini/tmp/p/chats/session-x.jsonl" {
		t.Fatalf("session ref %q %q", id, tr)
	}
	if h := GeminiEvent([]byte(`{` + b + `"hook_event_name":"AfterAgent","stop_hook_active":true}`)); h.Event != "AfterAgent" || !h.StopActive {
		t.Fatalf("GeminiEvent %+v", h)
	}
}

func TestSetupGemini(t *testing.T) {
	dir, sys := t.TempDir(), t.TempDir()
	real := filepath.Join(sys, "settings.json")
	t.Setenv(geminiSystemEnv, real)
	t.Setenv(geminiDefaultsEnv, "")

	// No system file: just panal's hooks.
	inj, err := Setup(AgentGemini, dir, "/usr/local/bin/panal", "/tmp/a b/d.sock")
	if err != nil || inj.Note != "" || len(inj.Args) != 0 {
		t.Fatalf("inj %+v err %v", inj, err)
	}
	file := filepath.Join(dir, "gemini-settings.json")
	want := []string{geminiSystemEnv + "=" + file, geminiDefaultsEnv + "=" + filepath.Join(sys, "system-defaults.json")}
	if strings.Join(inj.Env, "\n") != strings.Join(want, "\n") {
		t.Fatalf("env %v", inj.Env)
	}
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type, Command string
				Timeout       int
			}
		}
	}
	data, _ := os.ReadFile(file)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, ev := range geminiEvents {
		g := cfg.Hooks[ev]
		if len(g) != 1 || g[0].Hooks[0].Type != "command" ||
			g[0].Hooks[0].Command != `/usr/local/bin/panal notify --sock '/tmp/a b/d.sock' --from-hook gemini` || g[0].Hooks[0].Timeout < 1000 {
			t.Errorf("%s: %+v", ev, g)
		}
	}

	// A system file: kept, its hooks first.
	os.WriteFile(real, []byte(`{"general":{"vimMode":true},"hooks":{"BeforeTool":[{"matcher":"write_file","hooks":[{"type":"command","command":"audit.sh"}]}]}}`), 0o600)
	if _, err := Setup(AgentGemini, dir, "/usr/local/bin/panal", ""); err != nil {
		t.Fatal(err)
	}
	var merged map[string]any
	data, _ = os.ReadFile(file)
	json.Unmarshal(data, &merged)
	if merged["general"] == nil {
		t.Fatalf("system settings dropped: %s", data)
	}
	bt := merged["hooks"].(map[string]any)["BeforeTool"].([]any)
	if len(bt) != 2 || !strings.Contains(string(data), "audit.sh") {
		t.Fatalf("BeforeTool %v", bt)
	}

	// One panal can't read back (comments): left in charge, no hooks.
	os.WriteFile(real, []byte("// managed\n{}"), 0o600)
	inj, err = Setup(AgentGemini, dir, "/usr/local/bin/panal", "")
	if err != nil || inj.Note == "" || len(inj.Env) != 0 {
		t.Fatalf("commented system file: %+v %v", inj, err)
	}
}

func TestParseOpencode(t *testing.T) {
	const s = `"sessionID":"ses_01dc6e014ffeNfBVUYDRvNkIJv"`
	checkSamples(t, AgentOpencode, map[string]sample{
		"execution started": {`{"event":"session.execution.started",` + s + `}`,
			true, status.Event{State: status.Running, Turn: true, Activity: "Thinking…", ActivityKind: "thinking"}},
		"tool bash": {`{"event":"session.tool.called",` + s + `,"tool":"bash","input":{"command":"go test ./...","description":"tests"}}`,
			true, status.Event{State: status.Running, Activity: "Bash: go test ./...", ActivityKind: "bash", ActivityTarget: "go test ./..."}},
		"tool edit": {`{"event":"session.tool.called",` + s + `,"tool":"edit","input":{"filePath":"/p/main.go","oldString":"a","newString":"b"}}`,
			true, status.Event{State: status.Running, Activity: "Editing main.go", ActivityKind: "edit", ActivityTarget: "main.go"}},
		"permission": {`{"event":"permission.asked",` + s + `,"action":"bash","resources":["rm -rf dist"]}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "bash: rm -rf dist"}},
		"permission with message": {`{"event":"permission.asked",` + s + `,"action":"edit","resources":["a.go"],"message":"Edit a.go?"}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindPermission, Message: "Edit a.go?"}},
		"replied": {`{"event":"permission.replied",` + s + `}`, true, status.Event{State: status.Running}},
		"form": {`{"event":"form.created",` + s + `,"title":"Which database?"}`,
			true, status.Event{State: status.WaitingInput, Kind: status.KindQuestion, Message: "Which database?"}},
		"form replied": {`{"event":"form.replied",` + s + `}`, true, status.Event{State: status.Running}},
		"succeeded":    {`{"event":"session.execution.succeeded",` + s + `}`, true, status.Event{State: status.Idle, Kind: status.KindDone}},
		"failed": {`{"event":"session.execution.failed",` + s + `,"error":"429 rate limited"}`,
			true, status.Event{State: status.Idle, Kind: status.KindError, Message: "429 rate limited"}},
		"interrupted": {`{"event":"session.execution.interrupted",` + s + `,"reason":"user"}`,
			true, status.Event{State: status.Idle, Kind: status.KindDone, Message: "Interrupted"}},
		"superseded": {`{"event":"session.execution.interrupted",` + s + `,"reason":"superseded"}`, false, status.Event{}},
		"created":    {`{"event":"session.created",` + s + `,"model":"claude-sonnet-5-5"}`, false, status.Event{}},
		"model":      {`{"event":"session.step.started",` + s + `,"model":"gpt-5"}`, false, status.Event{}},
	})
	if id, _ := SessionRef(AgentOpencode, []byte(`{"event":"session.created",`+s+`}`)); id != "ses_01dc6e014ffeNfBVUYDRvNkIJv" {
		t.Fatalf("session ref %q", id)
	}
	if m := Model(AgentOpencode, []byte(`{"event":"session.step.started",`+s+`,"model":"gpt-5"}`)); m != "gpt-5" {
		t.Fatalf("model %q", m)
	}
}

func TestSetupOpencode(t *testing.T) {
	t.Setenv(opencodeConfigEnv, "")
	dir := t.TempDir()
	inj, err := Setup(AgentOpencode, dir, "/usr/local/bin/panal", "/s.sock")
	if err != nil || len(inj.Args) != 1 || inj.Args[0] != "--standalone" || len(inj.Env) != 1 {
		t.Fatalf("inj %+v err %v", inj, err)
	}
	pdir := filepath.Join(dir, opencodePluginDir)
	k, v, _ := strings.Cut(inj.Env[0], "=")
	var cfg struct {
		Plugins []struct {
			Package string `json:"package"`
		} `json:"plugins"`
		Instructions []string `json:"instructions"`
	}
	if k != opencodeConfigEnv || json.Unmarshal([]byte(v), &cfg) != nil || len(cfg.Plugins) != 1 || cfg.Plugins[0].Package != pdir || cfg.Instructions != nil {
		t.Fatalf("env %q", inj.Env[0])
	}
	js, err := os.ReadFile(filepath.Join(pdir, "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`["/usr/local/bin/panal","notify","--sock","/s.sock","--from-hook","opencode"]`,
		`id: "panal"`, `"session.execution.failed"`, `"permission.asked"`, `ctx.event.subscribe(`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("plugin lacks %s", want)
		}
	}
	// The session's instructions join the same document.
	kv := OpencodeInstructionsEnv(inj, dir, "/h/instructions/p1.md")
	if !strings.Contains(kv, `"instructions":["/h/instructions/p1.md"]`) || !strings.Contains(kv, pdir) {
		t.Fatalf("instructions env %q", kv)
	}
	if OpencodeInstructionsEnv(Injection{}, dir, "/x.md") != "" {
		t.Fatal("no integration, no instructions env")
	}
	// The user's own inline config is not replaced.
	t.Setenv(opencodeConfigEnv, `{"theme":"x"}`)
	if inj, _ := Setup(AgentOpencode, dir, "/p", ""); inj.Note == "" || len(inj.Env) != 0 || len(inj.Args) != 0 {
		t.Fatalf("own config: %+v", inj)
	}
}

func TestOpencodeVersion(t *testing.T) {
	for s, want := range map[string]bool{"opencode v2.1.4\n": true, "2.3.2": true, "1.19.6": false, "v10.0.0": true, "": false, "dev": false} {
		if opencodeVersionOK(s) != want {
			t.Errorf("%q: want %v", s, want)
		}
	}
}

// A sessionStart payload recorded from cursor-agent 2026.11.19 (with
// --plugin-dir), user_email changed.
const cursorStart = `{"conversation_id":"d82ff75b-a8e8-4a47-b9e9-0711ca9cb96d","generation_id":"d82ff75b-a8e8-4a47-b9e9-0711ca9cb96d","model":"grok-4.7-high-fast","is_background_agent":false,"session_id":"d82ff75b-a8e8-4a47-b9e9-0711ca9cb96d","hook_event_name":"sessionStart","cursor_version":"2026.11.19-f005e9d","workspace_roots":["/p"],"user_email":"user@example.com","transcript_path":null}`

// Other shapes from https://cursor.com/docs/agent/hooks.
func TestParseCursor(t *testing.T) {
	const b = `"conversation_id":"d82ff75b-a8e8-4a47-b9e9-0711ca9cb96d","generation_id":"g2","model":"gpt-5","cursor_version":"2026.11.19","workspace_roots":["/p"],`
	checkSamples(t, AgentCursor, map[string]sample{
		"session start": {cursorStart, false, status.Event{}},
		"prompt": {`{` + b + `"hook_event_name":"beforeSubmitPrompt","prompt":"hi","attachments":[]}`,
			true, status.Event{State: status.Running, Turn: true, Activity: "Thinking…", ActivityKind: "thinking"}},
		"pre tool shell": {`{` + b + `"hook_event_name":"preToolUse","tool_name":"Shell","tool_input":{"command":"npm run build"},"tool_use_id":"abc123","cwd":"/p"}`,
			true, status.Event{State: status.Running, Activity: "Bash: npm run build", ActivityKind: "bash", ActivityTarget: "npm run build"}},
		"pre tool mcp": {`{` + b + `"hook_event_name":"preToolUse","tool_name":"MCP:create_issue","tool_input":{}}`,
			true, status.Event{State: status.Running, Activity: "MCP: create_issue", ActivityKind: "mcp", ActivityTarget: "create_issue"}},
		"post tool": {`{` + b + `"hook_event_name":"postToolUse","tool_name":"Read","tool_output":"{}","duration":12}`, true, status.Event{State: status.Running}},
		"stop":      {`{` + b + `"hook_event_name":"stop","status":"completed","loop_count":0}`, true, status.Event{State: status.Idle, Kind: status.KindDone}},
		"stop error": {`{` + b + `"hook_event_name":"stop","status":"error","loop_count":0}`,
			true, status.Event{State: status.Idle, Kind: status.KindError}},
		"other": {`{` + b + `"hook_event_name":"afterFileEdit","file_path":"/p/a"}`, false, status.Event{}},
	})
	if id, _ := SessionRef(AgentCursor, []byte(cursorStart)); id != "d82ff75b-a8e8-4a47-b9e9-0711ca9cb96d" {
		t.Fatalf("session ref %q", id)
	}
	if m := Model(AgentCursor, []byte(cursorStart)); m != "grok-4.7-high-fast" {
		t.Fatalf("model %q", m)
	}
}

func TestSetupCursor(t *testing.T) {
	dir := t.TempDir()
	inj, err := Setup(AgentCursor, dir, "/usr/local/bin/panal", "/s.sock")
	pdir := filepath.Join(dir, cursorPluginDir)
	if err != nil || strings.Join(inj.Args, " ") != "--plugin-dir "+pdir || len(inj.Env) != 0 {
		t.Fatalf("inj %+v err %v", inj, err)
	}
	var m struct{ Name string }
	data, _ := os.ReadFile(filepath.Join(pdir, ".cursor-plugin", "plugin.json"))
	if json.Unmarshal(data, &m) != nil || m.Name != "panal" {
		t.Fatalf("manifest %s", data)
	}
	var cfg struct {
		Version int
		Hooks   map[string][]struct {
			Command string
			Timeout int
		}
	}
	data, _ = os.ReadFile(filepath.Join(pdir, "hooks", "hooks.json"))
	if json.Unmarshal(data, &cfg) != nil || cfg.Version != 1 {
		t.Fatalf("hooks.json %s", data)
	}
	for _, ev := range cursorEvents {
		h := cfg.Hooks[ev]
		if len(h) != 1 || h[0].Command != "/usr/local/bin/panal notify --sock /s.sock --from-hook cursor" || h[0].Timeout <= 0 || h[0].Timeout > 10 {
			t.Errorf("%s: %+v", ev, h)
		}
	}
}

func TestModelFromGeminiTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-x.jsonl")
	lines := `{"sessionId":"s","projectHash":"h","startTime":"t"}
{"id":"1","timestamp":"t","type":"user","content":[{"text":"hi"}]}
{"id":"2","timestamp":"t","type":"gemini","content":[{"functionCall":{"name":"x","args":{"model":"not-this"}}}],"thoughts":[],"tokens":null,"model":"gemini-2.5-pro"}
{"$set":{"lastUpdated":"t"}}
`
	os.WriteFile(path, []byte(lines), 0o600)
	p := `{"session_id":"s","hook_event_name":"AfterAgent","transcript_path":"` + path + `"}`
	if m := Model(AgentGemini, []byte(p)); m != "gemini-2.5-pro" {
		t.Fatalf("model %q", m)
	}
	if m := Model(AgentGemini, []byte(strings.Replace(p, "AfterAgent", "BeforeTool", 1))); m != "" {
		t.Fatalf("BeforeTool reported %q", m)
	}
}

func TestReceivers(t *testing.T) {
	for _, a := range Agents {
		if !ReceivesInstructions(a) || !Known(a) {
			t.Errorf("%s", a)
		}
	}
	if ReceivesInstructions("") || ReceivesInstructions("none") || Known("bogus") {
		t.Fatal("no integration")
	}
	if TakesPrompt(AgentGemini) || TakesPrompt(AgentOpencode) || !TakesPrompt(AgentClaude) {
		t.Fatal("TakesPrompt")
	}
	if !DeliversMessages(AgentGemini) || DeliversMessages(AgentCursor) {
		t.Fatal("DeliversMessages")
	}
}
