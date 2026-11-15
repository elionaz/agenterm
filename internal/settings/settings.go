// Package settings reads the optional user settings file
// (~/.config/agenterm/settings.yaml). Every field defaults to "on".
package settings

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/atomicfile"
)

// Notify controls attention notifications.
type Notify struct {
	Enabled bool `json:"enabled"`      // native notifications at all
	Sound   bool `json:"sound"`        // sound for permission/question notifications
	OnDone  bool `json:"notifyOnDone"` // also notify when an agent finishes its turn
}

// Settings is the parsed file.
type Settings struct {
	Notify Notify `json:"notify"`
}

type fileNotify struct {
	Enabled *bool `yaml:"enabled"`
	Sound   *bool `yaml:"sound"`
	OnDone  *bool `yaml:"notify_on_done"`
}

type file struct {
	Notify fileNotify `yaml:"notify"`
}

// Save writes s to path atomically (keeping a .bak of the previous file).
// Keys other than the known ones are not preserved.
func Save(path string, s Settings) error {
	f := file{Notify: fileNotify{Enabled: &s.Notify.Enabled, Sound: &s.Notify.Sound, OnDone: &s.Notify.OnDone}}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append([]byte("# agenterm: ajustes (ver README)\n"), data...), 0o600)
}

// Defaults returns the settings used when the file (or a key) is missing.
func Defaults() Settings {
	return Settings{Notify: Notify{Enabled: true, Sound: true, OnDone: true}}
}

// Parse decodes YAML, filling missing keys with defaults.
func Parse(data []byte) (Settings, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Defaults(), fmt.Errorf("parse settings: %w", err)
	}
	s := Defaults()
	set := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	set(&s.Notify.Enabled, f.Notify.Enabled)
	set(&s.Notify.Sound, f.Notify.Sound)
	set(&s.Notify.OnDone, f.Notify.OnDone)
	return s, nil
}

// Load reads path. A missing file yields Defaults(); a broken one yields
// Defaults() plus the error, so callers can log it and carry on.
func Load(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), err
	}
	return Parse(data)
}

// ShouldNotify applies the toggles to an attention kind.
func (n Notify) ShouldNotify(kind string) bool {
	if !n.Enabled {
		return false
	}
	return kind != "done" || n.OnDone
}

// WantsSound reports whether a notification of kind should play a sound:
// only requests that block the agent (permission, question) do.
func (n Notify) WantsSound(kind string) bool {
	return n.Sound && (kind == "permission" || kind == "question")
}
