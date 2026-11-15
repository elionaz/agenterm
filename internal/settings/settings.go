// Package settings reads the optional user settings file
// (~/.config/agenterm/settings.yaml). Every toggle defaults to "on".
package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/atomicfile"
)

// Notify controls attention notifications.
type Notify struct {
	Enabled bool `json:"enabled"`       // native notifications at all
	Sound   bool `json:"sound"`         // sound for permission/question notifications
	OnDone  bool `json:"notifyOnDone"`  // also notify when an agent finishes its turn
	OnStall bool `json:"notifyOnStall"` // notify (quietly) when a turn looks stalled
}

// Activity controls activity tracking.
type Activity struct {
	// StallSeconds: a hook-driven turn with neither output nor hook events
	// for this long is flagged as stalled. 0 disables stall detection.
	StallSeconds int `json:"stallSeconds"`
}

// StallAfter is StallSeconds as a duration (0 = disabled).
func (a Activity) StallAfter() time.Duration {
	return time.Duration(max(a.StallSeconds, 0)) * time.Second
}

// DefaultStallSeconds is activity.stall_seconds when unset.
const DefaultStallSeconds = 180

// Settings is the parsed file.
type Settings struct {
	Notify   Notify   `json:"notify"`
	Activity Activity `json:"activity"`
}

type fileNotify struct {
	Enabled *bool `yaml:"enabled"`
	Sound   *bool `yaml:"sound"`
	OnDone  *bool `yaml:"notify_on_done"`
	OnStall *bool `yaml:"notify_on_stall"`
}

type fileActivity struct {
	StallSeconds *int `yaml:"stall_seconds"`
}

type file struct {
	Notify   fileNotify   `yaml:"notify"`
	Activity fileActivity `yaml:"activity"`
}

const header = "# agenterm: ajustes (ver README)\n"

// Save writes s to path atomically (keeping a .bak of the previous file).
// The existing file is edited in place: unknown keys and comments are kept,
// only the known keys are set. A file that does not parse is replaced.
func Save(path string, s Settings) error {
	var doc yaml.Node
	if old, err := os.ReadFile(path); err == nil {
		if yaml.Unmarshal(old, &doc) != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
			doc = yaml.Node{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, HeadComment: header[2 : len(header)-1],
			Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	b := strconv.FormatBool
	setKey(root, []string{"notify", "enabled"}, b(s.Notify.Enabled), "!!bool")
	setKey(root, []string{"notify", "sound"}, b(s.Notify.Sound), "!!bool")
	setKey(root, []string{"notify", "notify_on_done"}, b(s.Notify.OnDone), "!!bool")
	setKey(root, []string{"notify", "notify_on_stall"}, b(s.Notify.OnStall), "!!bool")
	setKey(root, []string{"activity", "stall_seconds"}, strconv.Itoa(max(s.Activity.StallSeconds, 0)), "!!int")
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return atomicfile.Write(path, buf.Bytes(), 0o600)
}

// setKey sets the scalar at path inside mapping m, creating (or replacing
// non-mapping) intermediate nodes as needed.
func setKey(m *yaml.Node, path []string, value, tag string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value != path[0] {
			continue
		}
		if len(path) == 1 {
			v.Kind, v.Tag, v.Value, v.Style, v.Content, v.Alias = yaml.ScalarNode, tag, value, 0, nil, nil
			return
		}
		if v.Kind != yaml.MappingNode {
			*v = yaml.Node{Kind: yaml.MappingNode, LineComment: v.LineComment, HeadComment: v.HeadComment}
		}
		setKey(v, path[1:], value, tag)
		return
	}
	k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: path[0]}
	if len(path) == 1 {
		m.Content = append(m.Content, k, &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value})
		return
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, k, v)
	setKey(v, path[1:], value, tag)
}

// Defaults returns the settings used when the file (or a key) is missing.
func Defaults() Settings {
	return Settings{
		Notify:   Notify{Enabled: true, Sound: true, OnDone: true, OnStall: true},
		Activity: Activity{StallSeconds: DefaultStallSeconds},
	}
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
	set(&s.Notify.OnStall, f.Notify.OnStall)
	if f.Activity.StallSeconds != nil {
		s.Activity.StallSeconds = max(*f.Activity.StallSeconds, 0)
	}
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
	switch kind {
	case "done":
		return n.OnDone
	case "stall":
		return n.OnStall
	}
	return true
}

// WantsSound reports whether a notification of kind should play a sound:
// only requests that block the agent (permission, question) do.
func (n Notify) WantsSound(kind string) bool {
	return n.Sound && (kind == "permission" || kind == "question")
}
