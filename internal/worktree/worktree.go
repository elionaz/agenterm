// Package worktree persists the git worktrees agenterm created for its
// sessions (runtime dir, worktrees.json), so the daemon still knows them —
// and can offer to reopen, review or remove them — after the session ended
// or the daemon restarted.
package worktree

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/atomicfile"
	"github.com/elionaz/agenterm/internal/fileformat"
)

// Format is the worktrees.json format version (see internal/fileformat).
const Format = 1

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
	// Branches are the other branches its sessions worked on in it (checked
	// out there, or pushed from it), oldest first, without Branch: where
	// the session's pull requests are. Optional, so older versions ignore it.
	Branches []string `json:"branches,omitempty"`
}

// MaxBranches bounds Record.Branches (the oldest are dropped).
const MaxBranches = 30

// AllBranches is Branch and Branches, newest first, without duplicates.
func (r Record) AllBranches() []string {
	out := make([]string, 0, len(r.Branches)+1)
	seen := map[string]bool{}
	for i := len(r.Branches) - 1; i >= 0; i-- {
		if b := r.Branches[i]; b != "" && !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	if r.Branch != "" && !seen[r.Branch] {
		out = append(out, r.Branch)
	}
	return out
}

// HasBranch reports whether b is Branch or in Branches.
func (r Record) HasBranch(b string) bool {
	if b == "" {
		return false
	}
	if b == r.Branch {
		return true
	}
	for _, x := range r.Branches {
		if x == b {
			return true
		}
	}
	return false
}

// ErrNotFound is returned for unknown worktree ids.
var ErrNotFound = errors.New("worktree not found")

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
	if s.recs, _, err = fileformat.DecodeList[Record](path, data, "worktrees", Format); err != nil {
		return nil, err
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
	return Record{}, apierr.New(apierr.WorktreeNotFound, map[string]any{"id": id}, "%w: %s", ErrNotFound, id)
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

// AddBranch records that worktree id worked on branch (see
// Record.Branches); it reports whether the record changed. Its own Branch
// and branches already listed change nothing, except moving to the end.
func (s *Store) AddBranch(id, branch string) (bool, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.recs {
		r := &s.recs[i]
		if r.ID != id {
			continue
		}
		if branch == r.Branch {
			return false, nil
		}
		if n := len(r.Branches); n > 0 && r.Branches[n-1] == branch {
			return false, nil // already the newest
		}
		prev := append([]string(nil), r.Branches...)
		next := make([]string, 0, len(prev)+1)
		for _, b := range prev {
			if b != branch {
				next = append(next, b)
			}
		}
		next = append(next, branch)
		if len(next) > MaxBranches {
			next = next[len(next)-MaxBranches:]
		}
		r.Branches = next
		if err := s.saveLocked(); err != nil {
			r.Branches = prev
			return false, err
		}
		return true, nil
	}
	return false, apierr.New(apierr.WorktreeNotFound, map[string]any{"id": id}, "%w: %s", ErrNotFound, id)
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
	data, err := fileformat.EncodeList("worktrees", Format, s.recs)
	if err != nil {
		return err
	}
	return atomicfile.Write(s.path, data, 0o600)
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
