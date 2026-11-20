package repoconfig

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	c, err := Parse("x.yaml", []byte("worktree:\n  copy: [.env, ' config/local.json ', '']\n  setup: '  npm install '\nports:\n  base: 4000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Worktree.Copy, []string{".env", "config/local.json"}) || c.Worktree.Setup != "npm install" {
		t.Fatalf("worktree = %+v", c.Worktree)
	}
	if c.Ports.Base != 4000 || !slices.Equal(c.Ports.Env, []string{"PORT"}) {
		t.Fatalf("ports = %+v", c.Ports)
	}
	if c, err := Parse("x", []byte("ports:\n  base: 5173\n  env: [VITE_PORT, PORT]\n")); err != nil || !slices.Equal(c.Ports.Env, []string{"VITE_PORT", "PORT"}) {
		t.Fatalf("ports.env = %+v, %v", c.Ports, err)
	}
	if c, err := Parse("x", nil); err != nil || c.Ports.Base != 0 || len(c.Worktree.Copy) != 0 {
		t.Fatalf("empty = %+v, %v", c, err)
	}
	for _, bad := range []string{
		"worktree:\n  copy: [/etc/passwd]\n",
		"worktree:\n  copy: [../other/.env]\n",
		"worktree:\n  copy: [a/../../x]\n",
		"worktree:\n  copy: ['~/.ssh']\n",
		"worktree:\n  copy: [.git/config]\n",
		"worktree:\n  copy: ['[']\n",
		"ports:\n  base: 80\n",
		"ports:\n  base: 3000\n  env: ['1X']\n",
		"version: 99\n",
		"worktree: [\n",
	} {
		if _, err := Parse("x.yaml", []byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	if c, err := Load(root); err != nil || c.Path != "" {
		t.Fatalf("no file = %+v, %v", c, err)
	}
	os.WriteFile(filepath.Join(root, ".panal.yml"), []byte("worktree:\n  setup: make\n"), 0o644)
	c, err := Load(root)
	if err != nil || c.Worktree.Setup != "make" || filepath.Base(c.Path) != ".panal.yml" {
		t.Fatalf("yml = %+v, %v", c, err)
	}
}

func TestLoadLegacyName(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".agenterm.yaml"), []byte("worktree:\n  setup: old\n"), 0o644)
	c, err := Load(root)
	if err != nil || c.Worktree.Setup != "old" {
		t.Fatalf("legacy = %+v, %v", c, err)
	}
	os.WriteFile(filepath.Join(root, ".panal.yaml"), []byte("worktree:\n  setup: new\n"), 0o644)
	if c, err := Load(root); err != nil || c.Worktree.Setup != "new" {
		t.Fatalf("panal file should win: %+v, %v", c, err)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCopyInto(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, src, ".env", "SECRET=1\n")
	write(t, src, ".env.local", "LOCAL=1\n")
	write(t, src, "config/local.json", "{}\n")
	write(t, src, "cache/a/b.txt", "deep\n")
	write(t, src, "tracked.txt", "main\n")
	write(t, dst, "tracked.txt", "worktree\n") // git already put it there
	outside := t.TempDir()
	write(t, outside, "secret", "x")
	ctx := context.Background()

	res, err := CopyInto(ctx, src, dst, []string{".env*", "config/local.json", "cache", "tracked.txt", "nope.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Copied, []string{".env", ".env.local", "config/local.json", "cache"}) {
		t.Fatalf("copied %v", res.Copied)
	}
	if !slices.Equal(res.Skipped, []string{"tracked.txt"}) || !slices.Equal(res.Missing, []string{"nope.txt"}) {
		t.Fatalf("skipped %v missing %v", res.Skipped, res.Missing)
	}
	for rel, want := range map[string]string{".env": "SECRET=1\n", "cache/a/b.txt": "deep\n", "tracked.txt": "worktree\n"} {
		if got, _ := os.ReadFile(filepath.Join(dst, rel)); string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	// A second run copies nothing new.
	if res, _ := CopyInto(ctx, src, dst, []string{".env"}); len(res.Copied) != 0 || !slices.Equal(res.Skipped, []string{".env"}) {
		t.Fatalf("again: %+v", res)
	}
	// A symlink that leaves the checkout is refused.
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyInto(ctx, src, t.TempDir(), []string{"link"}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("symlink out: %v", err)
	}
}

func TestTrust(t *testing.T) {
	tr := OpenTrust(filepath.Join(t.TempDir(), "trust.json"))
	if tr.Allowed("/r", "npm install") {
		t.Fatal("trusted before Allow")
	}
	if err := tr.Allow("/r", "npm install"); err != nil {
		t.Fatal(err)
	}
	if !tr.Allowed("/r", "npm install") || tr.Allowed("/r", "npm install && curl x") || tr.Allowed("/other", "npm install") {
		t.Fatal("trust is per repo and per exact command")
	}
	if err := tr.Allow("/r", "pnpm i"); err != nil {
		t.Fatal(err)
	}
	if tr.Allowed("/r", "npm install") || !tr.Allowed("/r", "pnpm i") {
		t.Fatal("a new command replaces the old one")
	}
	data, _ := os.ReadFile(tr.path)
	if strings.Contains(string(data), "pnpm") || !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("trust.json keeps commands or lacks a version:\n%s", data)
	}
}

func TestSuggestAndCreate(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	write(t, root, ".gitignore", ".env\n.env.local\n")
	write(t, root, ".env", "A=1\n")
	write(t, root, ".env.local", "B=1\n")
	write(t, root, ".env.example", "A=\n") // tracked, not ignored
	ctx := context.Background()
	if got := Suggest(ctx, root); !slices.Equal(got, []string{".env", ".env.local"}) {
		t.Fatalf("Suggest = %v", got)
	}
	if got := Suggest(ctx, t.TempDir()); got != nil {
		t.Fatalf("Suggest(empty) = %v", got)
	}
	p, err := Create(root, []string{".env", ".env.local"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil || c.Path != p || !slices.Equal(c.Worktree.Copy, []string{".env", ".env.local"}) || c.Worktree.Setup != "" || c.Ports.Base != 0 {
		t.Fatalf("created file reads as %+v, %v", c, err)
	}
	if _, err := Create(root, nil); err != ErrExists {
		t.Fatalf("Create over a file: %v", err)
	}
}
