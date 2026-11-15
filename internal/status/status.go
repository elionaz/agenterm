// Package status tracks what an agent session is doing.
//
// A Machine combines two sources of truth:
//
//   - explicit events (from `agenterm notify`, typically wired to agent hooks);
//   - a heuristic detector: output activity means running, and no output for
//     IdleAfter means idle.
//
// For profiles in ModeHooks the heuristic is only a fallback: once the first
// explicit event arrives, output activity and idle timeouts no longer change
// the state. Exited is terminal in every mode.
package status

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// State is the observable state of a session.
type State string

const (
	Running      State = "running"
	WaitingInput State = "waiting_input"
	Idle         State = "idle"
	Exited       State = "exited"
)

// ParseState accepts canonical names plus the aliases used by agent hooks
// ("waiting", "done").
func ParseState(s string) (State, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "running", "busy", "working":
		return Running, nil
	case "waiting_input", "waiting", "input":
		return WaitingInput, nil
	case "idle", "done", "stop", "stopped":
		return Idle, nil
	case "exited":
		return Exited, nil
	}
	return "", fmt.Errorf("unknown state %q", s)
}

// Mode selects how much the heuristic is trusted.
type Mode string

const (
	ModeHeuristic Mode = "heuristic"
	ModeHooks     Mode = "hooks"
)

// ParseMode defaults to ModeHeuristic for empty input.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case "", ModeHeuristic:
		return ModeHeuristic, nil
	case ModeHooks:
		return ModeHooks, nil
	}
	return "", fmt.Errorf("unknown status mode %q", s)
}

// Source says what triggered a transition.
type Source string

const (
	SourceHeuristic Source = "heuristic"
	SourceEvent     Source = "event"
	SourceExit      Source = "exit"
)

// Change describes a transition.
type Change struct {
	From, To State
	Source   Source
	At       time.Time
	ExitCode int // only meaningful when To == Exited
}

// DefaultIdleAfter is the quiet period after which a running session is idle.
const DefaultIdleAfter = 3 * time.Second

// Options configures a Machine.
type Options struct {
	Mode      Mode
	IdleAfter time.Duration    // 0 means DefaultIdleAfter
	Now       func() time.Time // injectable clock; nil means time.Now
	// OnChange is called synchronously, while the machine's lock is held, so
	// transitions are delivered in order. It must not block or call back into
	// the Machine.
	OnChange func(Change)
}

// Machine is a per-session state machine. It is safe for concurrent use.
type Machine struct {
	mode      Mode
	idleAfter time.Duration
	now       func() time.Time
	onChange  func(Change)

	mu         sync.Mutex
	state      State
	lastOutput time.Time
	hooked     bool // an explicit event has been received
	exitCode   int
}

// New returns a Machine in the Running state.
func New(o Options) *Machine {
	m := &Machine{
		mode:      o.Mode,
		idleAfter: o.IdleAfter,
		now:       o.Now,
		onChange:  o.OnChange,
		state:     Running,
	}
	if m.mode == "" {
		m.mode = ModeHeuristic
	}
	if m.idleAfter <= 0 {
		m.idleAfter = DefaultIdleAfter
	}
	if m.now == nil {
		m.now = time.Now
	}
	m.lastOutput = m.now()
	return m
}

// State returns the current state.
func (m *Machine) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// ExitCode returns the exit code and whether the session has exited.
func (m *Machine) ExitCode() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitCode, m.state == Exited
}

// IdleAfter returns the configured quiet period.
func (m *Machine) IdleAfter() time.Duration { return m.idleAfter }

func (m *Machine) heuristicActive() bool {
	return !(m.mode == ModeHooks && m.hooked)
}

func (m *Machine) set(to State, src Source, at time.Time) {
	if m.state == to {
		return
	}
	c := Change{From: m.state, To: to, Source: src, At: at, ExitCode: m.exitCode}
	m.state = to
	if m.onChange != nil {
		m.onChange(c)
	}
}

// Output records PTY activity.
func (m *Machine) Output() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.lastOutput = now
	if m.state == Exited || !m.heuristicActive() {
		return
	}
	m.set(Running, SourceHeuristic, now)
}

// Tick evaluates the idle timeout. Callers invoke it periodically.
func (m *Machine) Tick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Running || !m.heuristicActive() {
		return
	}
	now := m.now()
	if now.Sub(m.lastOutput) >= m.idleAfter {
		m.set(Idle, SourceHeuristic, now)
	}
}

// Notify applies an explicit event. Exited cannot be set explicitly.
func (m *Machine) Notify(s State) error {
	switch s {
	case Running, WaitingInput, Idle:
	default:
		return fmt.Errorf("state %q cannot be notified", s)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Exited {
		return fmt.Errorf("session already exited")
	}
	m.hooked = true
	now := m.now()
	// Treat an explicit "running" as fresh activity so the heuristic (in
	// heuristic mode) doesn't immediately flip it back to idle.
	if s == Running {
		m.lastOutput = now
	}
	m.set(s, SourceEvent, now)
	return nil
}

// Exit marks the session as exited with code. Further calls are ignored.
func (m *Machine) Exit(code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Exited {
		return
	}
	m.exitCode = code
	m.set(Exited, SourceExit, m.now())
}
