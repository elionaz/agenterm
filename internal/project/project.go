// Package project keeps the registry of project folders, persisted as JSON.
package project

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/fileformat"
	"github.com/elionaz/agenterm/internal/order"
)

// Format is the projects.json format version (see internal/fileformat).
const Format = 1

// Project is a registered folder.
type Project struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
	// Color is the badge color, one of Colors; empty picks one from the id.
	Color string `json:"color,omitempty"`
	// Icon replaces the badge initials: an emoji (or a short text).
	Icon string `json:"icon,omitempty"`
}

// Colors are the badge colors a project can pick (the app maps each to a
// theme token).
var Colors = []string{"blue", "teal", "green", "amber", "orange", "red", "pink", "purple"}

// Limits. An icon has room for one emoji, skin-toned ZWJ sequences and
// tag flags included (the app also checks it is at most two graphemes).
const (
	maxNameRunes = 80
	maxIconBytes = 64
	maxIconRunes = 16
)

// Patch changes some of a project's fields; nil leaves a field as is.
// An empty Color or Icon goes back to the automatic badge.
type Patch struct {
	Name  *string
	Color *string
	Icon  *string
}

// ErrNotFound is returned for unknown project ids.
var ErrNotFound = errors.New("project not found")

// Registry is a concurrency-safe, file-backed project list.
type Registry struct {
	path string

	mu       sync.Mutex
	projects []Project
}

// Open loads the registry at path; a missing file yields an empty registry.
func Open(path string) (*Registry, error) {
	r := &Registry{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if r.projects, _, err = fileformat.DecodeList[Project](path, data, "projects", Format); err != nil {
		return nil, err
	}
	return r, nil
}

// Add registers dir (made absolute). Adding an already-registered path
// returns the existing project.
func (r *Registry) Add(dir, name string) (Project, error) {
	if !filepath.IsAbs(dir) {
		return Project{}, fmt.Errorf("project path must be absolute: %q", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Project{}, err
	}
	if !fi.IsDir() {
		return Project{}, fmt.Errorf("%s is not a directory", abs)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.projects {
		if p.Path == abs {
			return p, nil
		}
	}
	if name == "" {
		name = filepath.Base(abs)
	}
	p := Project{ID: newID(), Name: name, Path: abs, AddedAt: time.Now().UTC().Truncate(time.Second)}
	r.projects = append(r.projects, p)
	if err := r.saveLocked(); err != nil {
		r.projects = r.projects[:len(r.projects)-1]
		return Project{}, err
	}
	return p, nil
}

// List returns a copy of all projects in insertion order.
func (r *Registry) List() []Project {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Project(nil), r.projects...)
}

// Get resolves a project by id, falling back to an exact name match.
func (r *Registry) Get(ref string) (Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.projects {
		if p.ID == ref {
			return p, nil
		}
	}
	for _, p := range r.projects {
		if p.Name == ref {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
}

// Remove deletes the project with id.
func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range r.projects {
		if p.ID == id {
			prev := r.projects
			r.projects = append(append([]Project(nil), prev[:i]...), prev[i+1:]...)
			if err := r.saveLocked(); err != nil {
				r.projects = prev
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Reorder moves the given project ids to the front, in that order; the rest
// keep their relative order after them. The registry order is the sidebar's.
func (r *Registry) Reorder(ids []string) ([]Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := make([]string, len(r.projects))
	byID := make(map[string]Project, len(r.projects))
	for i, p := range r.projects {
		cur[i] = p.ID
		byID[p.ID] = p
	}
	next, err := order.Front(cur, ids)
	if err != nil {
		return nil, fmt.Errorf("reorder projects: %w", err)
	}
	prev := r.projects
	r.projects = make([]Project, len(next))
	for i, id := range next {
		r.projects[i] = byID[id]
	}
	if err := r.saveLocked(); err != nil {
		r.projects = prev
		return nil, err
	}
	return append([]Project(nil), r.projects...), nil
}

// Update applies patch to the project with id.
func (r *Registry) Update(id string, patch Patch) (Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.projects, func(p Project) bool { return p.ID == id })
	if i < 0 {
		return Project{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	p := r.projects[i]
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return Project{}, apierr.New(apierr.ProjectNameEmpty, nil, "a project name cannot be empty")
		}
		if utf8.RuneCountInString(name) > maxNameRunes || strings.ContainsFunc(name, unicode.IsControl) {
			return Project{}, apierr.New(apierr.ProjectNameInvalid, map[string]any{"max": maxNameRunes},
				"a project name has at most %d characters and no control characters", maxNameRunes)
		}
		for _, o := range r.projects {
			// Another project's id counts too: references resolve ids first.
			if o.ID != id && (o.Name == name || o.ID == name) {
				return Project{}, apierr.New(apierr.ProjectNameTaken, map[string]any{"name": name},
					"another project is already called %q", name)
			}
		}
		p.Name = name
	}
	if patch.Color != nil {
		c := strings.TrimSpace(*patch.Color)
		if c != "" && !slices.Contains(Colors, c) {
			return Project{}, apierr.New(apierr.ProjectColorInvalid, map[string]any{"color": c},
				"unknown color %q (use one of %s)", c, strings.Join(Colors, ", "))
		}
		p.Color = c
	}
	if patch.Icon != nil {
		icon, err := normIcon(*patch.Icon)
		if err != nil {
			return Project{}, err
		}
		p.Icon = icon
	}
	prev := r.projects
	r.projects = slices.Clone(prev)
	r.projects[i] = p
	if err := r.saveLocked(); err != nil {
		r.projects = prev
		return Project{}, err
	}
	return p, nil
}

// normIcon trims icon and checks it fits a badge.
func normIcon(icon string) (string, error) {
	icon = strings.TrimSpace(icon)
	if len(icon) > maxIconBytes || utf8.RuneCountInString(icon) > maxIconRunes || !utf8.ValidString(icon) ||
		strings.ContainsFunc(icon, unicode.IsControl) {
		return "", apierr.New(apierr.ProjectIconInvalid, nil, "icon %q does not fit a badge: use one emoji or two characters", icon)
	}
	return icon, nil
}

func (r *Registry) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	data, err := fileformat.EncodeList("projects", Format, r.projects)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".projects-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), r.path)
}

func newID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
