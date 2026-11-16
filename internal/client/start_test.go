package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppExeForHelper(t *testing.T) {
	for in, want := range map[string]string{
		"/Applications/agenterm.app/Contents/Helpers/agenterm":     "/Applications/agenterm.app/Contents/MacOS/agenterm",
		"/x/My Apps/Dev.app/Contents/Helpers/agenterm":             "/x/My Apps/Dev.app/Contents/MacOS/agenterm",
		"/a.app/b/c.app/Contents/Helpers/agenterm":                 "/a.app/b/c.app/Contents/MacOS/agenterm",
		"/Applications/agenterm.app/Contents/MacOS/agenterm":       "",
		"/Applications/agenterm.app/Contents/Helpers/sub/agenterm": "",
		"/Applications/agenterm.app/Contents/Helpers/":             "",
		"/opt/homebrew/bin/agenterm":                               "",
		"/Users/me/go/bin/agenterm":                                "",
	} {
		if got := appExeForHelper(in); got != want {
			t.Errorf("appExeForHelper(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDaemonExecutable(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(p string, mode os.FileMode) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	helper := write(filepath.Join(dir, "agenterm.app/Contents/Helpers/agenterm"), 0o755)
	appExe := write(filepath.Join(dir, "agenterm.app/Contents/MacOS/agenterm"), 0o755)

	// The bundled CLI, directly and through a symlink chain (Homebrew's
	// PATH link -> Caskroom link -> Helpers/agenterm).
	link1 := filepath.Join(dir, "Caskroom", "agenterm")
	link2 := filepath.Join(dir, "bin", "agenterm")
	os.MkdirAll(filepath.Dir(link1), 0o755)
	os.MkdirAll(filepath.Dir(link2), 0o755)
	if err := os.Symlink(helper, link1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatal(err)
	}
	for _, exe := range []string{helper, link1, link2} {
		if got := DaemonExecutable(exe); got != appExe {
			t.Errorf("DaemonExecutable(%q) = %q, want %q", exe, got, appExe)
		}
	}

	// Outside a bundle (and for the app's own binary): unchanged.
	plain := write(filepath.Join(dir, "go/bin/agenterm"), 0o755)
	for _, exe := range []string{plain, appExe, filepath.Join(dir, "missing")} {
		if got := DaemonExecutable(exe); got != exe {
			t.Errorf("DaemonExecutable(%q) = %q, want unchanged", exe, got)
		}
	}

	// A bundle whose app binary is missing or not executable: unchanged.
	lone := write(filepath.Join(dir, "Lone.app/Contents/Helpers/agenterm"), 0o755)
	if got := DaemonExecutable(lone); got != lone {
		t.Errorf("no app binary: got %q", got)
	}
	noexec := write(filepath.Join(dir, "NoExec.app/Contents/Helpers/agenterm"), 0o755)
	write(filepath.Join(dir, "NoExec.app/Contents/MacOS/agenterm"), 0o644)
	if got := DaemonExecutable(noexec); got != noexec {
		t.Errorf("non-executable app binary: got %q", got)
	}
}
