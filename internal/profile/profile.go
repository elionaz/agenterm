// Package profile loads agent launch profiles from YAML.
package profile

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/status"
)

// Profile describes how to launch an agent.
type Profile struct {
	ID         string            `yaml:"id" json:"id"`
	Cmd        string            `yaml:"cmd" json:"cmd"`
	Args       []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Status     status.Mode       `yaml:"status,omitempty" json:"status"`
	SetupHooks string            `yaml:"setup_hooks,omitempty" json:"setupHooks,omitempty"`
	// IdleSeconds overrides the daemon's heuristic idle timeout (optional).
	IdleSeconds float64 `yaml:"idle_seconds,omitempty" json:"idleSeconds,omitempty"`
}

// Defaults returns the built-in profiles used when no profiles.yaml exists.
func Defaults() []Profile {
	return []Profile{
		{ID: "claude", Cmd: "claude", Args: []string{}, Env: map[string]string{}, Status: status.ModeHooks, SetupHooks: "claude"},
		{ID: "codex", Cmd: "codex", Status: status.ModeHooks},
		{ID: "opencode", Cmd: "opencode", Status: status.ModeHeuristic},
		{ID: "shell", Cmd: "$SHELL", Status: status.ModeHeuristic},
	}
}

// Load reads profiles from path. A missing file yields Defaults().
func Load(path string) ([]Profile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes and validates a YAML list of profiles.
func Parse(data []byte) ([]Profile, error) {
	var ps []Profile
	if err := yaml.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	seen := make(map[string]bool, len(ps))
	for i := range ps {
		p := &ps[i]
		if p.ID == "" {
			return nil, fmt.Errorf("profile #%d: missing id", i+1)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("profile %q: duplicate id", p.ID)
		}
		seen[p.ID] = true
		if p.Cmd == "" {
			return nil, fmt.Errorf("profile %q: missing cmd", p.ID)
		}
		mode, err := status.ParseMode(string(p.Status))
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", p.ID, err)
		}
		p.Status = mode
	}
	return ps, nil
}

// Find returns the profile with id.
func Find(ps []Profile, id string) (Profile, bool) {
	for _, p := range ps {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// Command returns the executable with $VARS expanded. An empty expansion of
// $SHELL falls back to /bin/sh.
func (p Profile) Command() string {
	c := os.ExpandEnv(p.Cmd)
	if c == "" && p.Cmd == "$SHELL" {
		return "/bin/sh"
	}
	return c
}

// Environ returns Env as KEY=VALUE pairs, with values $VAR-expanded.
func (p Profile) Environ() []string {
	out := make([]string, 0, len(p.Env))
	for k, v := range p.Env {
		out = append(out, k+"="+os.ExpandEnv(v))
	}
	return out
}
