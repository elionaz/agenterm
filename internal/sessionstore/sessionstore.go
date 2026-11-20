// Package sessionstore persists what the daemon needs to bring sessions
// back after it restarts: a registry of sessions (runtime dir,
// sessions.json) and each session's scrollback (scrollback/<id>.log).
//
// Records hold what a respawn needs — project, profile id, where it ran,
// the agent's own session id — but never the resolved environment:
// profiles may put secrets there, and a restore re-resolves the profile
// from profiles.yaml anyway.
package sessionstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/atomicfile"
	"github.com/elionaz/agenterm/internal/fileformat"
)

// Resume is what an agent needs to continue the same conversation.
type Resume struct {
	// Agent is the hook integration that reported it: claude | codex.
	Agent string `json:"agent"`
	// SessionID is the agent's own session (claude session_id, codex
	// thread-id).
	SessionID string `json:"sessionId"`
	// TranscriptPath is where the agent keeps it (claude; informational).
	TranscriptPath string    `json:"transcriptPath,omitempty"`
	At             time.Time `json:"at"`
}

// Format is the sessions.json format version (see internal/fileformat).
const Format = 1

// Record is one persisted session.
type Record struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	ProfileID string `json:"profileId"`
	// Name is the user-given display name (empty: the profile's).
	Name string `json:"name,omitempty"`
	// Model is the agent's model as last reported (empty: unknown).
	Model string `json:"model,omitempty"`
	// ContinuedFrom: the sessions whose work this one continues.
	ContinuedFrom []string `json:"continuedFrom,omitempty"`
	// Port is the session's port and PortEnv the variables set to it
	// besides AGENTERM_PORT (kept when it is resumed).
	Port    int      `json:"port,omitempty"`
	PortEnv []string `json:"portEnv,omitempty"`
	// Mode is same | worktree; WorktreeID names the worktree record.
	Mode       string `json:"mode,omitempty"`
	WorktreeID string `json:"worktreeId,omitempty"`
	Cwd        string `json:"cwd"`
	// Cmd and Args are what was launched (shown while suspended; a
	// restore recomputes them from the profile).
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
	// Pid of the last process (to avoid respawning one still alive).
	Pid       int       `json:"pid,omitempty"`
	Cols      uint16    `json:"cols,omitempty"`
	Rows      uint16    `json:"rows,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// State is the last known state (running, idle…, suspended).
	State string `json:"state"`
	// SuspendedAt is when the daemon that ran it stopped (zero while it
	// is live).
	SuspendedAt time.Time `json:"suspendedAt,omitzero"`
	Resume      *Resume   `json:"resume,omitempty"`
}

// Store is a concurrency-safe, file-backed list of records.
type Store struct {
	path string
	mu   sync.Mutex
	recs []Record
}

// Open loads path. A missing file yields an empty store. A file that does
// not parse is moved aside (path.corrupt-<time>) and the store starts
// empty: backup names the moved file ("" if nothing was moved). A file
// written by a newer agenterm, or with a version it can't read, is left
// alone and Open fails (fileformat.Protected): starting empty would drop
// its sessions.
func Open(path string) (s *Store, backup string, err error) {
	s = &Store{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	recs, _, jerr := fileformat.DecodeList[Record](path, data, "sessions", Format)
	if fileformat.Protected(jerr) {
		return nil, "", jerr
	}
	s.recs = recs
	if jerr != nil {
		s.recs = nil
		backup = fmt.Sprintf("%s.corrupt-%s", path, time.Now().UTC().Format("20060102T150405"))
		if err := os.Rename(path, backup); err != nil {
			return nil, "", fmt.Errorf("parse %s: %v; move it aside: %w", path, jerr, err)
		}
		return s, backup, nil
	}
	s.recs = slices.DeleteFunc(s.recs, func(r Record) bool { return !validID(r.ID) })
	return s, "", nil
}

// List returns a copy of every record, in insertion (spawn) order.
func (s *Store) List() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.recs))
	for i, r := range s.recs {
		out[i] = r.clone()
	}
	return out
}

// Get returns the record with id.
func (s *Store) Get(id string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.ID == id {
			return r.clone(), true
		}
	}
	return Record{}, false
}

// Put inserts or replaces (by ID) a record.
func (s *Store) Put(r Record) error {
	if !validID(r.ID) {
		return fmt.Errorf("invalid session id %q", r.ID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.Clone(s.recs)
	if i := slices.IndexFunc(next, func(x Record) bool { return x.ID == r.ID }); i >= 0 {
		next[i] = r.clone()
	} else {
		next = append(next, r.clone())
	}
	return s.commitLocked(next)
}

// Update applies fn to the record with id, if any, and saves when fn
// reports a change.
func (s *Store) Update(id string, fn func(*Record) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.recs, func(x Record) bool { return x.ID == id })
	if i < 0 {
		return nil
	}
	next := slices.Clone(s.recs)
	r := next[i].clone()
	if !fn(&r) {
		return nil
	}
	next[i] = r
	return s.commitLocked(next)
}

// Delete removes the records with the given ids (no error if absent).
func (s *Store) Delete(ids ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := slices.DeleteFunc(slices.Clone(s.recs), func(r Record) bool { return slices.Contains(ids, r.ID) })
	if len(next) == len(s.recs) {
		return nil
	}
	return s.commitLocked(next)
}

// Reorder rearranges the records to follow ids (the daemon's session
// order); records not listed keep their relative order after them. Ids
// without a record are ignored. Restores follow this order.
func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := make(map[string]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	next := slices.Clone(s.recs)
	slices.SortStableFunc(next, func(a, b Record) int {
		pa, oka := pos[a.ID]
		pb, okb := pos[b.ID]
		switch {
		case oka && okb:
			return pa - pb
		case oka:
			return -1
		case okb:
			return 1
		}
		return 0
	})
	if slices.EqualFunc(next, s.recs, func(a, b Record) bool { return a.ID == b.ID }) {
		return nil
	}
	return s.commitLocked(next)
}

func (s *Store) commitLocked(next []Record) error {
	data, err := fileformat.EncodeList("sessions", Format, next)
	if err != nil {
		return err
	}
	// No .bak: the file changes often and a corrupt one is moved aside.
	if err := atomicfile.Replace(s.path, data, 0o600); err != nil {
		return err
	}
	s.recs = next
	return nil
}

func (r Record) clone() Record {
	r.Args = slices.Clone(r.Args)
	if r.Resume != nil {
		c := *r.Resume
		r.Resume = &c
	}
	return r
}

// Session ids are short hex strings; anything else never names a file.
var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validID(id string) bool { return idRe.MatchString(id) }

// ScrollbackFile is where session id's scrollback is kept under dir.
func ScrollbackFile(dir, id string) string { return filepath.Join(dir, id+".log") }

// SaveScrollback atomically writes data (its last limit bytes when
// limit > 0) as session id's scrollback, readable only by the user.
func SaveScrollback(dir, id string, data []byte, limit int) error {
	if !validID(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	if limit > 0 && len(data) > limit {
		data = data[len(data)-limit:]
	}
	return atomicfile.Replace(ScrollbackFile(dir, id), data, 0o600)
}

// LoadScrollback reads session id's scrollback (at most its last limit
// bytes when limit > 0). A missing file is empty, not an error.
func LoadScrollback(dir, id string, limit int) ([]byte, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid session id %q", id)
	}
	f, err := os.Open(ScrollbackFile(dir, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	var off int64
	if limit > 0 && size > int64(limit) {
		off = size - int64(limit)
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil, err
	}
	return buf, nil
}

// RemoveScrollback deletes session id's scrollback (no error if absent).
func RemoveScrollback(dir, id string) error {
	if !validID(id) {
		return nil
	}
	if err := os.Remove(ScrollbackFile(dir, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// PruneScrollback removes scrollback files of sessions not in keep, and
// leftover temp files.
func PruneScrollback(dir string, keep map[string]bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		name := e.Name()
		if id, isLog := strings.CutSuffix(name, ".log"); isLog && keep[id] {
			continue
		}
		os.Remove(filepath.Join(dir, name))
	}
}
