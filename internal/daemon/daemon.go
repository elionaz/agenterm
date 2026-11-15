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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/status"
	"github.com/elionaz/agenterm/internal/transport"
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
}

// Server is the daemon.
type Server struct {
	cfg      Config
	sock     string
	projects *project.Registry
	bus      *bus
	log      *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session.Session
	order    []string

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
	wg     sync.WaitGroup

	stopOnce sync.Once
	stop     chan struct{}
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
	if err := cfg.Paths.Ensure(); err != nil {
		return nil, err
	}
	reg, err := project.Open(cfg.Paths.Projects())
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		sock:     cfg.Paths.Socket(),
		projects: reg,
		bus:      newBus(),
		log:      cfg.Logger,
		sessions: make(map[string]*session.Session),
		conns:    make(map[net.Conn]struct{}),
		stop:     make(chan struct{}),
	}, nil
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

	select {
	case <-ctx.Done():
	case <-s.stop:
	}
	s.log.Info("daemon shutting down")
	ln.Close()
	<-acceptDone
	s.killAll()
	s.bus.closeAll()
	s.connMu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.connMu.Unlock()
	s.wg.Wait()
	_ = t.Cleanup(s.sock)
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

func sessionInfo(sess *session.Session) protocol.SessionInfo {
	cols, rows := sess.Size()
	info := protocol.SessionInfo{
		ID:         sess.ID,
		ProjectID:  sess.ProjectID,
		ProjectDir: sess.ProjectDir,
		ProfileID:  sess.ProfileID,
		Cmd:        sess.Cmd,
		Args:       sess.Args,
		Cwd:        sess.Cwd,
		Pid:        sess.Pid,
		CreatedAt:  sess.CreatedAt,
		State:      string(sess.State()),
		Cols:       cols,
		Rows:       rows,
	}
	if code, exited := sess.ExitCode(); exited {
		info.ExitCode = &code
	}
	return info
}

func (s *Server) publishState(id string, c status.Change) {
	ev := protocol.Event{
		Type:      protocol.EventSessionState,
		SessionID: id,
		State:     string(c.To),
		PrevState: string(c.From),
		Source:    string(c.Source),
		At:        c.At,
	}
	if c.To == status.Exited {
		code := c.ExitCode
		ev.ExitCode = &code
	}
	s.bus.publish(ev)
}
