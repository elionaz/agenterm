// Package profile resolves agent launch profiles.
//
// The effective profiles are the built-in catalog (catalog.go) overlaid by
// the user's profiles.yaml, matched by id: fields set in the file win, unset
// ones inherit the catalog defaults. The file can also define custom
// profiles (any command), variants (`extends: <id>` plus their own fields),
// hide profiles (`enabled: false`) and set their order (list position, or an
// explicit `order:`). See Resolve for the exact rules.
package profile

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/status"
)

// Profile sources.
const (
	SourceCatalog  = "catalog"  // built-in, no user fields
	SourceOverride = "override" // built-in with fields from profiles.yaml
	SourceCustom   = "custom"   // only in profiles.yaml (including variants)
)

// Profile is an effective, ready-to-launch profile.
type Profile struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Cmd    string            `json:"cmd"`
	Args   []string          `json:"args,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	Status status.Mode       `json:"status"`
	// SetupHooks wires the agent's own hooks to agenterm for sessions it
	// spawns: claude | codex | none (see internal/hooks).
	SetupHooks string `json:"setupHooks,omitempty"`
	// AttentionPatterns are regular expressions matched against recent
	// output (ANSI removed); a match marks the session as waiting for you.
	AttentionPatterns []string `json:"attentionPatterns,omitempty"`
	// IdleSeconds overrides the daemon's heuristic idle timeout (optional).
	IdleSeconds float64 `json:"idleSeconds,omitempty"`
	// ResumeArgs are appended to Args when a session is respawned after a
	// daemon restart and there is no agent session id to resume (claude
	// and codex resume theirs by id). E.g. ["--continue"]. Empty: the
	// plain command.
	ResumeArgs []string `json:"resumeArgs,omitempty"`

	Source  string `json:"source"`
	Extends string `json:"extends,omitempty"`
	// Enabled: shown in the spawn menu (when also Installed).
	Enabled bool `json:"enabled"`
	// Installed: the command was found; Path is where.
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	// Homepage and Install come from the catalog entry this profile
	// derives from.
	Homepage string `json:"homepage,omitempty"`
	Install  string `json:"install,omitempty"`
	// Error explains why the profile cannot be used (bad regexp, unknown
	// extends…). Other profiles are unaffected.
	Error string `json:"error,omitempty"`
}

// Launchable reports whether the profile belongs in a spawn menu.
func (p Profile) Launchable() bool { return p.Enabled && p.Installed && p.Error == "" }

// Entry is one item of profiles.yaml. Empty / nil fields inherit from the
// catalog entry with the same id, or from Extends. Lists and maps are
// pointers so that an explicit empty value (`args: []`) can be told apart
// from "inherit".
type Entry struct {
	ID      string `yaml:"id" json:"id"`
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Extends string `yaml:"extends,omitempty" json:"extends,omitempty"`
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// Order sorts entries (lower first); without it the list position is
	// used. Rewritten files drop it and keep the list in order instead.
	Order             *int               `yaml:"order,omitempty" json:"order,omitempty"`
	Cmd               string             `yaml:"cmd,omitempty" json:"cmd,omitempty"`
	Args              *[]string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env               *map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Status            status.Mode        `yaml:"status,omitempty" json:"status,omitempty"`
	SetupHooks        string             `yaml:"setup_hooks,omitempty" json:"setupHooks,omitempty"`
	IdleSeconds       float64            `yaml:"idle_seconds,omitempty" json:"idleSeconds,omitempty"`
	AttentionPatterns *[]string          `yaml:"attention_patterns,omitempty" json:"attentionPatterns,omitempty"`
	ResumeArgs        *[]string          `yaml:"resume_args,omitempty" json:"resumeArgs,omitempty"`
}

// isStub reports whether the entry only pins the position of an id.
func (e Entry) isStub() bool {
	return e == Entry{ID: e.ID}
}

// Parse decodes a YAML list of entries. Only structural problems (syntax,
// missing or duplicate ids) are errors; semantic ones are reported per
// profile by Resolve.
func Parse(data []byte) ([]Entry, error) {
	var es []Entry
	if err := yaml.Unmarshal(data, &es); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	seen := make(map[string]bool, len(es))
	for i, e := range es {
		if e.ID == "" {
			return nil, fmt.Errorf("profile #%d: missing id", i+1)
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("profile %q: duplicate id", e.ID)
		}
		seen[e.ID] = true
	}
	return es, nil
}

// ReadEntries reads path. A missing file has no entries.
func ReadEntries(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Load reads path and resolves it against the catalog.
func Load(path string, look Lookup) ([]Profile, error) {
	es, err := ReadEntries(path)
	if err != nil {
		return nil, err
	}
	return Resolve(es, look), nil
}

// Resolve computes the effective profiles:
//
//   - every catalog entry, overlaid by the user entry with the same id;
//   - every other user entry, overlaid on its `extends` parent (resolved
//     first, user fields included) or on nothing for plain custom ones;
//   - user fields win; "" / nil / 0 inherit; `setup_hooks: none` disables;
//   - Enabled defaults to true and is not inherited;
//   - order: user entries by `order:` or list position, then catalog
//     entries the file does not mention, in catalog order.
//
// look (may be nil) decides Installed/Path and which catalog binary is used.
func Resolve(entries []Entry, look Lookup) []Profile {
	r := resolver{
		look:    look,
		user:    make(map[string]Entry, len(entries)),
		done:    make(map[string]Profile),
		visited: make(map[string]bool),
	}
	for _, e := range entries {
		r.user[e.ID] = e
	}

	type keyed struct {
		id  string
		key int
	}
	var ks []keyed
	for i, e := range entries {
		k := i
		if e.Order != nil {
			k = *e.Order
		}
		ks = append(ks, keyed{e.ID, k})
	}
	sort.SliceStable(ks, func(i, j int) bool { return ks[i].key < ks[j].key })
	ids := make([]string, 0, len(ks)+len(catalog))
	for _, k := range ks {
		ids = append(ids, k.id)
	}
	for _, c := range catalog {
		if _, ok := r.user[c.ID]; !ok {
			ids = append(ids, c.ID)
		}
	}
	out := make([]Profile, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.resolve(id))
	}
	return out
}

type resolver struct {
	look    Lookup
	user    map[string]Entry
	done    map[string]Profile
	visited map[string]bool
}

func (r *resolver) resolve(id string) Profile {
	if p, ok := r.done[id]; ok {
		return p
	}
	if r.visited[id] {
		return Profile{ID: id, Name: id, Source: SourceCustom, Error: "extends cycle"}
	}
	r.visited[id] = true
	p := r.build(id)
	r.done[id] = p
	return p
}

func (r *resolver) build(id string) Profile {
	e, hasUser := r.user[id]
	ci := catalogIndex(id)
	var p Profile
	switch {
	case ci >= 0:
		c := catalog[ci]
		p = Profile{
			ID: c.ID, Name: c.Name, Cmd: c.command(r.look), Args: c.Args, Status: c.Status,
			SetupHooks: c.SetupHooks, AttentionPatterns: c.AttentionPatterns, ResumeArgs: c.ResumeArgs,
			Homepage: c.Homepage, Install: c.Install, Source: SourceCatalog,
		}
		if hasUser && !e.isStub() {
			p.Source = SourceOverride
		}
		if hasUser && e.Extends != "" {
			p.Error = "extends is only allowed on custom profiles"
		}
	case e.Extends != "":
		if e.Extends == id {
			return r.finish(Profile{ID: id, Name: id, Source: SourceCustom, Extends: id}, e, "extends itself")
		}
		if _, known := r.user[e.Extends]; !known && catalogIndex(e.Extends) < 0 {
			return r.finish(Profile{ID: id, Name: id, Source: SourceCustom, Extends: e.Extends}, e,
				fmt.Sprintf("extends unknown profile %q", e.Extends))
		}
		parent := r.resolve(e.Extends)
		p = parent
		p.ID, p.Name, p.Source, p.Extends = id, id, SourceCustom, e.Extends
		p.Error = ""
		if parent.Error != "" {
			p.Error = fmt.Sprintf("parent %q: %s", e.Extends, parent.Error)
		}
		// Copy what the overlay may share with the parent.
		p.Args = slices.Clone(p.Args)
		p.AttentionPatterns = slices.Clone(p.AttentionPatterns)
		p.ResumeArgs = slices.Clone(p.ResumeArgs)
		if p.Env != nil {
			env := make(map[string]string, len(p.Env))
			for k, v := range p.Env {
				env[k] = v
			}
			p.Env = env
		}
	default:
		p = Profile{ID: id, Name: id, Source: SourceCustom}
	}
	return r.finish(p, e, p.Error)
}

// finish overlays the user entry, validates and detects.
func (r *resolver) finish(p Profile, e Entry, errMsg string) Profile {
	if e.ID != "" {
		p = overlay(p, e)
	}
	p.Enabled = e.Enabled == nil || *e.Enabled
	if errMsg == "" {
		if err := validate(p); err != nil {
			errMsg = err.Error()
		}
	}
	p.Error = errMsg
	if mode, err := status.ParseMode(string(p.Status)); err == nil {
		p.Status = mode
	}
	if r.look != nil && p.Cmd != "" {
		p.Path, p.Installed = r.look(p.Command())
	}
	return p
}

func overlay(p Profile, e Entry) Profile {
	if e.Name != "" {
		p.Name = e.Name
	}
	if e.Cmd != "" {
		p.Cmd = e.Cmd
	}
	if e.Args != nil {
		p.Args = slices.Clone(*e.Args)
	}
	if e.Env != nil {
		p.Env = make(map[string]string, len(*e.Env))
		for k, v := range *e.Env {
			p.Env[k] = v
		}
	}
	if e.Status != "" {
		p.Status = e.Status
	}
	if e.SetupHooks != "" {
		p.SetupHooks = e.SetupHooks
	}
	if p.SetupHooks == "none" {
		p.SetupHooks = ""
	}
	if e.IdleSeconds != 0 {
		p.IdleSeconds = e.IdleSeconds
	}
	if e.AttentionPatterns != nil {
		p.AttentionPatterns = slices.Clone(*e.AttentionPatterns)
	}
	if e.ResumeArgs != nil {
		p.ResumeArgs = slices.Clone(*e.ResumeArgs)
	}
	return p
}

func validate(p Profile) error {
	if p.Cmd == "" {
		return errors.New("missing cmd")
	}
	if _, err := status.ParseMode(string(p.Status)); err != nil {
		return err
	}
	switch p.SetupHooks {
	case "", "none", "claude", "codex":
	default:
		return fmt.Errorf("unknown setup_hooks %q (want claude, codex or none)", p.SetupHooks)
	}
	if p.IdleSeconds < 0 {
		return errors.New("idle_seconds must be positive")
	}
	if _, err := p.Patterns(); err != nil {
		return err
	}
	return nil
}

// Patterns compiles AttentionPatterns.
func (p Profile) Patterns() ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(p.AttentionPatterns))
	for _, s := range p.AttentionPatterns {
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("attention_patterns: %w", err)
		}
		out = append(out, re)
	}
	return out, nil
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
