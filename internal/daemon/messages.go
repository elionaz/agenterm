package daemon

import (
	"errors"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/i18n"
	"github.com/elionaz/agenterm/internal/mailbox"
	"github.com/elionaz/agenterm/internal/peercred"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/settings"
	"github.com/elionaz/agenterm/internal/status"
)

// Messages between the sessions of a project (msg.*). Who sends is never
// taken from the request: a call from a process inside a session (its
// PTY's session, or a descendant) is from that session; anything else (the
// app, a terminal outside agenterm) is from the user. Messages are
// delivered by agent hooks (msg.take from `notify --from-hook`), by
// `agenterm msg read`, or nudged by the user (msg.deliver, which types a
// fixed line, never the message, into the recipient's terminal).

// maxAncestors bounds the walk up a caller's process tree.
const maxAncestors = 32

// callerSession is the live session conn's peer runs in, nil for none.
func (s *Server) callerSession(conn net.Conn) *session.Session {
	pid, err := peercred.PeerPID(conn)
	if err != nil || pid <= 1 {
		return nil
	}
	byPid := make(map[int]*session.Session)
	// ExitCode takes the session's status lock, whose change callbacks take
	// s.mu (publishState): never call it with s.mu held.
	s.mu.Lock()
	all := make([]*session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		all = append(all, sess)
	}
	s.mu.Unlock()
	for _, sess := range all {
		if sess.Pid > 0 && !sess.Suspended() {
			if _, exited := sess.ExitCode(); !exited {
				byPid[sess.Pid] = sess
			}
		}
	}
	if len(byPid) == 0 {
		return nil
	}
	// A session's process leads its own (PTY) session: what runs in it
	// shares that session id, unless it started one of its own.
	if sid, err := unix.Getsid(pid); err == nil {
		if sess := byPid[sid]; sess != nil {
			return sess
		}
	}
	for i, p := 0, pid; i < maxAncestors && p > 1; i++ {
		if sess := byPid[p]; sess != nil {
			return sess
		}
		pp, err := peercred.Parent(p)
		if err != nil {
			return nil
		}
		p = pp
	}
	return nil
}

func (s *Server) messagesEnabled() bool {
	st, _ := settings.Load(s.cfg.Paths.Settings())
	return st.Messages.Enabled
}

// handleMsg serves a msg.* request (they need the connection: the caller).
func (s *Server) handleMsg(conn net.Conn, req protocol.Request) (any, error) {
	caller := s.callerSession(conn)
	switch req.Method {
	case protocol.MethodMsgPeers:
		p, err := decode[protocol.MsgPeersParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.msgPeers(caller, p.SessionID)
	case protocol.MethodMsgSend:
		p, err := decode[protocol.MsgSendParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.msgSend(caller, p)
	case protocol.MethodMsgList:
		p, err := decode[protocol.MsgListParams](req.Params)
		if err != nil {
			return nil, err
		}
		f := mailbox.Filter{Session: p.SessionID, ProjectID: p.ProjectID, PendingOnly: p.PendingOnly}
		if caller != nil {
			f = mailbox.Filter{Session: caller.ID, PendingOnly: p.PendingOnly}
		}
		return protocol.MsgList{Messages: s.mail.List(f)}, nil
	case protocol.MethodMsgTake:
		p, err := decode[protocol.MsgTakeParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.msgTake(caller, p)
	case protocol.MethodMsgAck:
		p, err := decode[protocol.MsgAckParams](req.Params)
		if err != nil {
			return nil, err
		}
		if caller == nil {
			return nil, apierr.New(apierr.MsgNotInSession, nil, "only a session can acknowledge its messages")
		}
		n, err := s.mail.Ack(caller.ID, p.IDs)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			s.log.Info("messages delivered", "to", caller.ID, "count", n, "via", "hook")
			s.publishMessages(caller.ProjectID, caller.ID)
			for _, from := range senders(s.mail.List(mailbox.Filter{To: caller.ID})) {
				s.publishMessages(caller.ProjectID, from)
			}
		}
		return map[string]any{"acked": n}, nil
	case protocol.MethodMsgDeliver:
		p, err := decode[protocol.MsgDeliverParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.msgDeliver(caller, p.SessionID)
	}
	return nil, apierr.New(apierr.UnknownMethod, map[string]string{"method": req.Method}, "unknown method %q", req.Method)
}

// displayName is how a session is named to other sessions: its user-given
// name, else its profile.
func (s *Server) displayName(sess *session.Session) string {
	if n := s.sessionName(sess.ID); n != "" {
		return n
	}
	return sess.ProfileID
}

// projectSessions are the sessions of projectID that are not exited, in
// the sidebar's order.
func (s *Server) projectSessions(projectID string) []*session.Session {
	s.mu.Lock()
	var mine []*session.Session
	for _, id := range s.order {
		if sess := s.sessions[id]; sess != nil && sess.ProjectID == projectID {
			mine = append(mine, sess)
		}
	}
	s.mu.Unlock()
	// Outside s.mu: see callerSession.
	var out []*session.Session
	for _, sess := range mine {
		if _, exited := sess.ExitCode(); exited && !sess.Suspended() {
			continue
		}
		out = append(out, sess)
	}
	return out
}

func (s *Server) msgPeer(sess *session.Session, self *session.Session) protocol.MsgPeer {
	p := protocol.MsgPeer{ID: sess.ID, Name: s.displayName(sess), ProfileID: sess.ProfileID, State: string(sess.State()),
		Activity: sess.Live().Activity}
	if sess.Suspended() {
		p.State = "suspended"
	}
	g, sg := s.sessionGit(sess.ID), (*sessGit)(nil)
	if self != nil {
		sg = s.sessionGit(self.ID)
	}
	if g != nil {
		p.Mode, p.Branch = g.Mode, g.Branch
		p.SameFolder = sg != nil && filepath.Clean(sg.Dir) == filepath.Clean(g.Dir)
	}
	return p
}

func (s *Server) msgPeers(caller *session.Session, sessionID string) (protocol.MsgPeers, error) {
	ref := caller
	if ref == nil {
		if sessionID == "" {
			return protocol.MsgPeers{}, apierr.New(apierr.MsgNotInSession, nil,
				"not inside an agenterm session: name one (--session) to see its project")
		}
		sess, err := s.lookup(sessionID)
		if err != nil {
			return protocol.MsgPeers{}, err
		}
		ref = sess
	}
	out := protocol.MsgPeers{ProjectID: ref.ProjectID, ProjectName: s.projectName(ref.ProjectID, ref.ProjectName), Peers: []protocol.MsgPeer{}, Enabled: s.messagesEnabled()}
	for _, sess := range s.projectSessions(ref.ProjectID) {
		p := s.msgPeer(sess, caller)
		if caller != nil && sess.ID == caller.ID {
			p.SameFolder = false
			out.Self = &p
			continue
		}
		out.Peers = append(out.Peers, p)
	}
	return out, nil
}

// resolveTo finds the recipients of a message from (nil: the user) in
// projectID: a session id, a display name unique in the project (any
// case), or "all" (every other session except shells).
func (s *Server) resolveTo(from *session.Session, projectID, to string) ([]*session.Session, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to}, "say who the message is for (a session id, a name or all)")
	}
	if sess, err := s.lookup(to); err == nil {
		if _, exited := sess.ExitCode(); exited && !sess.Suspended() {
			return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to}, "session %s has exited", to)
		}
		if projectID != "" && sess.ProjectID != projectID {
			return nil, apierr.New(apierr.MsgOtherProject, map[string]any{"to": to},
				"session %s is in another project: messages stay within a project", to)
		}
		if from != nil && sess.ID == from.ID {
			return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to}, "that is this session")
		}
		return []*session.Session{sess}, nil
	}
	if projectID == "" {
		return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to}, "no session %q", to)
	}
	var out []*session.Session
	for _, sess := range s.projectSessions(projectID) {
		if from != nil && sess.ID == from.ID {
			continue
		}
		switch {
		case strings.EqualFold(to, "all"):
			// Shells never read messages: a broadcast would sit in their
			// Messages tab for good. Name one to write to it.
			if !isShell(sess) {
				out = append(out, sess)
			}
		case strings.EqualFold(s.displayName(sess), to):
			out = append(out, sess)
		}
	}
	switch {
	case len(out) == 0 && strings.EqualFold(to, "all"):
		return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to}, "there are no other agent sessions in this project (all skips shells)")
	case len(out) == 0:
		return nil, apierr.New(apierr.MsgNoSession, map[string]any{"to": to},
			"no session of this project is called %q (agenterm msg peers lists them)", to)
	case len(out) > 1 && !strings.EqualFold(to, "all"):
		ids := make([]string, len(out))
		for i, sess := range out {
			ids[i] = sess.ID
		}
		return nil, apierr.New(apierr.MsgAmbiguous, map[string]any{"to": to, "ids": ids},
			"%d sessions are called %q: use the id (%s)", len(out), to, strings.Join(ids, ", "))
	}
	return out, nil
}

func (s *Server) msgSend(caller *session.Session, p protocol.MsgSendParams) (protocol.MsgList, error) {
	if !s.messagesEnabled() {
		return protocol.MsgList{}, apierr.New(apierr.MsgDisabled, nil, "messages between sessions are off (Settings › Messages)")
	}
	body := strings.TrimSpace(p.Body)
	if body == "" {
		return protocol.MsgList{}, mailbox.ErrEmpty
	}
	projectID, from, fromName := p.ProjectID, mailbox.User, ""
	if caller != nil {
		projectID, from, fromName = caller.ProjectID, caller.ID, s.displayName(caller)
	}
	to, err := s.resolveTo(caller, projectID, p.To)
	if err != nil {
		return protocol.MsgList{}, err
	}
	out := protocol.MsgList{Messages: []mailbox.Message{}}
	for _, sess := range to {
		m, err := s.mail.Add(mailbox.Message{ProjectID: sess.ProjectID, From: from, FromName: fromName,
			To: sess.ID, ToName: s.displayName(sess), Body: body})
		if err != nil {
			return out, err
		}
		out.Messages = append(out.Messages, m)
		s.log.Info("message sent", "id", m.ID, "from", from, "to", sess.ID, "bytes", len(m.Body))
		s.publishMessages(sess.ProjectID, sess.ID)
	}
	for _, sess := range to {
		if caller == nil {
			s.userActed(sess.ID) // a message from the user starts a new chain
		}
		s.scheduleAutoDeliver(sess.ID)
	}
	if caller != nil {
		s.publishMessages(caller.ProjectID, caller.ID)
	}
	return out, nil
}

func (s *Server) msgTake(caller *session.Session, p protocol.MsgTakeParams) (protocol.MsgTakeResult, error) {
	if caller == nil {
		return protocol.MsgTakeResult{}, apierr.New(apierr.MsgNotInSession, nil,
			"only a session can read its messages: run it inside an agenterm session")
	}
	res := protocol.MsgTakeResult{Messages: []mailbox.Message{}, Enabled: s.messagesEnabled(), Self: caller.ID}
	if p.Intro {
		if proj, err := s.projects.Get(caller.ProjectID); err == nil {
			res.Instructions = s.sessionInstructions(proj)
		}
	}
	if !res.Enabled {
		return res, nil
	}
	if p.Peek {
		res.Messages = s.mail.Peek(caller.ID)
		if p.Intro {
			if peers, err := s.msgPeers(caller, ""); err == nil {
				res.Peers = &peers
			}
		}
		return res, nil
	}
	ms, err := s.mail.Take(caller.ID)
	if err != nil {
		return res, err
	}
	if len(ms) > 0 {
		res.Messages = ms
		s.log.Info("messages delivered", "to", caller.ID, "count", len(ms))
		s.publishMessages(caller.ProjectID, caller.ID)
		for _, from := range senders(ms) {
			s.publishMessages(caller.ProjectID, from)
		}
	}
	if p.Intro {
		if peers, err := s.msgPeers(caller, ""); err == nil {
			res.Peers = &peers
		}
	}
	return res, nil
}

// senders are the distinct session senders of ms.
func senders(ms []mailbox.Message) []string {
	var out []string
	for _, m := range ms {
		if m.From != mailbox.User && !slices.Contains(out, m.From) {
			out = append(out, m.From)
		}
	}
	return out
}

// shells are commands that would run the nudge line as a command.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "tcsh": true, "csh": true, "nu": true}

func isShell(sess *session.Session) bool {
	return sess.ProfileID == "shell" || shells[filepath.Base(sess.Cmd)]
}

// nudgeGap separates the nudge's text from its Enter, so an agent's TUI
// takes them as typing and a submit rather than one pasted chunk.
const nudgeGap = 150 * time.Millisecond

// errNotAgent: msg.deliver to a shell.
var errNotAgent = errors.New("this session is a shell: read its messages in the app")

// errNudgeInterrupted: input arrived between the nudge line and its Enter,
// so the Enter was not sent (it would send the user's text with the line).
var errNudgeInterrupted = errors.New("you typed while the line was going in: it was left unsent in the terminal")

// errNotIdle: msg.deliver to a session that is not at its prompt.
var errNotIdle = errors.New("the session is busy or waiting for an answer: deliver when it is idle")

// hookDelivers reports whether profileID's hooks hand messages over.
func (s *Server) hookDelivers(profileID string) bool {
	ps, err := s.profiles.List()
	if err != nil {
		return false
	}
	p, ok := profile.Find(ps, profileID)
	return ok && p.SetupHooks == hooks.AgentClaude
}

func (s *Server) msgDeliver(caller *session.Session, id string) (map[string]any, error) {
	if caller != nil {
		return nil, apierr.New(apierr.MsgFromSession, nil, "only the user can nudge a session, not another session")
	}
	sess, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	if !s.messagesEnabled() {
		return nil, apierr.New(apierr.MsgDisabled, nil, "messages between sessions are off (Settings › Messages)")
	}
	n := s.mail.Pending(sess.ID)
	if n == 0 {
		return map[string]any{"pending": 0}, nil
	}
	if isShell(sess) {
		return nil, errNotAgent
	}
	// Only at its prompt: a permission dialog or a question would take
	// the line, and its Enter, as an answer.
	if sess.State() != status.Idle {
		return nil, errNotIdle
	}
	// Text typed at its prompt and not sent: the line would join it and
	// its Enter send both (the automatic delivery holds back for the same
	// reason). Sending or clearing it lets the message through.
	if draft, _ := sess.Draft(); draft {
		return nil, apierr.New(apierr.MsgUnsentInput, nil,
			"the session's prompt has text you haven't sent: send or clear it, then deliver")
	}
	s.userActed(sess.ID)
	l := s.nudgeLockFor(sess.ID)
	l.Lock()
	defer l.Unlock()
	// An automatic nudge just went in: a second line would garble the
	// first (it is still idle until the agent reports its turn).
	if time.Since(l.last) < autoQuiet {
		return map[string]any{"pending": n}, nil
	}
	if err := s.nudge(sess); err != nil {
		return nil, err
	}
	l.last = time.Now()
	s.log.Info("messages nudged", "to", sess.ID, "pending", n)
	s.noteNudged(sess)
	return map[string]any{"pending": n}, nil
}

// nudge types the fixed "you have messages" line into sess and submits it.
func (s *Server) nudge(sess *session.Session) error {
	st, _ := settings.Load(s.cfg.Paths.Settings())
	// Agents whose hooks hand messages over (Claude) see them as soon as
	// the line is submitted; the others are told to read them.
	key := i18n.MsgNudgeRead
	if s.hookDelivers(sess.ProfileID) {
		key = i18n.MsgNudgeHook
	}
	line := i18n.T(st.UI.Lang(), key)
	if _, err := sess.Write([]byte(line)); err != nil {
		return err
	}
	_, typed := sess.Draft()
	time.Sleep(nudgeGap)
	// Someone typed meanwhile: their text follows the line, and an Enter
	// would send it with the line. Leave both there for them to see.
	if _, at := sess.Draft(); !at.Equal(typed) {
		s.log.Info("nudge left unsent: input arrived meanwhile", "to", sess.ID)
		return errNudgeInterrupted
	}
	_, err := sess.Write([]byte("\r"))
	return err
}

func (s *Server) publishMessages(projectID, sessionID string) {
	s.bus.publish(protocol.Event{Type: protocol.EventMessages, SessionID: sessionID, ProjectID: projectID,
		MsgPending: s.mail.Pending(sessionID), MsgHold: s.msgHold(sessionID), At: time.Now().UTC()})
}

// forgetMessages drops what was still waiting for a session that is gone
// for good (delivered messages stay as the project's history).
func (s *Server) forgetMessages(sess *session.Session) {
	s.forgetAuto(sess.ID)
	if n, err := s.mail.DropTo(sess.ID); err != nil {
		s.log.Warn("messages", "err", err)
	} else if n > 0 {
		s.log.Info("pending messages dropped", "to", sess.ID, "count", n)
		s.publishMessages(sess.ProjectID, sess.ID)
	}
}
