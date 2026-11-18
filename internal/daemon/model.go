package daemon

import (
	"strings"
	"time"
	"unicode"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/sessionstore"
)

// The model each session's agent uses, as the sidebar shows it. Claude
// reports it through its hooks (payload or transcript, see hooks.Model);
// other agents only when their command line names it (argsModel). Kept in
// s.models (under namesMu, a leaf lock) and in the session's record, so it
// survives a restart.

// maxModel bounds a model id.
const maxModel = 80

// normModel keeps model ids printable and short ("" for nothing usable).
func normModel(m string) string {
	m = strings.TrimSpace(m)
	if m == "" || len(m) > maxModel || strings.HasPrefix(m, "<") {
		return ""
	}
	for _, r := range m {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ""
		}
	}
	return m
}

func (s *Server) sessionModel(id string) string {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	return s.models[id]
}

// setModel records sess's model; when it changed, it is persisted and the
// UIs are told (session.model).
func (s *Server) setModel(sess *session.Session, model string) {
	model = normModel(model)
	if model == "" {
		return
	}
	s.namesMu.Lock()
	changed := s.models[sess.ID] != model
	s.models[sess.ID] = model
	s.namesMu.Unlock()
	if !changed {
		return
	}
	if err := s.store.Update(sess.ID, func(r *sessionstore.Record) bool {
		if r.Model == model {
			return false
		}
		r.Model = model
		return true
	}); err != nil {
		s.log.Warn("sessions.json", "id", sess.ID, "err", err)
	}
	s.bus.publish(protocol.Event{Type: protocol.EventSessionModel, SessionID: sess.ID, State: string(sess.State()),
		At: time.Now().UTC(), Model: model})
}

func (s *Server) forgetModel(id string) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	delete(s.models, id)
}

// argsModel is the model a command line asks for: --model X, --model=X,
// -m X, or codex's -c model=X ("" when none).
func argsModel(args []string) string {
	for i, a := range args {
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case a == "--model" || a == "-m":
			return normModel(next)
		case strings.HasPrefix(a, "--model="):
			return normModel(strings.TrimPrefix(a, "--model="))
		case a == "-c" || a == "--config":
			if v, ok := strings.CutPrefix(next, "model="); ok {
				return normModel(strings.Trim(v, `"'`))
			}
		}
	}
	return ""
}
