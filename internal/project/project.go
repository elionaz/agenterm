// Package project keeps the registry of project folders, persisted as JSON.
package project

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/fileformat"
)

// Format is the projects.json format version (see internal/fileformat).
const Format = 1

// Project is a registered folder.
type Project struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
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
