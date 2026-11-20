package profile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/status"
)

// fakePath creates a directory with fake executables (and one
// non-executable file) and returns a Lookup over it.
func fakePath(t *testing.T, names ...string) (string, Lookup) {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &Detector{SearchPath: func() string { return dir }}
	return dir, d.Lookup
}

func mustFind(t *testing.T, ps []Profile, id string) Profile {
	t.Helper()
	p, ok := Find(ps, id)
	if !ok {
		t.Fatalf("profile %q missing from %v", id, ids(ps))
	}
	return p
}

func TestDefaultsAndDetection(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir, look := fakePath(t, "claude", "gemini")
	ps, err := Load(filepath.Join(t.TempDir(), "nope.yaml"), look)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(ps), ids(Resolve(nil, nil)); !slices.Equal(got, want) || len(got) != len(catalog) {
		t.Fatalf("ids %v", got)
	}
	cl := mustFind(t, ps, "claude")
	if cl.Status != status.ModeHooks || cl.SetupHooks != "claude" || !cl.Installed || cl.Path != filepath.Join(dir, "claude") ||
		cl.Source != SourceCatalog || !cl.Enabled || !cl.Launchable() || cl.Name != "Claude Code" {
		t.Fatalf("claude = %+v", cl)
	}
	if cx := mustFind(t, ps, "codex"); cx.Installed || cx.Launchable() || cx.SetupHooks != "codex" || cx.Install == "" {
		t.Fatalf("codex = %+v", cx)
	}
	if g := mustFind(t, ps, "gemini"); !g.Installed {
		t.Fatalf("gemini = %+v", g)
	}
	if sh := mustFind(t, ps, "shell"); !sh.Installed || sh.Path != "/bin/sh" {
		t.Fatalf("shell = %+v", sh)
	}
	if _, ok := LookPathIn("notexec", dir); ok {
		t.Fatal("non-executable file detected")
	}
	if _, ok := LookPathIn("rel/claude", dir); ok {
		t.Fatal("relative path resolved")
	}
}

// An old profiles.yaml (full list, no setup_hooks) keeps working and now
// inherits the catalog's hook integration.
func TestBackwardCompatOldYAML(t *testing.T) {
	_, look := fakePath(t, "claude", "codex")
	es, err := Parse([]byte(`
- id: claude
  cmd: claude
  args: []
  env: {}
  status: hooks
- id: codex
  cmd: codex
- id: mine
  cmd: /bin/cat
- id: shell
  cmd: $SHELL
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := Resolve(es, look)
	if got := ids(ps)[:5]; !slices.Equal(got, []string{"claude", "codex", "mine", "shell", "opencode"}) {
		t.Fatalf("order %v", ids(ps))
	}
	cl := mustFind(t, ps, "claude")
	if cl.SetupHooks != "claude" || cl.Status != status.ModeHooks || cl.Source != SourceOverride {
		t.Fatalf("claude = %+v", cl)
	}
	cx := mustFind(t, ps, "codex")
	if cx.SetupHooks != "codex" || cx.Status != status.ModeHeuristic {
		t.Fatalf("codex = %+v", cx)
	}
	if m := mustFind(t, ps, "mine"); m.Source != SourceCustom || !m.Installed || m.Status != status.ModeHeuristic || m.Name != "mine" {
		t.Fatalf("mine = %+v", m)
	}
}

func TestOverridePrecedence(t *testing.T) {
	es, err := Parse([]byte(`
- id: claude
  name: Claude (trabajo)
  cmd: /opt/claude
  args: ["--verbose"]
  env: {A: b}
  setup_hooks: none
  idle_seconds: 2
- id: goose
  args: []
- id: aider
  attention_patterns: ['custom\?']
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := Resolve(es, nil)
	cl := mustFind(t, ps, "claude")
	if cl.Name != "Claude (trabajo)" || cl.Cmd != "/opt/claude" || !slices.Equal(cl.Args, []string{"--verbose"}) ||
		cl.Env["A"] != "b" || cl.SetupHooks != "" || cl.IdleSeconds != 2 || cl.Status != status.ModeHooks {
		t.Fatalf("claude = %+v", cl)
	}
	if g := mustFind(t, ps, "goose"); len(g.Args) != 0 || g.Cmd != "goose" {
		t.Fatalf("goose = %+v (explicit [] must clear catalog args)", g)
	}
	if a := mustFind(t, ps, "aider"); !slices.Equal(a.AttentionPatterns, []string{`custom\?`}) {
		t.Fatalf("aider = %+v", a)
	}
	// The catalog itself is untouched.
	if g := mustFind(t, Resolve(nil, nil), "goose"); !slices.Equal(g.Args, []string{"session"}) {
		t.Fatalf("catalog goose = %+v", g)
	}
}

func TestExtends(t *testing.T) {
	_, look := fakePath(t, "claude")
	es, err := Parse([]byte(`
- id: claude
  env: {X: "1"}
- id: claude-yolo
  extends: claude
  args: ["--dangerously-skip-permissions"]
- id: claude-yolo-env
  extends: claude-yolo
  name: Yolo con env
  env: {Y: "2"}
- id: orphan
  extends: nope
- id: loop-a
  extends: loop-b
- id: loop-b
  extends: loop-a
- id: codex
  extends: claude
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := Resolve(es, look)
	y := mustFind(t, ps, "claude-yolo")
	if y.Source != SourceCustom || y.Extends != "claude" || y.Cmd != "claude" || y.SetupHooks != "claude" ||
		y.Status != status.ModeHooks || y.Env["X"] != "1" || !y.Installed || y.Error != "" || y.Name != "claude-yolo" ||
		!slices.Equal(y.Args, []string{"--dangerously-skip-permissions"}) || y.Homepage == "" {
		t.Fatalf("claude-yolo = %+v", y)
	}
	ye := mustFind(t, ps, "claude-yolo-env")
	if ye.Name != "Yolo con env" || len(ye.Args) != 1 || ye.Env["Y"] != "2" || ye.Env["X"] != "" {
		t.Fatalf("claude-yolo-env = %+v", ye)
	}
	// The parent's env map was not mutated by the child.
	if cl := mustFind(t, ps, "claude"); len(cl.Env) != 1 || len(cl.Args) != 0 {
		t.Fatalf("claude = %+v", cl)
	}
	for _, id := range []string{"orphan", "loop-a", "loop-b", "codex"} {
		if p := mustFind(t, ps, id); p.Error == "" || p.Launchable() {
			t.Errorf("%s should be an error: %+v", id, p)
		}
	}
}

func TestEnabledAndOrder(t *testing.T) {
	_, look := fakePath(t, "claude", "codex", "gemini")
	es, err := Parse([]byte(`
- id: claude
  enabled: false
- id: gemini
  order: -1
- id: zed
  cmd: /bin/cat
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := Resolve(es, look)
	if got := ids(ps)[:4]; !slices.Equal(got, []string{"gemini", "claude", "zed", "codex"}) {
		t.Fatalf("order %v", ids(ps))
	}
	cl := mustFind(t, ps, "claude")
	if cl.Enabled || cl.Launchable() || !cl.Installed {
		t.Fatalf("claude = %+v", cl)
	}
	var launch []string
	for _, p := range ps {
		if p.Launchable() {
			launch = append(launch, p.ID)
		}
	}
	if !slices.Equal(launch, []string{"gemini", "zed", "codex", "shell"}) {
		t.Fatalf("launchable %v", launch)
	}
}

func TestParseAndExpand(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("AGT_BIN", "/opt/bin")
	es, err := Parse([]byte(`
- id: claude
  cmd: claude
  args: ["--verbose"]
  env: {FOO: "bar-$AGT_BIN"}
- id: mine
  cmd: $AGT_BIN/agent
- id: shell
  idle_seconds: 1.5
`))
	if err != nil {
		t.Fatal(err)
	}
	ps := Resolve(es, nil)
	mine := mustFind(t, ps, "mine")
	if mine.Command() != "/opt/bin/agent" || mine.Status != status.ModeHeuristic {
		t.Fatalf("mine = %+v cmd=%q", mine, mine.Command())
	}
	sh := mustFind(t, ps, "shell")
	if sh.Command() != "/bin/zsh" || sh.IdleSeconds != 1.5 {
		t.Fatalf("shell = %+v", sh)
	}
	cl := mustFind(t, ps, "claude")
	if len(cl.Args) != 1 || cl.Environ()[0] != "FOO=bar-/opt/bin" {
		t.Fatalf("claude = %+v env=%v", cl, cl.Environ())
	}
}

func TestShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if sh := mustFind(t, Resolve(nil, nil), "shell"); sh.Command() != "/bin/sh" {
		t.Fatalf("got %q", sh.Command())
	}
}

func TestParseErrors(t *testing.T) {
	hard := []string{
		"- cmd: x\n",
		"- id: a\n  cmd: x\n- id: a\n  cmd: y\n",
		"not: a list\n",
	}
	for _, b := range hard {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
	// Semantic problems only break the profile concerned.
	soft := map[string]string{
		"- id: a\n":                                        "missing cmd",
		"- id: a\n  cmd: x\n  status: magic\n":             "mode",
		"- id: a\n  cmd: x\n  attention_patterns: ['(']\n": "attention_patterns",
		"- id: a\n  cmd: x\n  setup_hooks: bogus\n":        "setup_hooks",
		"- id: claude\n  status: magic\n":                  "mode",
	}
	for b, want := range soft {
		es, err := Parse([]byte(b))
		if err != nil {
			t.Fatalf("%q: %v", b, err)
		}
		ps := Resolve(es, nil)
		p := ps[0]
		if !strings.Contains(p.Error, want) {
			t.Errorf("%q: error %q, want %q", b, p.Error, want)
		}
		if sh := mustFind(t, ps, "shell"); sh.Error != "" {
			t.Errorf("%q broke shell: %v", b, sh.Error)
		}
	}
}

func TestAttentionPatterns(t *testing.T) {
	es, _ := Parse([]byte("- id: x\n  cmd: x\n  attention_patterns: ['\\(y/n\\)', 'Do you want to proceed\\?']\n"))
	res, err := Resolve(es, nil)[0].Patterns()
	if err != nil || len(res) != 2 || !res[0].MatchString("Continue? (y/n)") {
		t.Fatalf("patterns %v %v", res, err)
	}
	a := mustFind(t, Resolve(nil, nil), "aider")
	if res, err := a.Patterns(); err != nil || !res[0].MatchString("Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:") {
		t.Fatalf("aider patterns %v %v", res, err)
	}
	h := mustFind(t, Resolve(nil, nil), "hermes")
	if res, err := h.Patterns(); err != nil || !res[0].MatchString("      [o]nce  |  [s]ession  |  [a]lways  |  [d]eny") {
		t.Fatalf("hermes patterns %v %v", res, err)
	}
	// Cursor CLI's approval menu (from a real session).
	menu := " $  panal msg read in .\n\n Run this command?\n Not in allowlist: panal\n  → Run (once) (y)\n" +
		"    Add Shell(panal) to allowlist? (tab)\n    Run Everything (shift+tab)\n    Skip & tell the agent what to do instead (esc or n)\n"
	c := mustFind(t, Resolve(nil, nil), "cursor-agent")
	res, err = c.Patterns()
	if err != nil || len(res) == 0 {
		t.Fatalf("cursor patterns %v %v", res, err)
	}
	for _, re := range res {
		if !re.MatchString(menu) {
			t.Errorf("cursor pattern %q misses its approval menu", re)
		}
		if re.MatchString("Ran panal msg read: no new messages\n> ") {
			t.Errorf("cursor pattern %q matches plain output", re)
		}
	}
}

func TestCatalogSanity(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Catalog() {
		if seen[c.ID] || c.Name == "" || (c.Cmd == "" && len(c.Binaries) == 0) {
			t.Errorf("bad catalog entry %+v", c)
		}
		seen[c.ID] = true
		if !idRe.MatchString(c.ID) {
			t.Errorf("bad id %q", c.ID)
		}
	}
	for _, p := range Resolve(nil, nil) {
		if p.Error != "" {
			t.Errorf("%s: %s", p.ID, p.Error)
		}
	}
	if len(seen) < 12 || !seen["shell"] {
		t.Fatalf("catalog %v", seen)
	}
}

func TestDetectorCache(t *testing.T) {
	calls := 0
	d := &Detector{SearchPath: func() string { calls++; return "/nonexistent" }}
	d.Lookup("x")
	d.Lookup("y")
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
	d.Rescan()
	d.Lookup("x")
	if calls != 2 {
		t.Fatalf("calls after rescan = %d", calls)
	}
}

func TestInstallCommand(t *testing.T) {
	for hint, want := range map[string][2]string{
		"npm i -g @openai/codex   (or brew install --cask codex)":                                  {"npm i -g @openai/codex", "brew install --cask codex"},
		"brew install block-goose-cli":                                                             {"brew install block-goose-cli", ""},
		"curl -fsSL https://claude.ai/install.sh | bash   (or npm i -g @anthropic-ai/claude-code)": {"curl -fsSL https://claude.ai/install.sh | bash", "npm i -g @anthropic-ai/claude-code"},
		"": {"", ""},
	} {
		if c, a := InstallCommand(hint); c != want[0] || a != want[1] {
			t.Errorf("InstallCommand(%q) = %q, %q", hint, c, a)
		}
	}
	if n, c := CatalogInstall("codex"); n == "" || c != "npm i -g @openai/codex" {
		t.Errorf("CatalogInstall(codex) = %q, %q", n, c)
	}
	if _, c := CatalogInstall("shell"); c != "" {
		t.Errorf("shell has no install: %q", c)
	}
	if _, c := CatalogInstall("my-own"); c != "" {
		t.Errorf("unknown id: %q", c)
	}
}
