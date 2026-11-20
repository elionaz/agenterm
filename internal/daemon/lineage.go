package daemon

import (
	"slices"

	"github.com/elionaz/agenterm/internal/sessionstore"
)

// Lineage of sessions, for the UIs: which ones agenterm ended (stop,
// close, continue with another agent: their exit code is not a failure)
// and whose work each one continues (session.handoff), so the earlier
// ones show under it instead of as sessions of their own.

func (s *Server) markStopped(id string) {
	s.namesMu.Lock()
	s.stopped[id] = true
	s.namesMu.Unlock()
}

// lineage is whether agenterm ended session id and the sessions it
// continues, oldest first.
func (s *Server) lineage(id string) (stopped bool, from []string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	return s.stopped[id], slices.Clone(s.contFrom[id])
}

// setContinued records that session id continues from's work (kept in
// sessions.json, for a restarted daemon).
func (s *Server) setContinued(id, from string) {
	s.namesMu.Lock()
	if !slices.Contains(s.contFrom[id], from) {
		s.contFrom[id] = append(s.contFrom[id], from)
	}
	list := slices.Clone(s.contFrom[id])
	s.namesMu.Unlock()
	if err := s.store.Update(id, func(r *sessionstore.Record) bool {
		if slices.Equal(r.ContinuedFrom, list) {
			return false
		}
		r.ContinuedFrom = list
		return true
	}); err != nil {
		s.log.Warn("sessions.json", "id", id, "err", err)
	}
}

// forgetLineage drops session id (removed): its own entries, and its
// place in the lists of the sessions that continued it.
func (s *Server) forgetLineage(id string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	delete(s.stopped, id)
	delete(s.contFrom, id)
	for k, v := range s.contFrom {
		if i := slices.Index(v, id); i >= 0 {
			if v = slices.Delete(slices.Clone(v), i, i+1); len(v) == 0 {
				delete(s.contFrom, k)
			} else {
				s.contFrom[k] = v
			}
		}
	}
}
