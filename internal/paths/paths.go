// Package paths resolves the on-disk locations used by agenterm.
//
// By default runtime state (socket, pid file, logs) lives in ~/.agenterm and
// configuration (profiles, projects, settings) in ~/.config/agenterm. Setting
// AGENTERM_HOME overrides both with a single directory, which is what tests use.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvHome overrides both the runtime and config directories.
const EnvHome = "AGENTERM_HOME"

// Paths groups the directories agenterm reads and writes.
type Paths struct {
	RuntimeDir string
	ConfigDir  string
}

// Default resolves Paths from the environment.
func Default() (Paths, error) {
	if h := os.Getenv(EnvHome); h != "" {
		abs, err := filepath.Abs(h)
		if err != nil {
			return Paths{}, err
		}
		return Paths{RuntimeDir: abs, ConfigDir: abs}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}
	return Paths{
		RuntimeDir: filepath.Join(home, ".agenterm"),
		ConfigDir:  filepath.Join(home, ".config", "agenterm"),
	}, nil
}

// In returns Paths rooted at a single directory.
func In(dir string) Paths { return Paths{RuntimeDir: dir, ConfigDir: dir} }

// Ensure creates the directories with private permissions.
func (p Paths) Ensure() error {
	for _, d := range []string{p.RuntimeDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (p Paths) Socket() string  { return filepath.Join(p.RuntimeDir, "d.sock") }
func (p Paths) PidFile() string { return filepath.Join(p.RuntimeDir, "d.pid") }
func (p Paths) LogFile() string { return filepath.Join(p.RuntimeDir, "daemon.log") }

// RestartFile lists the sessions a daemon restart (the app's "Restart now")
// suspended: the next daemon restores them whatever sessions.restore says
// and leaves resuming them to the app. Written by the app, consumed (and
// removed) by the daemon at startup.
func (p Paths) RestartFile() string { return filepath.Join(p.RuntimeDir, "restart.json") }
func (p Paths) Profiles() string    { return filepath.Join(p.ConfigDir, "profiles.yaml") }
func (p Paths) Projects() string    { return filepath.Join(p.ConfigDir, "projects.json") }
func (p Paths) Settings() string    { return filepath.Join(p.ConfigDir, "settings.yaml") }

// Trust records the worktree setup commands the user allowed, per repository.
func (p Paths) Trust() string { return filepath.Join(p.ConfigDir, "trust.json") }

// HooksDir holds generated, agenterm-only agent integration files (Claude
// settings, opencode plugin…). It is never inside a project.
func (p Paths) HooksDir() string { return filepath.Join(p.RuntimeDir, "hooks") }

// WorktreesDir is where per-session git worktrees are created by default
// (settings git.worktree_dir overrides it). It is never inside a project.
func (p Paths) WorktreesDir() string { return filepath.Join(p.RuntimeDir, "worktrees") }

// Worktrees is the registry of agenterm-managed worktrees.
func (p Paths) Worktrees() string { return filepath.Join(p.RuntimeDir, "worktrees.json") }

// Sessions is the registry of sessions to restore after a daemon restart.
func (p Paths) Sessions() string { return filepath.Join(p.RuntimeDir, "sessions.json") }

// Messages holds the messages between sessions (agenterm msg).
func (p Paths) Messages() string { return filepath.Join(p.RuntimeDir, "messages.json") }

// ScrollbackDir holds each persisted session's scrollback (<id>.log).
func (p Paths) ScrollbackDir() string { return filepath.Join(p.RuntimeDir, "scrollback") }
