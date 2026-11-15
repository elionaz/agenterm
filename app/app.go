package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/transport"
)

// Output batching: one `pty:<id>` event per window or per chunk.
const (
	flushDelay = 8 * time.Millisecond
	flushBytes = 32 * 1024
	callTO     = 10 * time.Second
)

// Event names emitted to the frontend. Per-session events are suffixed
// with ":<sessionId>".
const (
	evPty        = "pty"             // base64 output chunk
	evReplayDone = "pty-replay-done" // scrollback replay finished
	evExit       = "pty-exit"        // exit code (number)
	evError      = "pty-error"       // stream ended unexpectedly (message)
	evDaemon     = "daemon:event"    // protocol.Event
	evResync     = "daemon:resync"   // (re)subscribed: reload lists
	evDown       = "daemon:down"     // event stream lost (message)
)

// App is bound to the frontend. It is a thin proxy over the daemon client:
// the daemon owns the PTYs, so quitting the app leaves sessions running.
type App struct {
	ctx    context.Context
	cancel context.CancelFunc
	paths  paths.Paths
	client *client.Client
	emit   func(event string, data ...any) // runtime.EventsEmit; swapped in tests

	startMu sync.Mutex // serialises daemon auto-start

	mu      sync.Mutex
	streams map[string]*attachment
}

type attachment struct {
	stream *client.Stream
	out    *coalescer
	mu     sync.Mutex
	closed bool // set on intentional Detach: suppress error events
}

func NewApp() *App {
	return &App{streams: make(map[string]*attachment)}
}

func (a *App) startup(ctx context.Context) {
	p, err := paths.Default()
	if err != nil {
		log.Printf("agenterm: resolve paths: %v", err)
	}
	a.emit = func(event string, data ...any) { runtime.EventsEmit(ctx, event, data...) }
	a.start(ctx, p)
}

func (a *App) start(ctx context.Context, p paths.Paths) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.paths = p
	a.client = client.New(p.Socket())
	go a.eventLoop()
}

// shutdown detaches everything. It deliberately does not stop the daemon:
// sessions must survive the window.
func (a *App) shutdown(context.Context) {
	a.cancel()
	a.mu.Lock()
	streams := a.streams
	a.streams = make(map[string]*attachment)
	a.mu.Unlock()
	for _, at := range streams {
		at.close()
	}
}

// ensureDaemon starts the daemon from this very executable if it is not
// reachable, so the .app does not need a separately installed CLI.
func (a *App) ensureDaemon(ctx context.Context) error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	if transport.Alive(transport.Default(), a.paths.Socket()) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	prepareDaemonEnv()
	return client.StartDaemon(ctx, exe, a.paths)
}

// call runs fn with a timeout, starting the daemon first if needed.
func (a *App) call(fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(a.ctx, callTO)
	defer cancel()
	if err := a.ensureDaemon(ctx); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	return fn(ctx)
}

// eventLoop forwards daemon events, resubscribing (and restarting the daemon
// if necessary) whenever the stream drops.
func (a *App) eventLoop() {
	backoff := 250 * time.Millisecond
	for a.ctx.Err() == nil {
		var ev *client.Events
		err := a.call(func(ctx context.Context) error {
			var err error
			ev, err = a.client.Subscribe(ctx)
			return err
		})
		if err != nil {
			a.emit(evDown, err.Error())
			select {
			case <-a.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 5*time.Second)
			continue
		}
		backoff = 250 * time.Millisecond
		stop := context.AfterFunc(a.ctx, func() { ev.Close() })
		a.emit(evResync)
		for {
			e, err := ev.Next()
			if err != nil {
				break
			}
			a.emit(evDaemon, e)
		}
		stop()
		ev.Close()
	}
}

// --- Projects & profiles ----------------------------------------------------

func (a *App) ListProjects() ([]project.Project, error) {
	var out []project.Project
	err := a.call(func(ctx context.Context) (err error) {
		out, err = a.client.ProjectList(ctx)
		return err
	})
	return nonNil(out), err
}

// AddProject opens the native folder picker. It returns nil if cancelled.
func (a *App) AddProject() (*project.Project, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Añadir proyecto",
		CanCreateDirectories: true,
	})
	if err != nil || dir == "" {
		return nil, err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, err
	}
	var p project.Project
	err = a.call(func(ctx context.Context) (err error) {
		p, err = a.client.ProjectAdd(ctx, dir, "")
		return err
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *App) RemoveProject(id string) error {
	return a.call(func(ctx context.Context) error { return a.client.ProjectRemove(ctx, id) })
}

func (a *App) ListProfiles() ([]profile.Profile, error) {
	var out []profile.Profile
	err := a.call(func(ctx context.Context) (err error) {
		out, err = a.client.Profiles(ctx)
		return err
	})
	return nonNil(out), err
}

// --- Sessions ---------------------------------------------------------------

func (a *App) ListSessions() ([]protocol.SessionInfo, error) {
	var out []protocol.SessionInfo
	err := a.call(func(ctx context.Context) (err error) {
		out, err = a.client.Sessions(ctx)
		return err
	})
	return nonNil(out), err
}

func (a *App) Spawn(projectID, profileID string, cols, rows int) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := a.call(func(ctx context.Context) (err error) {
		info, err = a.client.Spawn(ctx, protocol.SpawnParams{
			ProjectID: projectID, ProfileID: profileID, Cols: clampDim(cols), Rows: clampDim(rows),
		})
		return err
	})
	return info, err
}

// Kill terminates a running session, or forgets an exited one.
func (a *App) Kill(id string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := a.call(func(ctx context.Context) (err error) {
		info, err = a.client.Kill(ctx, id)
		return err
	})
	return info, err
}

// Attach (re)opens the output stream of a session. The frontend must listen
// to `pty:<id>` before calling it: the daemon replays the scrollback first,
// then `pty-replay-done:<id>`, then live output. Attaching an already
// attached session replaces the stream (and replays again), which is what a
// reloaded frontend needs.
func (a *App) Attach(id string, cols, rows int) (protocol.SessionInfo, error) {
	a.Detach(id)
	var st *client.Stream
	err := a.call(func(ctx context.Context) (err error) {
		st, err = a.client.Attach(ctx, id, clampDim(cols), clampDim(rows))
		return err
	})
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	ptyEvent := evPty + ":" + id
	at := &attachment{stream: st}
	at.out = newCoalescer(flushDelay, flushBytes, func(b []byte) {
		a.emit(ptyEvent, base64.StdEncoding.EncodeToString(b))
	})
	a.mu.Lock()
	if old := a.streams[id]; old != nil { // lost a race with another Attach
		old.close()
	}
	a.streams[id] = at
	a.mu.Unlock()
	go a.pump(id, at)
	return st.Info, nil
}

// pump reads frames until the stream ends, translating them to events.
func (a *App) pump(id string, at *attachment) {
	defer a.forget(id, at)
	for {
		f, err := at.stream.Recv()
		if err != nil {
			at.out.Flush()
			if !at.isClosed() {
				a.emit(evError+":"+id, "disconnected from daemon: "+err.Error())
			}
			return
		}
		switch f.Type {
		case protocol.FrameOutput:
			at.out.Write(f.Payload)
		case protocol.FrameReplayDone:
			at.out.Flush()
			if !at.isClosed() {
				a.emit(evReplayDone + ":" + id)
			}
		case protocol.FrameExit:
			at.out.Flush()
			code, _ := protocol.ParseExit(f.Payload)
			if !at.isClosed() {
				a.emit(evExit+":"+id, code)
			}
			return
		case protocol.FrameError:
			at.out.Flush()
			if !at.isClosed() {
				a.emit(evError+":"+id, string(f.Payload))
			}
			return
		}
	}
}

// Detach closes the output stream; the session keeps running.
func (a *App) Detach(id string) {
	a.mu.Lock()
	at := a.streams[id]
	delete(a.streams, id)
	a.mu.Unlock()
	if at != nil {
		at.close()
	}
}

func (a *App) forget(id string, at *attachment) {
	at.out.Stop()
	at.stream.Close()
	a.mu.Lock()
	if a.streams[id] == at {
		delete(a.streams, id)
	}
	a.mu.Unlock()
}

// Input writes UTF-8 text (keystrokes, pastes) to the session.
func (a *App) Input(id, data string) error {
	return a.input(id, []byte(data))
}

// InputBase64 writes raw bytes (xterm's onBinary, e.g. legacy mouse reports).
func (a *App) InputBase64(id, b64 string) error {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return a.input(id, b)
}

func (a *App) input(id string, b []byte) error {
	at := a.attached(id)
	if at == nil {
		return errors.New("session not attached")
	}
	return at.stream.Input(b)
}

func (a *App) Resize(id string, cols, rows int) error {
	c, r := clampDim(cols), clampDim(rows)
	if c == 0 || r == 0 {
		return nil
	}
	if at := a.attached(id); at != nil {
		return at.stream.Resize(c, r)
	}
	return a.call(func(ctx context.Context) error { return a.client.Resize(ctx, id, c, r) })
}

func (a *App) attached(id string) *attachment {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.streams[id]
}

func (at *attachment) close() {
	at.mu.Lock()
	at.closed = true
	at.mu.Unlock()
	at.out.Stop()
	at.stream.Close()
}

func (at *attachment) isClosed() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.closed
}

func clampDim(n int) uint16 {
	if n <= 0 {
		return 0
	}
	return uint16(min(n, 1000))
}

// nonNil makes empty lists serialise as [] instead of null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
