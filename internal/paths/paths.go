// Package paths resolves the on-disk locations used by panal.
//
// By default runtime state (socket, pid file, logs) lives in ~/.panal and
// configuration (profiles, projects, settings) in ~/.config/panal. Setting
// PANAL_HOME overrides both with a single directory, which is what tests use.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvHome overrides both the runtime and config directories.
const EnvHome = "PANAL_HOME"

// Paths groups the directories panal reads and writes.
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
	return inHome(home), nil
}

// inHome is the default layout under the user's home directory: the
// panal directories, or, for each one that does not exist yet while its
// pre-rename agenterm directory does (MigrateLegacy has not run), the
// agenterm one, so clients and an old daemon still agree.
func inHome(home string) Paths {
	p := Paths{
		RuntimeDir: filepath.Join(home, ".panal"),
		ConfigDir:  filepath.Join(home, ".config", "panal"),
	}
	l := legacyInHome(home)
	if !exists(p.RuntimeDir) && isDir(l.RuntimeDir) {
		p.RuntimeDir = l.RuntimeDir
	}
	if !exists(p.ConfigDir) && isDir(l.ConfigDir) {
		p.ConfigDir = l.ConfigDir
	}
	return p
}

// legacyInHome is the default layout from before the rename to panal.
func legacyInHome(home string) Paths {
	return Paths{
		RuntimeDir: filepath.Join(home, ".agenterm"),
		ConfigDir:  filepath.Join(home, ".config", "agenterm"),
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// isDir reports whether p is a real directory (not a symlink to one).
func isDir(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.IsDir()
}

// MigrateLegacy moves the pre-rename directories (~/.agenterm,
// ~/.config/agenterm) to their panal names and leaves a symlink at each
// old path, so paths recorded before the move (session folders under
// ~/.agenterm/worktrees, git's worktree links, an old app's settings) keep
// resolving. It does nothing with PANAL_HOME set, for a directory whose
// panal name already exists, or while a daemon still listens on the old
// socket (alive reports that; that daemon would lose its files). It
// returns the directories it moved, as "old -> new".
func MigrateLegacy(alive func(sock string) bool) ([]string, error) {
	if os.Getenv(EnvHome) != "" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return migrateLegacy(home, alive)
}

func migrateLegacy(home string, alive func(sock string) bool) ([]string, error) {
	old := legacyInHome(home)
	if isDir(old.RuntimeDir) && alive != nil && alive(old.Socket()) {
		return nil, nil
	}
	cur := Paths{
		RuntimeDir: filepath.Join(home, ".panal"),
		ConfigDir:  filepath.Join(home, ".config", "panal"),
	}
	var moved []string
	for _, d := range [][2]string{{old.RuntimeDir, cur.RuntimeDir}, {old.ConfigDir, cur.ConfigDir}} {
		from, to := d[0], d[1]
		if !isDir(from) || exists(to) {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return moved, fmt.Errorf("move %s to %s: %w", from, to, err)
		}
		if err := os.Symlink(to, from); err != nil {
			return moved, fmt.Errorf("link %s to %s: %w", from, to, err)
		}
		moved = append(moved, from+" -> "+to)
	}
	return moved, nil
}

// LegacyEnvPrefix is the prefix of the environment variables from before
// the rename to panal (AGENTERM_HOME…).
const LegacyEnvPrefix = "AGENTERM_"

// ApplyLegacyEnv sets PANAL_X from AGENTERM_X for every pre-rename
// variable whose panal one is unset, so a session started by an old daemon,
// a test script or a shell profile keeps working. Call it first thing in
// main.
func ApplyLegacyEnv() {
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, LegacyEnvPrefix) {
			continue
		}
		n := "PANAL_" + strings.TrimPrefix(k, LegacyEnvPrefix)
		if _, set := os.LookupEnv(n); !set {
			os.Setenv(n, v)
		}
	}
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

// HooksDir holds generated, panal-only agent integration files (Claude
// settings, opencode plugin…). It is never inside a project.
func (p Paths) HooksDir() string { return filepath.Join(p.RuntimeDir, "hooks") }

// WorktreesDir is where per-session git worktrees are created by default
// (settings git.worktree_dir overrides it). It is never inside a project.
func (p Paths) WorktreesDir() string { return filepath.Join(p.RuntimeDir, "worktrees") }

// Worktrees is the registry of panal-managed worktrees.
func (p Paths) Worktrees() string { return filepath.Join(p.RuntimeDir, "worktrees.json") }

// Sessions is the registry of sessions to restore after a daemon restart.
func (p Paths) Sessions() string { return filepath.Join(p.RuntimeDir, "sessions.json") }

// Messages holds the messages between sessions (panal msg).
func (p Paths) Messages() string { return filepath.Join(p.RuntimeDir, "messages.json") }

// ScrollbackDir holds each persisted session's scrollback (<id>.log).
func (p Paths) ScrollbackDir() string { return filepath.Join(p.RuntimeDir, "scrollback") }

// HistoryDir holds the archive of ended agent sessions (internal/history).
func (p Paths) HistoryDir() string { return filepath.Join(p.RuntimeDir, "history") }
