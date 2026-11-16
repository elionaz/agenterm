// Package worktree persists the git worktrees agenterm created for its
// sessions (runtime dir, worktrees.json), so the daemon still knows them —
// and can offer to reopen, review or remove them — after the session ended
// or the daemon restarted.
package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/atomicfile"
)

// Record is one agenterm-managed worktree.
type Record struct {
	// ID is the id of the session it was created for (also its directory
	// name).
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	// Repo is the repository's main work tree (where git commands for it
	// run); Path the worktree itself; Subdir the project's path relative to
	// the repository root ("" when the project is the root).
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Subdir string `json:"subdir,omitempty"`
	Branch string `json:"branch"`
	// Base is the branch it was created from; BaseCommit that branch's
	// commit at the time (fallback if the branch is gone).
	Base       string    `json:"base"`
	BaseCommit string    `json:"baseCommit,omitempty"`
	ProfileID  string    `json:"profileId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	// Adopted: found on disk (git worktree list) without a record.
	Adopted bool `json:"adopted,omitempty"`
}

// ErrNotFound is returned for unknown worktree ids.
var ErrNotFound = errors.New("worktree no encontrado")

// Store is a concurrency-safe, file-backed list of records.
type Store struct {
	path string
	mu   sync.Mutex
	recs []Record
}

// Open loads path; a missing file yields an empty store.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.recs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// List returns a copy of every record (optionally of one project).
func (s *Store) List(projectID string) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Record
	for _, r := range s.recs {
		if projectID == "" || r.ProjectID == projectID {
			out = append(out, r)
		}
	}
	return out
}

// Get returns the record with id.
func (s *Store) Get(id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Put inserts or replaces (by ID) a record.
func (s *Store) Put(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.recs
	next := append([]Record(nil), prev...)
	replaced := false
	for i := range next {
		if next[i].ID == r.ID {
			next[i], replaced = r, true
			break
		}
	}
	if !replaced {
		next = append(next, r)
	}
	s.recs = next
	if err := s.saveLocked(); err != nil {
		s.recs = prev
		return err
	}
	return nil
}

// Delete removes the record with id (no error if it does not exist).
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := s.recs
	var next []Record
	for _, r := range prev {
		if r.ID != id {
			next = append(next, r)
		}
	}
	if len(next) == len(prev) {
		return nil
	}
	s.recs = next
	if err := s.saveLocked(); err != nil {
		s.recs = prev
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	list := s.recs
	if list == nil {
		list = []Record{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(s.path, append(data, '\n'), 0o600)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// ProjectSlug names a project's folder under the worktrees dir:
// "<name>-<id>" in lowercase ASCII.
func ProjectSlug(name, id string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		return id
	}
	return s + "-" + id
}
