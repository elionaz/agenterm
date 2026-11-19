package daemon

import (
	"fmt"
	"net"
	"strings"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/peercred"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
)

// handleShutdown serves daemon.shutdown. Without force it refuses while any
// session is alive, so a stray `agenterm stop` (a test cleanup aimed at the
// wrong daemon…) cannot take live work down. Every request is logged with
// the requesting process. Signals (SIGTERM/SIGINT) bypass this: that is the
// OS asking.
func (s *Server) handleShutdown(conn net.Conn, req protocol.Request) (any, error) {
	p, err := decode[protocol.ShutdownParams](req.Params)
	if err != nil {
		return nil, err
	}
	attrs := append(peerAttrs(conn), "force", p.Force)
	live := s.liveSessions()
	if !p.Force && len(live) > 0 {
		s.log.Warn("daemon.shutdown refused: live sessions", append(attrs, "live", len(live))...)
		return nil, liveSessionsError(live)
	}
	s.log.Info("daemon.shutdown accepted", append(attrs, "live", len(live))...)
	s.stopOnce.Do(func() {
		s.stopAttrs = attrs
		close(s.stop)
	})
	return map[string]any{"ok": true}, nil
}

// liveSessions lists the sessions with a running process (not exited nor
// suspended), in spawn order.
func (s *Server) liveSessions() []*session.Session {
	s.mu.Lock()
	all := make([]*session.Session, 0, len(s.order))
	for _, id := range s.order {
		if sess := s.sessions[id]; sess != nil {
			all = append(all, sess)
		}
	}
	s.mu.Unlock()
	// Outside s.mu: Alive takes the session's status lock, whose change
	// callbacks take s.mu (publishState).
	var out []*session.Session
	for _, sess := range all {
		if sess.Alive() {
			out = append(out, sess)
		}
	}
	return out
}

func liveSessionsError(live []*session.Session) error {
	names := make([]string, len(live))
	list := make([]map[string]string, len(live))
	for i, sess := range live {
		names[i] = sess.ID + " (" + sess.ProfileID + ")"
		list[i] = map[string]string{"id": sess.ID, "profileId": sess.ProfileID}
	}
	what := fmt.Sprintf("%d live sessions", len(live))
	if len(live) == 1 {
		what = "1 live session"
	}
	return apierr.New(apierr.LiveSessions, map[string]any{"count": len(live), "sessions": list},
		"%s: %s; use --force to stop them", what, strings.Join(names, ", "))
}

// peerAttrs identifies the process behind conn for the log: pid, parent
// pid and both command lines (best effort, bounded by ps' timeout).
func peerAttrs(conn net.Conn) []any {
	pid, err := peercred.PeerPID(conn)
	if err != nil {
		return []any{"peer_pid", "unknown", "peer_err", err.Error()}
	}
	attrs := []any{"peer_pid", pid}
	proc, err := peercred.Lookup(pid)
	if err != nil {
		return append(attrs, "peer_cmd", "unknown")
	}
	attrs = append(attrs, "peer_cmd", clip(proc.Command))
	if proc.PPID > 0 {
		attrs = append(attrs, "peer_ppid", proc.PPID)
		if parent, err := peercred.Lookup(proc.PPID); err == nil {
			attrs = append(attrs, "peer_parent_cmd", clip(parent.Command))
		}
	}
	return attrs
}

// clip bounds a command line for the log (shell wrappers can be huge).
func clip(s string) string {
	const max = 300
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
