// Package session owns a single PTY-backed process: it spawns it, records its
// output into a ring buffer, fans output out to attached subscribers and
// tracks its status.
package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/status"
	"github.com/elionaz/agenterm/internal/termsig"
)

// Environment variables injected into every session.
const (
	EnvSession = protocol.EnvSession
	EnvSock    = protocol.EnvSock
)

// subscriberQueue is how many output chunks a subscriber may lag behind
// before it is disconnected.
const subscriberQueue = 256

// Options configures Spawn.
type Options struct {
	ID          string
	ProjectID   string
	ProjectDir  string
	ProjectName string
	ProfileID   string
	Cmd         string
	Args        []string
	Env         []string // extra KEY=VALUE pairs appended to os.Environ()
	Cwd         string   // defaults to ProjectDir
	SockPath    string   // exported as AGENTERM_SOCK
	Cols, Rows  uint16
	RingSize    int

	StatusMode status.Mode
	IdleAfter  time.Duration
	OnState    func(status.Change) // see status.Options.OnChange

	// AttentionPatterns are matched against the recent ANSI-free output
	// once it settles; a match raises a "question" attention.
	AttentionPatterns []*regexp.Regexp
}

// Pattern matching: how long output must be quiet before the tail is
// checked, and how much of it is kept.
const (
	patternQuiet = 300 * time.Millisecond
	tailSize     = 2048
)

// Session is a running (or exited) PTY process. Identity fields are immutable
// after Spawn; dynamic state is read through methods.
type Session struct {
	ID          string
	ProjectID   string
	ProjectDir  string
	ProjectName string
	ProfileID   string
	Cmd         string
	Args        []string
	Env         []string
	Cwd         string
	CreatedAt   time.Time
	Pid         int

	cmd    *exec.Cmd
	ptmx   *os.File
	ring   *Ring
	status *status.Machine

	// outMu serializes ring writes, subscriber fan-out and Attach so a new
	// subscriber's snapshot and live stream never overlap or leave a gap.
	outMu sync.Mutex
	subs  map[*Subscriber]struct{}

	sizeMu     sync.Mutex
	cols, rows uint16

	sig      termsig.Parser // only used by readLoop
	patterns []*regexp.Regexp
	tailMu   sync.Mutex
	tail     []byte    // ANSI-free recent output, for patterns
	tailAt   time.Time // last append
	tailDirt bool      // appended since last pattern check
	matched  string    // last line that matched (debounce)

	done     chan struct{}
	readDone chan struct{}
	killOnce sync.Once
}

// Spawn starts the process on a new PTY.
func Spawn(o Options) (*Session, error) {
	if o.ID == "" || o.Cmd == "" {
		return nil, errors.New("session: ID and Cmd are required")
	}
	if o.Cols == 0 {
		o.Cols = 80
	}
	if o.Rows == 0 {
		o.Rows = 24
	}
	if o.Cwd == "" {
		o.Cwd = o.ProjectDir
	}
	s := &Session{
		ID:          o.ID,
		ProjectID:   o.ProjectID,
		ProjectDir:  o.ProjectDir,
		ProjectName: o.ProjectName,
		ProfileID:   o.ProfileID,
		Cmd:         o.Cmd,
		Args:        append([]string(nil), o.Args...),
		Env:         append([]string(nil), o.Env...),
		Cwd:         o.Cwd,
		CreatedAt:   time.Now().UTC(),
		ring:        NewRing(o.RingSize),
		subs:        make(map[*Subscriber]struct{}),
		cols:        o.Cols,
		rows:        o.Rows,
		done:        make(chan struct{}),
		readDone:    make(chan struct{}),
		patterns:    o.AttentionPatterns,
	}
	s.status = status.New(status.Options{Mode: o.StatusMode, IdleAfter: o.IdleAfter, OnChange: o.OnState})

	cmd := exec.Command(o.Cmd, o.Args...)
	cmd.Dir = o.Cwd
	cmd.Env = buildEnv(o)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: o.Cols, Rows: o.Rows})
	if err != nil {
		return nil, fmt.Errorf("spawn %s: %w", o.Cmd, err)
	}
	s.cmd, s.ptmx, s.Pid = cmd, ptmx, cmd.Process.Pid

	go s.readLoop()
	go s.waitLoop()
	go s.tickLoop()
	return s, nil
}

func buildEnv(o Options) []string {
	env := os.Environ()
	hasTerm := false
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "TERM=" {
			hasTerm = true
		}
	}
	if !hasTerm {
		env = append(env, "TERM=xterm-256color")
	}
	env = append(env, o.Env...)
	env = append(env, EnvSession+"="+o.ID)
	if o.SockPath != "" {
		env = append(env, EnvSock+"="+o.SockPath)
	}
	return env
}

func (s *Session) readLoop() {
	defer close(s.readDone)
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			s.status.Output()
			s.scan(chunk)
			s.broadcast(chunk)
		}
		if err != nil {
			// EIO is the normal "slave closed" signal on Linux/macOS.
			return
		}
	}
}

// scan looks for terminal notifications (BEL, OSC 9/777/99) and feeds the
// pattern tail. The chunk itself is passed to subscribers untouched.
func (s *Session) scan(chunk []byte) {
	var onText func([]byte)
	if len(s.patterns) > 0 {
		onText = s.appendTail
	}
	s.sig.Feed(chunk, func(sg termsig.Signal) {
		s.status.Signal(status.KindQuestion, sg.Text())
	}, onText)
}

func (s *Session) appendTail(b []byte) {
	s.tailMu.Lock()
	defer s.tailMu.Unlock()
	s.tail = append(s.tail, b...)
	if over := len(s.tail) - tailSize; over > 0 {
		s.tail = append(s.tail[:0], s.tail[over:]...)
	}
	s.tailAt = time.Now()
	s.tailDirt = true
}

// checkPatterns runs once output has been quiet for patternQuiet. A line
// that already matched does not fire again until the user types.
func (s *Session) checkPatterns() {
	if len(s.patterns) == 0 {
		return
	}
	s.tailMu.Lock()
	if !s.tailDirt || time.Since(s.tailAt) < patternQuiet {
		s.tailMu.Unlock()
		return
	}
	s.tailDirt = false
	text := string(s.tail)
	var line string
	for _, re := range s.patterns {
		if loc := re.FindStringIndex(text); loc != nil {
			line = lineAround(text, loc[0], loc[1])
			break
		}
	}
	fire := line != "" && line != s.matched
	if fire {
		s.matched = line
	}
	s.tailMu.Unlock()
	if fire {
		s.status.Signal(status.KindQuestion, line)
	}
}

// lineAround returns the trimmed line(s) containing text[i:j].
func lineAround(text string, i, j int) string {
	start := strings.LastIndexAny(text[:i], "\r\n") + 1
	end := len(text)
	if k := strings.IndexAny(text[j:], "\r\n"); k >= 0 {
		end = j + k
	}
	line := strings.Join(strings.Fields(text[start:end]), " ")
	if r := []rune(line); len(r) > 200 {
		line = string(r[:200])
	}
	return line
}

func (s *Session) broadcast(chunk []byte) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.ring.Write(chunk)
	for sub := range s.subs {
		select {
		case sub.ch <- chunk:
		default:
			// Too slow: drop the subscriber rather than block the PTY.
			sub.dropped = true
			delete(s.subs, sub)
			close(sub.ch)
		}
	}
}

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	code := exitCode(s.cmd.ProcessState, err)
	// Drain whatever output is still buffered, but don't hang forever if a
	// grandchild keeps the slave side open.
	select {
	case <-s.readDone:
	case <-time.After(500 * time.Millisecond):
	}
	s.status.Exit(code)
	s.ptmx.Close()
	<-s.readDone

	s.outMu.Lock()
	for sub := range s.subs {
		delete(s.subs, sub)
		close(sub.ch)
	}
	s.outMu.Unlock()
	close(s.done)
}

func exitCode(ps *os.ProcessState, err error) int {
	if ps == nil {
		if err != nil {
			return -1
		}
		return 0
	}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

func (s *Session) tickLoop() {
	interval := max(s.status.IdleAfter()/4, 50*time.Millisecond)
	if len(s.patterns) > 0 {
		interval = min(interval, patternQuiet/2)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.status.Tick()
			s.checkPatterns()
		}
	}
}

// Subscriber receives live output chunks. C is closed when the session exits
// or when the subscriber falls too far behind (Dropped reports which).
type Subscriber struct {
	C       <-chan []byte
	ch      chan []byte
	s       *Session
	dropped bool // guarded by s.outMu
}

// Dropped reports whether the subscriber was disconnected for being slow.
// Only meaningful after C is closed.
func (sub *Subscriber) Dropped() bool {
	sub.s.outMu.Lock()
	defer sub.s.outMu.Unlock()
	return sub.dropped
}

// Close detaches the subscriber. It is safe to call multiple times.
func (sub *Subscriber) Close() {
	sub.s.outMu.Lock()
	defer sub.s.outMu.Unlock()
	if _, ok := sub.s.subs[sub]; ok {
		delete(sub.s.subs, sub)
		close(sub.ch)
	}
}

// Attach returns the current scrollback and a subscriber for output produced
// after it, atomically. Attaching to an exited session returns a subscriber
// whose channel is already closed.
func (s *Session) Attach() (snapshot []byte, sub *Subscriber) {
	ch := make(chan []byte, subscriberQueue)
	sub = &Subscriber{C: ch, ch: ch, s: s}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	snapshot = s.ring.Snapshot()
	select {
	case <-s.done:
		close(ch)
	default:
		s.subs[sub] = struct{}{}
	}
	return snapshot, sub
}

// Scrollback returns a copy of the ring buffer.
func (s *Session) Scrollback() []byte { return s.ring.Snapshot() }

// Write sends input to the PTY. Input that is not just an automatic
// terminal reply counts as the user answering: it acknowledges attention.
func (s *Session) Write(p []byte) (int, error) {
	select {
	case <-s.done:
		return 0, errors.New("session exited")
	default:
	}
	if !isTerminalReply(p) {
		s.status.Input()
		s.tailMu.Lock()
		s.tail, s.matched, s.tailDirt = s.tail[:0], "", false
		s.tailMu.Unlock()
	}
	return s.ptmx.Write(p)
}

// isTerminalReply reports whether p consists only of sequences a terminal
// emulator sends on its own: device attribute / status / mode reports
// (CSI … c|n|R|y|t|u), focus in/out (CSI I|O), and OSC/DCS replies.
func isTerminalReply(p []byte) bool {
	if len(p) == 0 {
		return true
	}
	for len(p) > 0 {
		if len(p) < 2 || p[0] != 0x1b {
			return false
		}
		switch p[1] {
		case '[':
			i := 2
			for i < len(p) && p[i] >= 0x20 && p[i] <= 0x3f {
				i++
			}
			if i >= len(p) {
				return false
			}
			switch p[i] {
			case 'c', 'n', 'R', 'y', 't', 'u', 'I', 'O':
			default:
				return false
			}
			p = p[i+1:]
		case ']', 'P':
			// Ends at BEL or ST.
			end := -1
			for i := 2; i < len(p); i++ {
				if p[i] == 0x07 {
					end = i + 1
					break
				}
				if p[i] == 0x1b && i+1 < len(p) && p[i+1] == '\\' {
					end = i + 2
					break
				}
			}
			if end < 0 {
				return false
			}
			p = p[end:]
		default:
			return false
		}
	}
	return true
}

// Resize changes the PTY window size.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return fmt.Errorf("invalid size %dx%d", cols, rows)
	}
	select {
	case <-s.done:
		return errors.New("session exited")
	default:
	}
	s.sizeMu.Lock()
	defer s.sizeMu.Unlock()
	if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return err
	}
	s.cols, s.rows = cols, rows
	return nil
}

// Size returns the last applied window size.
func (s *Session) Size() (cols, rows uint16) {
	s.sizeMu.Lock()
	defer s.sizeMu.Unlock()
	return s.cols, s.rows
}

// Notify applies an explicit status event.
func (s *Session) Notify(st status.State) error { return s.status.Notify(st) }

// NotifyEvent applies an explicit status event with attention details.
func (s *Session) NotifyEvent(ev status.Event) error { return s.status.NotifyEvent(ev) }

// Ack marks the session's attention as seen.
func (s *Session) Ack() bool { return s.status.Ack() }

// Attention returns the session's current attention.
func (s *Session) Attention() status.Attention { return s.status.Attention() }

// State returns the current status.
func (s *Session) State() status.State { return s.status.State() }

// ExitCode returns the exit code and whether the process has exited.
func (s *Session) ExitCode() (int, bool) { return s.status.ExitCode() }

// Done is closed once the process has exited and all output was delivered.
func (s *Session) Done() <-chan struct{} { return s.done }

// Kill terminates the process group: SIGHUP+SIGTERM, then SIGKILL after
// grace. It returns once the process has exited.
func (s *Session) Kill(grace time.Duration) {
	s.killOnce.Do(func() {
		pgid := s.Pid // pty.Start makes the child a session (and group) leader
		_ = syscall.Kill(-pgid, syscall.SIGHUP)
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		select {
		case <-s.done:
			return
		case <-time.After(grace):
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	})
	<-s.done
}
