package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elionaz/agenterm/internal/status"
)

func TestLoadMissingUsesDefaults(t *testing.T) {
	ps, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]status.Mode{"claude": status.ModeHooks, "codex": status.ModeHooks, "opencode": status.ModeHeuristic, "shell": status.ModeHeuristic}
	if len(ps) != len(want) {
		t.Fatalf("got %d profiles", len(ps))
	}
	for _, p := range ps {
		if want[p.ID] != p.Status {
			t.Errorf("%s: status %q", p.ID, p.Status)
		}
	}
	c, _ := Find(ps, "claude")
	if c.SetupHooks != "claude" {
		t.Errorf("claude setup_hooks = %q", c.SetupHooks)
	}
}

func TestParseAndExpand(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("AGT_BIN", "/opt/bin")
	data := []byte(`
- id: claude
  cmd: claude
  args: ["--verbose"]
  env: {FOO: "bar-$AGT_BIN"}
  status: hooks
  setup_hooks: claude
- id: mine
  cmd: $AGT_BIN/agent
- id: shell
  cmd: $SHELL
  idle_seconds: 1.5
`)
	ps, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	mine, ok := Find(ps, "mine")
	if !ok || mine.Command() != "/opt/bin/agent" || mine.Status != status.ModeHeuristic {
		t.Fatalf("mine = %+v cmd=%q", mine, mine.Command())
	}
	sh, _ := Find(ps, "shell")
	if sh.Command() != "/bin/zsh" || sh.IdleSeconds != 1.5 {
		t.Fatalf("shell = %+v", sh)
	}
	cl, _ := Find(ps, "claude")
	if len(cl.Args) != 1 || cl.Environ()[0] != "FOO=bar-/opt/bin" {
		t.Fatalf("claude = %+v env=%v", cl, cl.Environ())
	}
}

func TestShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	sh, _ := Find(Defaults(), "shell")
	if sh.Command() != "/bin/sh" {
		t.Fatalf("got %q", sh.Command())
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"- cmd: x\n",
		"- id: a\n",
		"- id: a\n  cmd: x\n- id: a\n  cmd: y\n",
		"- id: a\n  cmd: x\n  status: magic\n",
		"not: a list\n",
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
}

func TestLoadFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(p, []byte("- id: x\n  cmd: /bin/cat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ps, err := Load(p)
	if err != nil || len(ps) != 1 || ps[0].ID != "x" {
		t.Fatalf("%+v %v", ps, err)
	}
}
