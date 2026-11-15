// Package client talks to the agenterm daemon. It is used by the CLI and is
// meant to be reused by the desktop UI.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/transport"
)

// Client is a daemon client. Each control call uses its own short-lived
// connection, so a Client is safe for concurrent use.
type Client struct {
	Sock      string
	Transport transport.Transport
	// AutoStart, if set, is invoked once when the daemon is unreachable,
	// after which the dial is retried.
	AutoStart func(ctx context.Context) error

	startOnce sync.Once
	startErr  error
	nextID    atomic.Uint64
}

// New returns a client for the socket at sock.
func New(sock string) *Client {
	return &Client{Sock: sock, Transport: transport.Default()}
}

// SocketFromEnv returns $AGENTERM_SOCK if set, else the default socket path.
func SocketFromEnv() (string, error) {
	if s := os.Getenv(protocol.EnvSock); s != "" {
		return s, nil
	}
	p, err := paths.Default()
	if err != nil {
		return "", err
	}
	return p.Socket(), nil
}

// RemoteError is an error reported by the daemon.
type RemoteError struct{ Msg string }

func (e *RemoteError) Error() string { return e.Msg }

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, err := c.Transport.Dial(ctx, c.Sock)
	if err == nil || c.AutoStart == nil {
		if err != nil {
			return nil, fmt.Errorf("connect to daemon at %s: %w", c.Sock, err)
		}
		return conn, nil
	}
	c.startOnce.Do(func() { c.startErr = c.AutoStart(ctx) })
	if c.startErr != nil {
		return nil, fmt.Errorf("start daemon: %w", c.startErr)
	}
	return c.Transport.Dial(ctx, c.Sock)
}

// open dials, sends a request and reads its response, returning the
// connection positioned right after the response line.
func (c *Client) open(ctx context.Context, method string, params, result any) (net.Conn, *bufio.Reader, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	fail := func(err error) (net.Conn, *bufio.Reader, error) {
		stop()
		conn.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, err
	}
	req := protocol.Request{ID: c.nextID.Add(1), Method: method}
	if params != nil {
		if req.Params, err = json.Marshal(params); err != nil {
			return fail(err)
		}
	}
	b, _ := json.Marshal(req)
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return fail(err)
	}
	br := bufio.NewReaderSize(conn, 64*1024)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return fail(fmt.Errorf("read response: %w", err))
	}
	var resp protocol.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fail(fmt.Errorf("decode response: %w", err))
	}
	if resp.Error != "" {
		return fail(&RemoteError{Msg: resp.Error})
	}
	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fail(fmt.Errorf("decode result: %w", err))
		}
	}
	if !stop() {
		conn.Close()
		return nil, nil, ctx.Err()
	}
	return conn, br, nil
}

// Call performs one control request and decodes its result into result
// (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	conn, _, err := c.open(ctx, method, params, result)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Ping checks the daemon is up.
func (c *Client) Ping(ctx context.Context) error {
	return c.Call(ctx, protocol.MethodDaemonPing, nil, nil)
}

// Shutdown asks the daemon to kill all sessions and exit.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.Call(ctx, protocol.MethodDaemonShutdown, nil, nil)
}

func (c *Client) ProjectAdd(ctx context.Context, path, name string) (project.Project, error) {
	var p project.Project
	err := c.Call(ctx, protocol.MethodProjectAdd, protocol.ProjectAddParams{Path: path, Name: name}, &p)
	return p, err
}

func (c *Client) ProjectList(ctx context.Context) ([]project.Project, error) {
	var r protocol.ProjectList
	err := c.Call(ctx, protocol.MethodProjectList, nil, &r)
	return r.Projects, err
}

func (c *Client) ProjectRemove(ctx context.Context, id string) error {
	return c.Call(ctx, protocol.MethodProjectRemove, protocol.ProjectRef{ID: id}, nil)
}

func (c *Client) Profiles(ctx context.Context) ([]profile.Profile, error) {
	var r protocol.ProfileList
	err := c.Call(ctx, protocol.MethodProfileList, nil, &r)
	return r.Profiles, err
}

func (c *Client) Spawn(ctx context.Context, p protocol.SpawnParams) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionSpawn, p, &info)
	return info, err
}

func (c *Client) Sessions(ctx context.Context) ([]protocol.SessionInfo, error) {
	var r protocol.SessionList
	err := c.Call(ctx, protocol.MethodSessionList, nil, &r)
	return r.Sessions, err
}

func (c *Client) Kill(ctx context.Context, id string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionKill, protocol.SessionRef{SessionID: id}, &info)
	return info, err
}

func (c *Client) Resize(ctx context.Context, id string, cols, rows uint16) error {
	return c.Call(ctx, protocol.MethodSessionResize, protocol.ResizeParams{SessionID: id, Cols: cols, Rows: rows}, nil)
}

func (c *Client) Notify(ctx context.Context, id, state string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionNotify, protocol.NotifyParams{SessionID: id, State: state}, &info)
	return info, err
}

// Stream is an attached session. Recv must be called from a single
// goroutine; Input/Resize may be called concurrently with Recv.
type Stream struct {
	Info protocol.SessionInfo
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex
}

// Attach opens a framed stream to a session. cols/rows of 0 keep the
// current size. The context only bounds the handshake; use Close to detach.
func (c *Client) Attach(ctx context.Context, id string, cols, rows uint16) (*Stream, error) {
	st := &Stream{}
	conn, br, err := c.open(ctx, protocol.MethodSessionAttach, protocol.AttachParams{SessionID: id, Cols: cols, Rows: rows}, &st.Info)
	if err != nil {
		return nil, err
	}
	st.conn, st.br = conn, br
	return st, nil
}

// Recv returns the next frame from the daemon.
func (s *Stream) Recv() (protocol.Frame, error) { return protocol.ReadFrame(s.br) }

// Input sends keystrokes / bytes to the PTY.
func (s *Stream) Input(p []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return protocol.WriteChunked(s.conn, protocol.FrameInput, p)
}

// Resize changes the PTY size.
func (s *Stream) Resize(cols, rows uint16) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return protocol.WriteFrame(s.conn, protocol.FrameResize, protocol.ResizePayload(cols, rows))
}

// Close detaches; the session keeps running.
func (s *Stream) Close() error { return s.conn.Close() }

// Events is an events.subscribe stream.
type Events struct {
	conn net.Conn
	br   *bufio.Reader
}

// Subscribe opens an event stream.
func (c *Client) Subscribe(ctx context.Context) (*Events, error) {
	conn, br, err := c.open(ctx, protocol.MethodEventsSubscribe, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Events{conn: conn, br: br}, nil
}

// Next blocks for the next event. io.EOF-like errors mean the daemon closed
// the stream (shutdown or slow consumer): resubscribe and resync.
func (e *Events) Next() (protocol.Event, error) {
	line, err := e.br.ReadBytes('\n')
	if err != nil {
		return protocol.Event{}, err
	}
	var ev protocol.Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return protocol.Event{}, err
	}
	return ev, nil
}

// Close ends the subscription.
func (e *Events) Close() error { return e.conn.Close() }

// IsRemote reports whether err came from the daemon (as opposed to transport).
func IsRemote(err error) bool {
	var re *RemoteError
	return errors.As(err, &re)
}
