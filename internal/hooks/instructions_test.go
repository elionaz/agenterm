package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexInstructionArgs(t *testing.T) {
	if got := CodexInstructionArgs(" \n"); got != nil {
		t.Fatalf("blank: %v", got)
	}
	got := CodexInstructionArgs("Say \"hi\"\nthen <stop>")
	want := `developer_instructions="Say \"hi\"\nthen <stop>"`
	if len(got) != 2 || got[0] != "-c" || got[1] != want {
		t.Fatalf("args = %q, want -c %q", got, want)
	}
}

func TestCodexConfigSets(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		"model = \"o3\"\ndeveloper_instructions = \"mine\"\n":            true,
		"  developer_instructions=\"\"\"\nmulti\n\"\"\"\n":               true,
		"model = \"o3\"\n[profiles.x]\ndeveloper_instructions = \"p\"\n": false,
		"# developer_instructions = \"off\"\n":                           false,
		"":                                                               false,
	}
	i := 0
	for body, want := range cases {
		i++
		p := filepath.Join(dir, "c"+string(rune('0'+i))+".toml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := codexConfigSets(p); got != want {
			t.Errorf("%q: got %v, want %v", body, got, want)
		}
	}
	if codexConfigSets(filepath.Join(dir, "missing.toml")) {
		t.Error("missing file reported as set")
	}
	t.Setenv("CODEX_HOME", dir)
	if CodexOwnInstructions() {
		t.Error("no config.toml in CODEX_HOME")
	}
}
