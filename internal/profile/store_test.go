package profile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

const handWritten = `# my profiles
- id: mine
  cmd: /bin/cat
  args: ["-u"]
  env: {K: v}
  attention_patterns: ['\(y/n\)']
- id: claude
  args: ["--verbose"]
- id: wrapper
  cmd: $HOME/bin/wrap
  status: hooks
`

func newStore(t *testing.T, content string) (*Store, string) {
	t.Helper()
	_, look := fakePath(t, "claude", "codex")
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &Store{Path: path, Look: look}, path
}

func TestStoreRoundTripPreservesCustom(t *testing.T) {
	s, path := newStore(t, handWritten)
	before, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	// Hide gemini: everything else stays as it was.
	after, err := s.Save(Entry{ID: "gemini", Cmd: "gemini", Enabled: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids(before), ids(after)) {
		t.Fatalf("order changed:\n%v\n%v", ids(before), ids(after))
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil || string(bak) != handWritten {
		t.Fatalf(".bak = %q, %v", bak, err)
	}
	for _, id := range []string{"mine", "claude", "wrapper"} {
		b, a := mustFind(t, before, id), mustFind(t, after, id)
		if !slices.Equal(b.Args, a.Args) || len(b.Env) != len(a.Env) || b.Cmd != a.Cmd || b.Status != a.Status ||
			!slices.Equal(b.AttentionPatterns, a.AttentionPatterns) || b.Source != a.Source {
			t.Errorf("%s changed:\n%+v\n%+v", id, b, a)
		}
	}
	if g := mustFind(t, after, "gemini"); g.Enabled || g.Source != SourceOverride {
		t.Fatalf("gemini = %+v", g)
	}
	// What is on disk is canonical, minimal and parses to the same thing.
	data, _ := os.ReadFile(path)
	text := string(data)
	if strings.Contains(text, "cmd: gemini") || !strings.Contains(text, "enabled: false") ||
		!strings.Contains(text, "$HOME/bin/wrap") || !strings.Contains(text, "- id: opencode") {
		t.Fatalf("file:\n%s", text)
	}
	reread, err := s.List()
	if err != nil || !slices.Equal(ids(reread), ids(after)) {
		t.Fatalf("reread %v %v", ids(reread), err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", fi.Mode())
	}
	// Re-enabling with catalog values leaves a bare stub (source catalog).
	ps, err := s.Save(Entry{ID: "gemini", Name: "Gemini CLI", Cmd: "gemini", Args: ptr([]string{}), Status: "heuristic",
		SetupHooks: "none", Enabled: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if g := mustFind(t, ps, "gemini"); !g.Enabled || g.Source != SourceCatalog {
		t.Fatalf("gemini = %+v", g)
	}
	es, _ := ReadEntries(path)
	if i := slices.IndexFunc(es, func(e Entry) bool { return e.ID == "gemini" }); i < 0 || !es[i].isStub() {
		t.Fatalf("entries %+v", es)
	}
}

func TestStoreNewFileAndVariants(t *testing.T) {
	s, path := newStore(t, "")
	ps, err := s.Save(Entry{ID: "claude-yolo", Extends: "claude", Cmd: "claude", Args: ptr([]string{"--dangerously-skip-permissions"}),
		Status: "hooks", SetupHooks: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("unexpected .bak for a new file: %v", err)
	}
	if got := ids(ps)[:2]; !slices.Equal(got, []string{"claude", "claude-yolo"}) {
		t.Fatalf("variant should follow its parent: %v", ids(ps))
	}
	es, _ := ReadEntries(path)
	y := es[1]
	if y.ID != "claude-yolo" || y.Cmd != "" || y.Status != "" || y.SetupHooks != "" || y.Extends != "claude" || y.Args == nil {
		t.Fatalf("variant entry not minimal: %+v", y)
	}
	// Parent can't be deleted while extended... unless it is built in
	// (deleting a built-in only resets it).
	if _, err := s.Save(Entry{ID: "mine", Cmd: "/bin/cat"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Entry{ID: "mine-2", Extends: "mine", Cmd: "/bin/cat", Args: ptr([]string{"-n"})}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete("mine"); err == nil || !strings.Contains(err.Error(), "mine-2") {
		t.Fatalf("delete extended parent: %v", err)
	}
	if _, err := s.Delete("mine-2"); err != nil {
		t.Fatal(err)
	}
	ps, err = s.Delete("mine")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := Find(ps, "mine"); ok {
		t.Fatal("mine still listed")
	}
	if _, err := s.Delete("claude"); err == nil {
		t.Fatal("deleting an unchanged built-in should fail")
	}
	if _, err := s.Delete("nope"); err == nil {
		t.Fatal("deleting unknown should fail")
	}
	// Reset an override.
	if _, err := s.Save(Entry{ID: "codex", Cmd: "codex", Args: ptr([]string{"--full-auto"})}); err != nil {
		t.Fatal(err)
	}
	ps, err = s.Delete("codex")
	if err != nil {
		t.Fatal(err)
	}
	if c := mustFind(t, ps, "codex"); c.Source != SourceCatalog || len(c.Args) != 0 || c.SetupHooks != "codex" {
		t.Fatalf("codex = %+v", c)
	}
}

func TestStoreReorder(t *testing.T) {
	s, _ := newStore(t, handWritten)
	ps, err := s.Reorder([]string{"shell", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(ps)[:5]; !slices.Equal(got, []string{"shell", "codex", "mine", "claude", "wrapper"}) {
		t.Fatalf("order %v", ids(ps))
	}
	if again, _ := s.List(); !slices.Equal(ids(again), ids(ps)) {
		t.Fatalf("order not persisted: %v", ids(again))
	}
	if _, err := s.Reorder([]string{"nope"}); err == nil {
		t.Fatal("unknown id should fail")
	}
	if _, err := s.Reorder([]string{"shell", "shell"}); err == nil {
		t.Fatal("duplicate id should fail")
	}
}

func TestStoreValidation(t *testing.T) {
	s, path := newStore(t, handWritten)
	bad := []Entry{
		{ID: "", Cmd: "x"},
		{ID: "has space", Cmd: "x"},
		{ID: "nocmd"},
		{ID: "x", Cmd: "x", AttentionPatterns: ptr([]string{"("})},
		{ID: "x", Cmd: "x", Status: "magic"},
		{ID: "x", Cmd: "x", SetupHooks: "bogus"},
		{ID: "x", Extends: "nope"},
		{ID: "x", Extends: "x"},
		{ID: "claude", Extends: "codex"},
	}
	for _, e := range bad {
		if _, err := s.Save(e); err == nil {
			t.Errorf("expected error for %+v", e)
		}
	}
	if data, _ := os.ReadFile(path); string(data) != handWritten {
		t.Fatalf("failed saves touched the file:\n%s", data)
	}
	// A broken file is reported, not overwritten.
	os.WriteFile(path, []byte("- id: a\n- id: a\n"), 0o600)
	if _, err := s.Save(Entry{ID: "y", Cmd: "y"}); err == nil {
		t.Fatal("save over a broken file should fail")
	}
}

func TestResumeArgs(t *testing.T) {
	ps := Resolve(nil, nil)
	if p, _ := Find(ps, "opencode"); !slices.Equal(p.ResumeArgs, []string{"--continue"}) {
		t.Fatalf("opencode default resume_args %q", p.ResumeArgs)
	}
	if p, _ := Find(ps, "cursor-agent"); !slices.Equal(p.ResumeArgs, []string{"--continue"}) {
		t.Fatalf("cursor-agent default resume_args %q", p.ResumeArgs)
	}
	if p, _ := Find(ps, "hermes"); !slices.Equal(p.Args, []string{"--cli"}) || !slices.Equal(p.ResumeArgs, []string{"-c"}) || !p.Enabled {
		t.Fatalf("hermes = args %q resume %q enabled %v", p.Args, p.ResumeArgs, p.Enabled)
	}
	if p, _ := Find(ps, "claude"); len(p.ResumeArgs) != 0 {
		t.Fatalf("claude resume_args %q (resumes by session id)", p.ResumeArgs)
	}
	s, path := newStore(t, "- id: mine\n  cmd: /bin/cat\n  resume_args: [\"--continue\"]\n- id: v\n  extends: mine\n")
	ps, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := Find(ps, "v"); !slices.Equal(p.ResumeArgs, []string{"--continue"}) {
		t.Fatalf("variant inherits resume_args: %q", p.ResumeArgs)
	}
	// An editor that does not know resume_args keeps it when saving.
	if _, err := s.Save(Entry{ID: "mine", Cmd: "/bin/cat", Args: ptr([]string{"-u"})}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "resume_args:") {
		t.Fatalf("resume_args lost:\n%s", b)
	}
}
