package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestInHomeFresh(t *testing.T) {
	home := t.TempDir()
	p := inHome(home)
	if p.RuntimeDir != filepath.Join(home, ".panal") || p.ConfigDir != filepath.Join(home, ".config", "panal") {
		t.Fatalf("got %+v", p)
	}
}

func TestInHomeLegacyBeforeMigration(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, ".agenterm"))
	mkdir(t, filepath.Join(home, ".config", "agenterm"))
	p := inHome(home)
	if p.RuntimeDir != filepath.Join(home, ".agenterm") || p.ConfigDir != filepath.Join(home, ".config", "agenterm") {
		t.Fatalf("got %+v, want the agenterm dirs", p)
	}
}

func TestInHomeBothPreferPanal(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, ".agenterm"))
	mkdir(t, filepath.Join(home, ".panal"))
	if p := inHome(home); p.RuntimeDir != filepath.Join(home, ".panal") {
		t.Fatalf("got %+v", p)
	}
}

func TestMigrateLegacy(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".agenterm")
	mkdir(t, filepath.Join(old, "worktrees", "proj", "s1"))
	os.WriteFile(filepath.Join(old, "sessions.json"), []byte(`{"cwd":"`+filepath.Join(old, "worktrees", "proj", "s1")+`"}`), 0o600)
	mkdir(t, filepath.Join(home, ".config", "agenterm"))
	os.WriteFile(filepath.Join(home, ".config", "agenterm", "settings.yaml"), []byte("x: 1\n"), 0o600)

	var asked string
	moved, err := migrateLegacy(home, func(sock string) bool { asked = sock; return false })
	if err != nil || len(moved) != 2 {
		t.Fatalf("moved %v, err %v", moved, err)
	}
	if asked != filepath.Join(old, "d.sock") {
		t.Fatalf("alive asked for %q", asked)
	}
	if _, err := os.Stat(filepath.Join(home, ".panal", "sessions.json")); err != nil {
		t.Fatal("sessions.json not moved:", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "panal", "settings.yaml")); err != nil {
		t.Fatal("settings not moved:", err)
	}
	// Old paths, like a session's worktree folder, still resolve.
	if fi, err := os.Stat(filepath.Join(old, "worktrees", "proj", "s1")); err != nil || !fi.IsDir() {
		t.Fatal("old worktree path does not resolve:", err)
	}
	if p := inHome(home); p.RuntimeDir != filepath.Join(home, ".panal") || p.ConfigDir != filepath.Join(home, ".config", "panal") {
		t.Fatalf("after migration got %+v", p)
	}
	// Idempotent.
	if moved, err := migrateLegacy(home, nil); err != nil || len(moved) != 0 {
		t.Fatalf("second run moved %v, err %v", moved, err)
	}
}

func TestMigrateLegacySkipsLiveDaemon(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, ".agenterm"))
	mkdir(t, filepath.Join(home, ".config", "agenterm"))
	moved, err := migrateLegacy(home, func(string) bool { return true })
	if err != nil || len(moved) != 0 {
		t.Fatalf("moved %v, err %v", moved, err)
	}
	if !isDir(filepath.Join(home, ".agenterm")) || exists(filepath.Join(home, ".panal")) {
		t.Fatal("touched the dirs of a live daemon")
	}
}

func TestMigrateLegacyKeepsExistingPanal(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, ".agenterm"))
	mkdir(t, filepath.Join(home, ".panal"))
	moved, err := migrateLegacy(home, func(string) bool { return false })
	if err != nil || len(moved) != 0 {
		t.Fatalf("moved %v, err %v", moved, err)
	}
	if !isDir(filepath.Join(home, ".agenterm")) {
		t.Fatal("removed the old dir although panal's existed")
	}
}

func TestMigrateLegacyHomeOverride(t *testing.T) {
	t.Setenv(EnvHome, t.TempDir())
	if moved, err := MigrateLegacy(func(string) bool { return false }); err != nil || moved != nil {
		t.Fatalf("moved %v, err %v", moved, err)
	}
}

func TestApplyLegacyEnv(t *testing.T) {
	t.Setenv("AGENTERM_HOME", "/old/home")
	t.Setenv("AGENTERM_SESSION", "abc")
	t.Setenv("PANAL_SESSION", "new")
	os.Unsetenv("PANAL_HOME")
	t.Cleanup(func() { os.Unsetenv("PANAL_HOME") })
	ApplyLegacyEnv()
	if got := os.Getenv("PANAL_HOME"); got != "/old/home" {
		t.Fatalf("PANAL_HOME = %q", got)
	}
	if got := os.Getenv("PANAL_SESSION"); got != "new" {
		t.Fatalf("PANAL_SESSION overwritten: %q", got)
	}
}
