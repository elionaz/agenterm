// Package daemon implements agentermd: the long-lived process that owns the
// PTYs and serves control/attach requests over a local socket.
package daemon

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/mailbox"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/repoconfig"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/sessionstore"
	"github.com/elionaz/agenterm/internal/shellenv"
	"github.com/elionaz/agenterm/internal/status"
	"github.com/elionaz/agenterm/internal/termsig"
	"github.com/elionaz/agenterm/internal/transport"
	"github.com/elionaz/agenterm/internal/worktree"
)

// ErrAlreadyRunning is returned by Run when another daemon owns the socket.
var ErrAlreadyRunning = errors.New("daemon already running")

// Config configures a Server.
type Config struct {
	Paths     paths.Paths
	Transport transport.Transport // nil means transport.Default()
	IdleAfter time.Duration       // heuristic idle timeout; 0 means status.DefaultIdleAfter
	RingSize  int                 // per-session scrollback; 0 means session.DefaultRingSize
	KillGrace time.Duration       // SIGTERM -> SIGKILL delay; 0 means 2s
	Logger    *slog.Logger
	// Exe is the binary agent hooks call back (`<Exe> notify --from-hook …`).
	// Empty means this process's own executable, which is right for the CLI
	// and the .app (both implement `notify`).
	Exe string
	// Notifier shows a desktop notification when no UI client is
	// subscribed. nil means none (tests and embedders never post real
	// notifications by accident); the daemon command passes its own.
	Notifier func(n Notification)
	// SearchPath returns the PATH list used to detect installed agents.
	// nil means this process's PATH plus the login shell's (cached; see
	// profile.rescan).
	SearchPath func() string
}

// Notification is what the daemon's fallback notifier displays.
type Notification struct {
	Title, Body string
	Sound       bool
	SessionID   string
	Kind        string
	// Osascript: settings allow the osascript fallback (notify.fallback)
	// when native notifications cannot be used.
	Osascript bool
}

// Server is the daemon.
type Server struct {
	auto     autoDeliver
	trust    *repoconfig.Trust // worktree setup commands the user allowed
	cfg      Config
	sock     string
	projects *project.Registry
	profiles *profile.Store
	detector *profile.Detector
	bus      *bus
	log      *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session.Session
	order    []string
	// susp holds the suspended sessions' records (see persist.go);
	// resuming marks those being resumed.
	susp     map[string]*suspInfo
	resuming map[string]bool

	// names are the sessions' display names (rename.go).
	namesMu sync.Mutex
	names   map[string]string
	models  map[string]string   // the agents' models (model.go), same lock
	ports   map[string]sessPort // each session's port (ports.go), same lock

	// Persistence: the session registry and when each scrollback was
	// last saved (OutputTotal; guarded by persistMu).
	store       *sessionstore.Store
	persistMu   sync.Mutex
	saved       map[string]int64
	stopping    atomic.Bool
	persistStop chan struct{}
	persistDone chan struct{}

	gitState

	// mail holds the messages between sessions (messages.go).
	mail *mailbox.Store

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup

	stopOnce sync.Once
	stop     chan struct{}
	// stopAttrs describe the accepted daemon.shutdown request (peer, force);
	// written before stop is closed.
	stopAttrs []any
}

// New prepares a Server; it does not bind the socket yet.
func New(cfg Config) (*Server, error) {
	if cfg.Transport == nil {
		cfg.Transport = transport.Default()
	}
	if cfg.KillGrace <= 0 {
		cfg.KillGrace = 2 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Exe == "" {
		if exe, err := os.Executable(); err == nil {
			if real, err := filepath.EvalSymlinks(exe); err == nil {
				exe = real
			}
			cfg.Exe = exe
		}
	}
	if cfg.Notifier == nil {
		cfg.Notifier = func(Notification) {}
	}
	if err := cfg.Paths.Ensure(); err != nil {
		return nil, err
	}
	reg, err := project.Open(cfg.Paths.Projects())
	if err != nil {
		return nil, err
	}
	if cfg.SearchPath == nil {
		cfg.SearchPath = func() string { return shellenv.JoinPaths(os.Getenv("PATH"), shellenv.LoginPath()) }
	}
	det := &profile.Detector{SearchPath: cfg.SearchPath}
	wts, err := worktree.Open(cfg.Paths.Worktrees())
	if err != nil {
		return nil, err
	}
	store, err := openStore(cfg.Paths.Sessions(), cfg.Logger)
	if err != nil {
		return nil, err
	}
	// Message history is not worth refusing to start over: an unreadable
	// (or newer) messages.json is left alone and messages live in memory.
	mail, err := mailbox.Open(cfg.Paths.Messages())
	if err != nil {
		cfg.Logger.Warn("messages.json unreadable: messages are kept in memory until the next start", "err", err)
		mail = mailbox.Memory()
	}
	bg, bgCancel := context.WithCancel(context.Background())
	return &Server{
		trust: repoconfig.OpenTrust(cfg.Paths.Trust()),
		gitState: gitState{wts: wts, git: make(map[string]sessGit), gitCache: make(map[string]*gitEntry), fetched: make(map[string]time.Time),
			bg: bg, bgCancel: bgCancel},
		cfg:      cfg,
		sock:     cfg.Paths.Socket(),
		projects: reg,
		profiles: &profile.Store{Path: cfg.Paths.Profiles(), Look: det.Lookup},
		detector: det,
		bus:      newBus(),
		log:      cfg.Logger,
		sessions: make(map[string]*session.Session),
		susp:     make(map[string]*suspInfo),
		resuming: make(map[string]bool),
		names:    make(map[string]string),
		models:   make(map[string]string),
		ports:    make(map[string]sessPort),
		store:    store,
		mail:     mail,
		saved:    make(map[string]int64),
		conns:    make(map[net.Conn]struct{}),
		stop:     make(chan struct{}),
	}, nil
}

// homeEnv is the AGENTERM_HOME sessions get: the daemon's own single-dir
// home (AGENTERM_HOME layout), or none for the default layout. Clients
// prefer AGENTERM_HOME over AGENTERM_SOCK, so both must name this daemon.
func (s *Server) homeEnv() string {
	if p := s.cfg.Paths; p.RuntimeDir == p.ConfigDir {
		return p.RuntimeDir
	}
	return ""
}

// Socket returns the address the server listens on.
func (s *Server) Socket() string { return s.sock }

// Run serves until ctx is cancelled or daemon.shutdown is called, then kills
// all sessions and cleans up the socket and pid file.
func (s *Server) Run(ctx context.Context) error {
	t := s.cfg.Transport
	if transport.Alive(t, s.sock) {
		return fmt.Errorf("%w on %s", ErrAlreadyRunning, s.sock)
	}
	// Whatever is there is stale (previous crash): remove it.
	if err := t.Cleanup(s.sock); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	ln, err := t.Listen(s.sock)
	if err != nil {
		return err
	}
	pidFile := s.cfg.Paths.PidFile()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		ln.Close()
		return err
	}
	s.log.Info("daemon listening", "socket", s.sock, "pid", os.Getpid())
	go s.detector.Path() // warm the (login shell) search PATH
	s.restoreSessions()
	s.persistStop, s.persistDone = make(chan struct{}), make(chan struct{})
	go s.persistLoop(s.persistStop, s.persistDone)

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.trackConn(conn, true)
			s.wg.Go(func() {
				defer s.trackConn(conn, false)
				defer conn.Close()
				s.serveConn(conn)
			})
		}
	}()

	var attrs []any
	select {
	case <-ctx.Done():
		reason := "context cancelled"
		if c := context.Cause(ctx); c != nil && c != context.Canceled {
			reason = c.Error()
		}
		attrs = []any{"reason", reason}
	case <-s.stop:
		attrs = append([]any{"reason", "daemon.shutdown RPC"}, s.stopAttrs...)
	}
	s.log.Info("daemon shutting down", attrs...)
	// Records are final before the socket stops answering: `stop` returns
	// then, and a new daemon may read sessions.json right away.
	s.suspendAll()
	ln.Close()
	<-acceptDone
	s.killAll()
	s.stopGit()
	s.bus.closeAll()
	s.connMu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.connMu.Unlock()
	s.wg.Wait()
	// No t.Cleanup(s.sock) here: ln.Close above already unlinked the socket
	// (net.UnixListener), and by now the path may belong to the daemon that
	// replaced this one (restart), whose socket removing would orphan it.
	if b, err := os.ReadFile(pidFile); err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
		os.Remove(pidFile)
	}
	return nil
}

func (s *Server) trackConn(c net.Conn, add bool) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if add {
		s.conns[c] = struct{}{}
	} else {
		delete(s.conns, c)
	}
}

func (s *Server) killAll() {
	s.mu.Lock()
	list := make([]*session.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		list = append(list, sess)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sess := range list {
		wg.Go(func() { sess.Kill(s.cfg.KillGrace) })
	}
	wg.Wait()
}

// serveConn handles sequential control requests until the peer closes the
// connection or a streaming method takes it over.
func (s *Server) serveConn(conn net.Conn) {
	br := bufio.NewReaderSize(conn, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var req protocol.Request
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(conn, 0, nil, fmt.Errorf("bad request: %w", err))
			return
		}
		switch req.Method {
		case protocol.MethodDaemonShutdown:
			// Needs the connection (to identify the peer).
			result, err := s.handleShutdown(conn, req)
			if werr := writeResponse(conn, req.ID, result, err); werr != nil {
				return
			}
			continue
		case protocol.MethodMsgPeers, protocol.MethodMsgSend, protocol.MethodMsgList, protocol.MethodMsgTake, protocol.MethodMsgDeliver, protocol.MethodMsgAck:
			// Need the connection (the caller is who sends or reads).
			result, err := s.handleMsg(conn, req)
			if werr := writeResponse(conn, req.ID, result, err); werr != nil {
				return
			}
			continue
		case protocol.MethodSessionAttach:
			s.handleAttach(conn, br, req)
			return
		case protocol.MethodEventsSubscribe:
			s.handleSubscribe(conn, br, req)
			return
		}
		result, err := s.dispatch(req)
		if werr := writeResponse(conn, req.ID, result, err); werr != nil {
			return
		}
	}
}

func writeResponse(conn net.Conn, id uint64, result any, err error) error {
	resp := protocol.Response{ID: id}
	if err != nil {
		resp.Error = err.Error()
		if code, data, ok := apierr.Code(err); ok {
			resp.ErrorCode = code
			if data != nil {
				resp.ErrorData, _ = json.Marshal(data)
			}
		}
	} else if result != nil {
		b, merr := json.Marshal(result)
		if merr != nil {
			resp.Error = merr.Error()
		} else {
			resp.Result = b
		}
	}
	b, _ := json.Marshal(resp)
	_, werr := conn.Write(append(b, '\n'))
	return werr
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("bad params: %w", err)
	}
	return v, nil
}

func newID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Server) lookup(id string) (*session.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return sess, nil
}

// sessionInfo describes sess, with its git context and cached GitInfo.
func (s *Server) sessionInfo(sess *session.Session) protocol.SessionInfo {
	info := sessionInfo(sess)
	info.Name = s.sessionName(sess.ID)
	info.ProjectName = s.projectName(sess.ProjectID, sess.ProjectName)
	info.MsgPending = s.mail.Pending(sess.ID)
	info.Model = s.sessionModel(sess.ID)
	if p := s.sessionPort(sess.ID); len(p.Env) > 0 {
		info.Port = p.Port // the project asked for ports: show it
	}
	info.Permission = permissionMode(info.Cmd, info.Args)
	if g := s.sessionGit(sess.ID); g != nil {
		info.Mode, info.WorktreeID, info.Branch, info.Base = g.Mode, g.WorktreeID, g.Branch, g.Base
		if g.Mode == protocol.ModeWorktree {
			info.WorktreePath = g.Dir
		}
		info.Git = s.cachedGit(sess.ID)
	}
	if sess.Suspended() {
		s.suspendedFields(&info)
	}
	return info
}

func sessionInfo(sess *session.Session) protocol.SessionInfo {
	cols, rows := sess.Size()
	info := protocol.SessionInfo{
		ID:          sess.ID,
		ProjectID:   sess.ProjectID,
		ProjectDir:  sess.ProjectDir,
		ProjectName: sess.ProjectName,
		ProfileID:   sess.ProfileID,
		Cmd:         sess.Cmd,
		Args:        sess.Args,
		Cwd:         sess.Cwd,
		Pid:         sess.Pid,
		CreatedAt:   sess.CreatedAt,
		State:       string(sess.State()),
		Cols:        cols,
		Rows:        rows,
	}
	if code, exited := sess.ExitCode(); exited {
		info.ExitCode = &code
	}
	a := sess.Attention()
	info.AttentionKind, info.AttentionMessage, info.AttentionAt, info.Unread = attentionFields(a)
	info.Live = liveInfo(sess.Live(), sess.Timers())
	return info
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// liveInfo converts a session's activity and timers to the wire format.
func liveInfo(l session.Live, t status.Timers) protocol.Live {
	out := timersInfo(t)
	out.Activity, out.ActivitySource, out.ActivityAt = l.Activity, l.ActivitySource, timePtr(l.ActivityAt)
	out.ActivityKind, out.ActivityTarget = l.ActivityKind, l.ActivityTarget
	if l.Progress.State != termsig.ProgressNone {
		out.Progress = &protocol.Progress{State: l.Progress.State.String(), Value: l.Progress.Value}
	}
	return out
}

func timersInfo(t status.Timers) protocol.Live {
	return protocol.Live{
		TurnStartedAt: timePtr(t.TurnStartedAt),
		TurnEndedAt:   timePtr(t.TurnEndedAt),
		LastOutputAt:  timePtr(t.LastOutputAt),
		LastEventAt:   timePtr(t.LastEventAt),
		Stalled:       t.Stalled,
		StalledAt:     timePtr(t.StalledAt),
	}
}

// publishActivity emits a coalesced session.activity event and, when the
// session just stalled and no UI is listening, a quiet fallback
// notification.
func (s *Server) publishActivity(sess *session.Session, stallRaised bool) {
	ev := protocol.Event{
		Type:        protocol.EventSessionActivity,
		SessionID:   sess.ID,
		State:       string(sess.State()),
		At:          time.Now().UTC(),
		ProjectName: s.projectName(sess.ProjectID, sess.ProjectName),
		ProfileID:   sess.ProfileID,
		SessionName: s.sessionName(sess.ID),
		StallRaised: stallRaised,
		Live:        liveInfo(sess.Live(), sess.Timers()),
	}
	s.bus.publish(ev)
	if stallRaised && s.bus.uiCount() == 0 {
		s.fallbackNotify(stallEvent(ev))
	}
}

// stallEvent shapes a stall for the notification path (kind "stall"; the
// body is rendered at notify time, in the configured language).
func stallEvent(ev protocol.Event) protocol.Event {
	ev.AttentionKind = "stall"
	return ev
}

func attentionFields(a status.Attention) (kind, msg string, at *time.Time, unread bool) {
	if !a.Active() {
		return "", "", nil, false
	}
	t := a.At.UTC()
	return string(a.Kind), a.Message, &t, a.Unread
}

// publishState turns a status change into an event and, when a new
// attention was raised and no UI is listening, a fallback notification.
func (s *Server) publishState(id, projectName, profileID string, c status.Change) {
	ev := protocol.Event{
		Type:        protocol.EventSessionState,
		SessionID:   id,
		State:       string(c.To),
		PrevState:   string(c.From),
		Source:      string(c.Source),
		At:          c.At,
		Raised:      c.Raised,
		ProjectName: projectName,
		ProfileID:   profileID,
		SessionName: s.sessionName(id),
	}
	if c.From == c.To {
		ev.Type = protocol.EventSessionAttention
	}
	ev.AttentionKind, ev.AttentionMessage, ev.AttentionAt, ev.Unread = attentionFields(c.Attention)
	ev.Live = timersInfo(c.Timers)
	if c.To == status.Exited {
		code := c.ExitCode
		ev.ExitCode = &code
	}
	s.bus.publish(ev)
	// Messages that came while it worked; not after a failed turn (an API
	// error: rate limit, overload), which a nudge would only repeat.
	if c.To == status.Idle && c.From != status.Idle && ev.AttentionKind != string(status.KindError) {
		s.scheduleAutoDeliver(id)
	}
	if ev.Raised && s.bus.uiCount() == 0 {
		s.fallbackNotify(ev)
	}
	// The turn ended (or the agent waits): a good moment to look at git.
	if c.From != c.To && c.To != status.Running {
		s.requestGit(id)
	}
}
