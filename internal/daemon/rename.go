package daemon

import (
	"strings"
	"time"
	"unicode"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/sessionstore"
)

// Session display names (session.rename). They live in s.names (guarded
// by namesMu, a leaf lock: nothing else is taken while it is held, so it
// is safe from status callbacks) and in the session's sessions.json
// record, so they survive a daemon restart and a resume.

// normName trims name, turns control characters into spaces and caps it at
// protocol.MaxSessionName runes.
func normName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > protocol.MaxSessionName {
		name = strings.TrimSpace(string(r[:protocol.MaxSessionName]))
	}
	return name
}

// sessionName is id's display name ("" when it has none).
func (s *Server) sessionName(id string) string {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	return s.names[id]
}

func (s *Server) setName(id, name string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	if name == "" {
		delete(s.names, id)
	} else {
		s.names[id] = name
	}
}

// rename sets session id's display name (empty clears it), persists it and
// tells the UIs.
func (s *Server) rename(p protocol.RenameParams) (protocol.SessionInfo, error) {
	sess, err := s.lookup(p.SessionID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	name := normName(p.Name)
	s.setName(sess.ID, name)
	if err := s.store.Update(sess.ID, func(r *sessionstore.Record) bool {
		if r.Name == name {
			return false
		}
		r.Name = name
		return true
	}); err != nil {
		s.log.Warn("sessions.json", "id", sess.ID, "err", err)
	}
	s.bus.publish(protocol.Event{Type: protocol.EventSessionRenamed, SessionID: sess.ID, State: string(sess.State()),
		At: time.Now().UTC(), ProjectName: s.projectName(sess.ProjectID, sess.ProjectName), ProfileID: sess.ProfileID, SessionName: name})
	return s.sessionInfo(sess), nil
}
