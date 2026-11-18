package hooks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrompts(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"first prompt"},"origin":{"kind":"human"}}`,
		`{"type":"assistant","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"ok"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"x"}]}}`,
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"subagent task"}}`,
		`{"type":"user","isMeta":true,"message":{"role":"user","content":"meta note"}}`,
		`{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name>"}}`,
		`{"type":"user","message":{"role":"user","content":"from a hook"},"origin":{"kind":"hook"}}`,
		`not json "user" "content":"x"`,
		`{"type":"user","message":{"role":"user","content":"  second\nprompt  "}}`,
		`{"type":"user","message":{"role":"user","content":"  second\nprompt  "}}`,
		`{"type":"user","message":{"role":"user","content":"` + strings.Repeat("x", maxPromptLine) + `"}}`,
		`{"type":"user","message":{"role":"user","content":"third"}}`,
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Prompts(p)
	want := []string{"first prompt", "second\nprompt", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Prompts = %q, want %q", got, want)
	}
	if Prompts("relative.jsonl") != nil || Prompts(filepath.Join(t.TempDir(), "x.txt")) != nil {
		t.Fatal("non-transcript paths must give nil")
	}
}
