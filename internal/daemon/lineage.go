package daemon

import "github.com/elionaz/agenterm/internal/sessionstore"

// Lineage of sessions, for the UIs: which ones agenterm ended (stop,
// close, continue with another agent: their exit code is not a failure)
// and which session's work each one continues (session.handoff), so the
// old one shows under the new one instead of as a session of its own.

func (s *Server) markStopped(id string) {
	s.namesMu.Lock()
	s.stopped[id] = true
	s.namesMu.Unlock()
}

// lineage is whether agenterm ended session id and the session it
// continues ("" none).
func (s *Server) lineage(id string) (stopped bool, from string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	return s.stopped[id], s.contFrom[id]
}

// setContinued records that session id continues from's work (kept in
// sessions.json, for a restarted daemon).
func (s *Server) setContinued(id, from string) {
	s.namesMu.Lock()
	s.contFrom[id] = from
	s.namesMu.Unlock()
	if err := s.store.Update(id, func(r *sessionstore.Record) bool {
		if r.ContinuedFrom == from {
			return false
		}
		r.ContinuedFrom = from
		return true
	}); err != nil {
		s.log.Warn("sessions.json", "id", id, "err", err)
	}
}

// forgetLineage drops session id (removed): its own entries, and the
// link of a session that continued it.
func (s *Server) forgetLineage(id string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	delete(s.stopped, id)
	delete(s.contFrom, id)
	for k, v := range s.contFrom {
		if v == id {
			delete(s.contFrom, k)
		}
	}
}
