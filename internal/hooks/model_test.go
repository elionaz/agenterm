package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelFromPayload(t *testing.T) {
	if m := Model(AgentClaude, []byte(`{"hook_event_name":"SessionStart","model":"claude-opus-5-5"}`)); m != "claude-opus-5-5" {
		t.Fatalf("string field: %q", m)
	}
	if m := Model(AgentClaude, []byte(`{"hook_event_name":"Stop","model":{"id":"claude-sonnet-5-5","display_name":"Sonnet"}}`)); m != "claude-sonnet-5-5" {
		t.Fatalf("object field: %q", m)
	}
	if m := Model(AgentClaude, []byte(`{"hook_event_name":"PreToolUse","model":"x"}`)); m != "" {
		t.Fatalf("PreToolUse should not report: %q", m)
	}
	if m := Model(AgentCodex, []byte(`{"model":"x"}`)); m != "" {
		t.Fatalf("codex: %q", m)
	}
}

func TestModelFromTranscript(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	var b strings.Builder
	b.WriteString(`{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-5-5"}}` + "\n")
	b.WriteString(strings.Repeat(`{"type":"user","message":{"role":"user","content":"`+strings.Repeat("x", 1000)+`"}}`+"\n", 400))
	b.WriteString(`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-5-5"}}` + "\n")
	b.WriteString(`{"type":"assistant","message":{"role":"assistant","model":"<synthetic>"}}` + "\n")
	b.WriteString(`{"type":"user","message":{"role":"user","content":"the model said \"model\":\"fake\""}}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := `{"hook_event_name":"UserPromptSubmit","transcript_path":"` + path + `"}`
	if m := Model(AgentClaude, []byte(payload)); m != "claude-opus-5-5" {
		t.Fatalf("transcript: %q", m)
	}
	// Only the tail is read: the first model is out of reach.
	os.WriteFile(path, []byte(b.String()[:strings.LastIndex(b.String(), `{"type":"assistant","message":{"role":"assistant","model":"claude-opus`)]), 0o600)
	if m := transcriptModel(path); m != "" {
		t.Fatalf("read past the tail: %q", m)
	}
	if m := transcriptModel(filepath.Join(dir, "notes.txt")); m != "" {
		t.Fatalf("non-jsonl: %q", m)
	}
	if m := transcriptModel("rel/s.jsonl"); m != "" {
		t.Fatalf("relative: %q", m)
	}
}
