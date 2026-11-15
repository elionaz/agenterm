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
//
// On top of the state, a Machine tracks an Attention: why the session needs
// the user (permission, question, done, error), with an optional message and
// an unread flag cleared by Ack or user input. Attention comes from explicit
// events (waiting_input, and idle after a turn) and from terminal signals
// (BEL / OSC notifications / profile patterns) via Signal. Heuristic idle
// never raises attention: quiet output is not "done".
//
// A Machine also keeps activity timers (turn start/end, last output, last
// explicit event) and a stall flag: a turn opened by an explicit "running"
// event (an agent hook) that then sees neither output nor events for
// StallAfter is marked stalled. Stalled is a flag, not a state; any output or
// event clears it. Heuristic sessions have no notion of an open turn (quiet
// output already means idle), so they never stall.
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
	SourceSignal    Source = "signal" // terminal BEL/OSC notification or attention pattern
	SourceInput     Source = "input"  // the user typed into the session
	SourceAck       Source = "ack"
	SourceExit      Source = "exit"
)

// Kind classifies why a session needs the user.
type Kind string

const (
	KindPermission Kind = "permission" // the agent asks for approval
	KindQuestion   Kind = "question"   // the agent waits for an answer / input
	KindDone       Kind = "done"       // the agent finished its turn
	KindError      Kind = "error"      // the turn ended with an error
)

// ParseKind accepts the canonical kinds; empty input yields "".
func ParseKind(s string) (Kind, error) {
	switch k := Kind(strings.ToLower(strings.TrimSpace(s))); k {
	case "", KindPermission, KindQuestion, KindDone, KindError:
		return k, nil
	case "approval":
		return KindPermission, nil
	case "input", "waiting":
		return KindQuestion, nil
	case "idle", "stop", "finished":
		return KindDone, nil
	}
	return "", fmt.Errorf("unknown attention kind %q", s)
}

// Attention says that (and why) a session needs the user. The zero value
// means "nothing to see". Unread stays true until the user acknowledges it
// (focuses or types into the session).
type Attention struct {
	Kind    Kind
	Message string
	At      time.Time
	Unread  bool
	Source  Source
}

// Active reports whether there is an attention at all.
func (a Attention) Active() bool { return a.Kind != "" }

// Event is an explicit notification (agent hook or `agenterm notify`).
type Event struct {
	State   State
	Kind    Kind   // optional; derived from State when empty
	Message string // optional, shown to the user
	// Turn marks the start of a new turn (the user submitted a prompt): the
	// turn timer restarts even if the session was already running.
	Turn bool
	// Activity is a short description of what the agent is doing ("Leyendo
	// main.go"). The Machine ignores it; sessions keep it for display.
	Activity string
}

// Timers are the activity timestamps of a session. Zero values mean
// "never" (TurnEndedAt is zero while a turn is in progress).
type Timers struct {
	TurnStartedAt time.Time
	TurnEndedAt   time.Time
	LastOutputAt  time.Time
	LastEventAt   time.Time
	Stalled       bool
	StalledAt     time.Time
}

// Change describes a transition. Attention-only changes (a message filled
// in, an ack) have From == To.
type Change struct {
	From, To  State
	Source    Source
	At        time.Time
	ExitCode  int       // only meaningful when To == Exited
	Attention Attention // attention after the change
	// Raised is true when a new attention was raised by this change: the
	// signal UIs use to notify the user.
	Raised bool
	Timers Timers // timers after the change
}

// DoneGrace is how long a fresh explicit "idle/done" resists output in
// heuristic mode.
const DoneGrace = 2 * time.Second

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
	// StallAfter is how long an open turn may go without output or events
	// before it is flagged as stalled; 0 disables stall detection.
	StallAfter time.Duration
	// OnStall is called (like OnChange: lock held, must not block or call
	// back) whenever the stalled flag flips.
	OnStall func(stalled bool)
}

// Machine is a per-session state machine. It is safe for concurrent use.
type Machine struct {
	mode       Mode
	idleAfter  time.Duration
	now        func() time.Time
	onChange   func(Change)
	stallAfter time.Duration
	onStall    func(bool)

	mu         sync.Mutex
	state      State
	lastOutput time.Time // heuristic clock (explicit "running" also resets it)
	hooked     bool      // an explicit event has been received
	exitCode   int
	att        Attention
	t          Timers
	turnOpen   bool // running because of an explicit event (hook)
}

// New returns a Machine in the Running state.
func New(o Options) *Machine {
	m := &Machine{
		mode:       o.Mode,
		idleAfter:  o.IdleAfter,
		now:        o.Now,
		onChange:   o.OnChange,
		state:      Running,
		stallAfter: o.StallAfter,
		onStall:    o.OnStall,
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
	m.t.TurnStartedAt = m.lastOutput
	return m
}

// Timers returns the activity timers.
func (m *Machine) Timers() Timers {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t
}

// Stalled reports whether the open turn is flagged as stalled.
func (m *Machine) Stalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t.Stalled
}

func (m *Machine) setStalled(v bool, at time.Time) {
	if m.t.Stalled == v {
		return
	}
	m.t.Stalled = v
	if v {
		m.t.StalledAt = at
	} else {
		m.t.StalledAt = time.Time{}
	}
	if m.onStall != nil {
		m.onStall(v)
	}
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

// Attention returns the current attention.
func (m *Machine) Attention() Attention {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.att
}

func (m *Machine) set(to State, src Source, at time.Time) {
	att := m.att
	if to == Running || to == Exited {
		att = Attention{}
	}
	m.commit(to, src, at, att, false)
}

// commit applies a new state and attention, emitting one Change if anything
// changed.
func (m *Machine) commit(to State, src Source, at time.Time, att Attention, raised bool) {
	if m.state == to && m.att == att && !raised {
		return
	}
	from := m.state
	if to == Running && from != Running {
		// A new turn starts after an idle period, or when the user answers
		// a wait detected from the terminal; an approved permission (an
		// explicit wait → running) continues the same turn.
		if from == Idle || src == SourceInput || m.t.TurnStartedAt.IsZero() {
			m.t.TurnStartedAt, m.t.TurnEndedAt = at, time.Time{}
		}
	}
	if from == Running && to != Running {
		m.turnOpen = false
		m.setStalled(false, at)
		if to == Idle || to == Exited {
			m.t.TurnEndedAt = at
		}
	}
	c := Change{From: from, To: to, Source: src, At: at, ExitCode: m.exitCode, Attention: att, Raised: raised}
	m.state, m.att = to, att
	c.Timers = m.t
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
	m.t.LastOutputAt = now
	m.setStalled(false, now)
	if m.state == Exited || !m.heuristicActive() {
		return
	}
	// Agents keep redrawing while they wait (spinners, cursor blink), so
	// output alone never ends a wait: user input or an explicit event does.
	if m.state == WaitingInput {
		return
	}
	// A turn-complete event and the agent's final redraw race each other;
	// don't let that redraw swallow the fresh "done".
	if m.state == Idle && m.att.Unread && now.Sub(m.att.At) < DoneGrace {
		return
	}
	m.set(Running, SourceHeuristic, now)
}

// Input records that the user typed into the session. It acknowledges any
// attention and, when the wait was detected heuristically (or from a
// terminal signal), resumes the running state.
func (m *Machine) Input() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Exited {
		return
	}
	now := m.now()
	if m.state == WaitingInput && (m.heuristicActive() || m.att.Source == SourceSignal) {
		m.lastOutput = now
		m.set(Running, SourceInput, now)
		return
	}
	if m.att.Unread {
		att := m.att
		att.Unread = false
		m.commit(m.state, SourceInput, now, att, false)
	}
}

// Ack marks the current attention as seen. It reports whether anything
// changed.
func (m *Machine) Ack() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.att.Unread {
		return false
	}
	att := m.att
	att.Unread = false
	m.commit(m.state, SourceAck, m.now(), att, false)
	return true
}

// Signal records a terminal-level request for attention (BEL, OSC 9/777/99
// or a profile attention pattern). An unread attention only gets its
// message filled in. Sessions driven by hooks only take signals while
// running (the hooks already report waits and turn ends).
func (m *Machine) Signal(kind Kind, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Exited {
		return
	}
	now := m.now()
	if m.att.Active() && m.att.Unread {
		if m.att.Message == "" && msg != "" {
			att := m.att
			att.Message = msg
			m.commit(m.state, SourceSignal, now, att, false)
		}
		return
	}
	if !m.heuristicActive() && m.state != Running {
		return
	}
	if kind == "" {
		kind = KindQuestion
	}
	m.commit(WaitingInput, SourceSignal, now, Attention{Kind: kind, Message: msg, At: now, Unread: true, Source: SourceSignal}, true)
}

// Tick evaluates the idle and stall timeouts. Callers invoke it
// periodically.
func (m *Machine) Tick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != Running {
		return
	}
	now := m.now()
	if m.heuristicActive() && now.Sub(m.lastOutput) >= m.idleAfter {
		m.set(Idle, SourceHeuristic, now)
		return
	}
	if m.turnOpen && m.stallAfter > 0 && !m.t.Stalled {
		last := m.t.LastOutputAt
		if m.t.LastEventAt.After(last) {
			last = m.t.LastEventAt
		}
		if last.IsZero() {
			last = m.t.TurnStartedAt
		}
		if now.Sub(last) >= m.stallAfter {
			m.setStalled(true, now)
		}
	}
}

// Notify applies an explicit state. See NotifyEvent.
func (m *Machine) Notify(s State) error { return m.NotifyEvent(Event{State: s}) }

// NotifyEvent applies an explicit event. Exited cannot be set explicitly.
//
//   - running clears any attention;
//   - waiting_input raises a permission/question attention (a repeat of the
//     same unread wait only fills in a missing message);
//   - idle raises "done" when it ends a turn (running/waiting → idle) or
//     whenever a Kind is given (e.g. one per turn-complete hook).
func (m *Machine) NotifyEvent(ev Event) error {
	switch ev.State {
	case Running, WaitingInput, Idle:
	default:
		return fmt.Errorf("state %q cannot be notified", ev.State)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Exited {
		return fmt.Errorf("session already exited")
	}
	m.hooked = true
	now := m.now()
	m.t.LastEventAt = now
	m.setStalled(false, now)
	switch ev.State {
	case Running:
		// Treat an explicit "running" as fresh activity so the heuristic (in
		// heuristic mode) doesn't immediately flip it back to idle.
		m.lastOutput = now
		if ev.Turn && m.state == Running {
			m.t.TurnStartedAt, m.t.TurnEndedAt = now, time.Time{}
		}
		m.set(Running, SourceEvent, now)
		m.turnOpen = true
	case WaitingInput:
		kind := ev.Kind
		if kind == "" || kind == KindDone {
			kind = KindQuestion
		}
		if m.state == WaitingInput && m.att.Unread && m.att.Kind == kind {
			if m.att.Message == "" && ev.Message != "" {
				att := m.att
				att.Message = ev.Message
				m.commit(m.state, SourceEvent, now, att, false)
			}
			return nil
		}
		m.commit(WaitingInput, SourceEvent, now, Attention{Kind: kind, Message: ev.Message, At: now, Unread: true, Source: SourceEvent}, true)
	case Idle:
		kind := ev.Kind
		if kind == "" && m.state != Idle {
			kind = KindDone
		}
		if kind == "" {
			return nil // idle → idle without news
		}
		m.commit(Idle, SourceEvent, now, Attention{Kind: kind, Message: ev.Message, At: now, Unread: true, Source: SourceEvent}, true)
	}
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
