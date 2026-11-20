package daemon

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elionaz/panal/internal/project"
	"github.com/elionaz/panal/internal/session"
	"github.com/elionaz/panal/internal/settings"
	"github.com/elionaz/panal/internal/status"
)

// Automatic delivery (a project whose messages are automatic: its own
// choice, else messages.auto_deliver): when a session sits idle at its
// prompt with messages waiting, the daemon does what the Deliver button
// does (types the fixed "you have messages" line and Enter), so a message
// never waits for the user. In manual projects messages are held instead
// (mailbox.Message.Held) until the user delivers them.
//
// It holds back when that could go wrong:
//   - only Claude (its hooks hand the messages over) and only idle at its
//     prompt, never while it waits for an approval or an answer;
//   - never over a draft: when what was last typed into the session was not
//     a submit (session.Draft), its input line may hold the user's unsent
//     text, which the line and its Enter would send;
//   - never right after typing (autoQuiet);
//   - at most autoChainMax automatic deliveries to a session between two
//     things the user does to it (a prompt submitted in it, a message sent
//     to it from the app, a Deliver click), and at most autoRateMax in a
//     project per autoRateWindow whatever happens, so agents can't keep
//     waking each other up.
//
// What it holds back stays pending, with the Deliver button, as without it.
// A hold that can lift by itself (a draft, recent typing) is checked again
// every autoRetry while messages wait.
//
// The delays are atomic: tests shorten them, and a daemon's timers may still
// read them while a finished test restores them.
var (
	// autoDelay lets a burst of messages (msg send all) and the sender's
	// own turn settle before typing.
	autoDelay = newDur(1500 * time.Millisecond)
	// autoQuiet: no automatic delivery this soon after input; also the
	// window in which a second nudge (manual or automatic) is skipped.
	autoQuiet = newDur(3 * time.Second)
	// autoRetry re-checks a hold that can lift by itself, doubling up to
	// autoRetryMax while it lasts.
	autoRetry    = newDur(3 * time.Second)
	autoRetryMax = time.Minute
)

// dur is a time.Duration read and set atomically.
type dur struct{ v atomic.Int64 }

func newDur(d time.Duration) *dur {
	x := &dur{}
	x.v.Store(int64(d))
	return x
}

func (d *dur) get() time.Duration  { return time.Duration(d.v.Load()) }
func (d *dur) set(v time.Duration) { d.v.Store(int64(v)) }

const (
	// autoChainMax bounds the automatic deliveries to one session without
	// the user acting on it.
	autoChainMax = 5
	// autoRateMax per autoRateWindow bounds a project's automatic
	// deliveries whatever resets the chains.
	autoRateMax    = 20
	autoRateWindow = 10 * time.Minute
)

type autoDeliver struct {
	mu     sync.Mutex
	timers map[string]*time.Timer // session id → pending check
	chain  map[string]int         // session id → automatic deliveries since the user acted on it
	why    map[string]string      // session id → last hold logged (to log changes only)
	recent map[string][]time.Time // project id → automatic deliveries in the last autoRateWindow
	nudges map[string]*nudgeLock  // session id → its nudge lock
	retry  map[string]time.Duration
}

// forgetAuto drops what automatic delivery keeps for session id (gone).
func (s *Server) forgetAuto(id string) {
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	if t := s.auto.timers[id]; t != nil {
		t.Stop()
	}
	delete(s.auto.timers, id)
	delete(s.auto.chain, id)
	delete(s.auto.why, id)
	delete(s.auto.nudges, id)
	delete(s.auto.retry, id)
}

// nudgeLock serializes the nudges to one session (manual and automatic) and
// remembers the last one, so two never land on top of each other.
type nudgeLock struct {
	sync.Mutex
	last time.Time
}

func (s *Server) nudgeLockFor(id string) *nudgeLock {
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	if s.auto.nudges == nil {
		s.auto.nudges = map[string]*nudgeLock{}
	}
	l := s.auto.nudges[id]
	if l == nil {
		l = &nudgeLock{}
		s.auto.nudges[id] = l
	}
	return l
}

// userActed starts a new chain for session id: the user acted on it.
func (s *Server) userActed(id string) {
	s.auto.mu.Lock()
	_, had := s.auto.chain[id]
	delete(s.auto.chain, id)
	s.auto.mu.Unlock()
	if had {
		s.scheduleAutoDeliver(id) // a delivery held at the limit may go now
	}
}

// scheduleAutoDeliver checks session id for an automatic delivery after
// delay (one check per session at a time; a new trigger postpones it).
func (s *Server) scheduleAutoDeliver(id string) { s.scheduleAutoDeliverIn(id, autoDelay.get()) }

func (s *Server) scheduleAutoDeliverIn(id string, delay time.Duration) {
	sess, err := s.lookup(id)
	if err != nil || !s.messagesAuto(sess.ProjectID) {
		return
	}
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	if s.auto.timers == nil {
		s.auto.timers = map[string]*time.Timer{}
	}
	if t := s.auto.timers[id]; t != nil {
		t.Stop()
	}
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		s.auto.mu.Lock()
		if s.auto.timers[id] != t {
			s.auto.mu.Unlock()
			return // replaced meanwhile: that one runs instead
		}
		delete(s.auto.timers, id)
		s.auto.mu.Unlock()
		s.autoDeliverNow(id)
	})
	s.auto.timers[id] = t
}

// messagesAuto reports whether messages in project projectID travel by
// themselves (automatic mode): the project's own choice, else the
// messages.auto_deliver setting. Messages off means neither.
func (s *Server) messagesAuto(projectID string) bool {
	st, _ := settings.Load(s.cfg.Paths.Settings())
	if !st.Messages.Enabled {
		return false
	}
	if p, err := s.projects.Get(projectID); err == nil && p.ID == projectID {
		switch p.Messages {
		case project.MessagesAuto:
			return true
		case project.MessagesManual:
			return false
		}
	}
	return st.Messages.AutoDeliver
}

// Hold reasons that lift by themselves (checked again every autoRetry).
const (
	holdDraft  = "unsent input"
	holdTyping = "typed just now"
)

// holdHeld: the messages wait for the user to deliver them (the
// project's messages are manual).
const holdHeld = "held"

// holdNudged: the line was typed into the session (automatically or by
// Deliver): the messages stay pending until the agent takes them (Claude's
// hook on that prompt, panal msg read), so the UIs say it was told
// rather than offering to deliver again.
const holdNudged = "nudged"

// noteNudged records that sess was just told about its messages.
func (s *Server) noteNudged(sess *session.Session) {
	s.auto.mu.Lock()
	if s.auto.why == nil {
		s.auto.why = map[string]string{}
	}
	s.auto.why[sess.ID] = holdNudged
	s.auto.mu.Unlock()
	s.publishMessages(sess.ProjectID, sess.ID)
}

// holdNoTurnEnd: an agent without message hooks that went idle only by
// going quiet (no turn-end event): it gets the Deliver button.
const holdNoTurnEnd = "no turn end"

// autoDeliverNow nudges session id if every condition holds; it returns
// why not otherwise ("" when it did).
func (s *Server) autoDeliverNow(id string) string {
	sess, err := s.lookup(id)
	if err != nil {
		return "gone"
	}
	l := s.nudgeLockFor(id)
	l.Lock()
	defer l.Unlock()
	why := s.autoDeliverBlock(sess, l.last)
	if why == "" {
		why = s.takeAutoSlot(sess)
	}
	if why != "" {
		s.noteHold(sess, why)
		return why
	}
	if err := s.nudge(sess); err != nil {
		s.giveAutoSlot(sess)
		if errors.Is(err, errNudgeInterrupted) {
			s.noteHold(sess, holdTyping) // the user's input goes first; try later
			return holdTyping
		}
		s.log.Warn("automatic delivery", "to", id, "err", err)
		return "write failed"
	}
	l.last = time.Now()
	s.auto.mu.Lock()
	n := s.auto.chain[id]
	delete(s.auto.retry, id)
	s.auto.mu.Unlock()
	s.noteNudged(sess)
	s.log.Info("messages nudged", "to", id, "pending", s.mail.Pending(id), "via", "auto", "chain", n)
	return ""
}

// noteHold logs a new hold reason and re-arms a check when it can lift.
func (s *Server) noteHold(sess *session.Session, why string) {
	switch why {
	case "off", "gone", "nothing pending":
		return
	}
	s.auto.mu.Lock()
	if s.auto.why == nil {
		s.auto.why = map[string]string{}
	}
	changed := s.auto.why[sess.ID] != why
	s.auto.why[sess.ID] = why
	s.auto.mu.Unlock()
	if changed {
		s.log.Info("automatic delivery held back", "to", sess.ID, "why", why)
		s.publishMessages(sess.ProjectID, sess.ID) // the UIs say why it waits
	}
	if why == holdDraft || why == holdTyping {
		s.auto.mu.Lock()
		if s.auto.retry == nil {
			s.auto.retry = map[string]time.Duration{}
		}
		d := s.auto.retry[sess.ID]
		d = min(max(autoRetry.get(), 2*d), autoRetryMax)
		s.auto.retry[sess.ID] = d
		s.auto.mu.Unlock()
		s.scheduleAutoDeliverIn(sess.ID, d)
	}
}

// takeAutoSlot counts an automatic delivery to sess against its chain and
// its project's rate; it returns why there is no room ("" when there was).
func (s *Server) takeAutoSlot(sess *session.Session) string {
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	if s.auto.chain == nil {
		s.auto.chain = map[string]int{}
		s.auto.recent = map[string][]time.Time{}
	}
	if s.auto.chain[sess.ID] >= autoChainMax {
		return "chain limit"
	}
	cutoff := time.Now().Add(-autoRateWindow)
	recent := s.auto.recent[sess.ProjectID]
	for len(recent) > 0 && recent[0].Before(cutoff) {
		recent = recent[1:]
	}
	if len(recent) >= autoRateMax {
		s.auto.recent[sess.ProjectID] = recent
		return "project rate limit"
	}
	s.auto.chain[sess.ID]++
	s.auto.recent[sess.ProjectID] = append(recent, time.Now())
	return ""
}

// giveAutoSlot undoes takeAutoSlot (the nudge failed).
func (s *Server) giveAutoSlot(sess *session.Session) {
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	if s.auto.chain[sess.ID] > 0 {
		s.auto.chain[sess.ID]--
	}
	if r := s.auto.recent[sess.ProjectID]; len(r) > 0 {
		s.auto.recent[sess.ProjectID] = r[:len(r)-1]
	}
}

// autoDeliverBlock is why sess can't get an automatic delivery now ("" when
// it can); lastNudge is when it was last nudged.
func (s *Server) autoDeliverBlock(sess *session.Session, lastNudge time.Time) string {
	if !s.messagesAuto(sess.ProjectID) {
		return "off"
	}
	if sess.Suspended() {
		return "gone"
	}
	if s.mail.Pending(sess.ID) == 0 {
		return "nothing pending"
	}
	if s.mail.Held(sess.ID) == s.mail.Pending(sess.ID) {
		return holdHeld // sent while the project was manual: the user delivers them
	}
	if isShell(sess) {
		return "shell"
	}
	if sess.State() != status.Idle {
		return "not idle"
	}
	// Agents without message hooks are told to run panal msg read; only
	// when they reported the end of their turn (codex's notify): one that
	// merely went quiet may still be thinking, and the line would land in
	// the middle of its work.
	if !s.hookDelivers(sess.ProfileID) && sess.IdleSource() != status.SourceEvent {
		return holdNoTurnEnd
	}
	if time.Since(lastNudge) < autoQuiet.get() {
		// Told moments ago: for the UIs that is "nudged" (a check that
		// ran meanwhile must not turn it into something to deliver again).
		return holdNudged
	}
	draft, at := sess.Draft()
	if draft {
		return holdDraft
	}
	if !at.IsZero() && time.Since(at) < autoQuiet.get() {
		return holdTyping
	}
	return ""
}

// msgHold is why messages wait for session id ("" when none wait, or when
// they will be delivered by themselves as soon as it is free): the reason
// automatic delivery last held back, or "off". Shown by the Messages tab.
func (s *Server) msgHold(id string) string {
	if s.mail.Pending(id) == 0 {
		return ""
	}
	if s.mail.Held(id) > 0 {
		return holdHeld
	}
	sess, err := s.lookup(id)
	if err != nil || !s.messagesAuto(sess.ProjectID) {
		return "off"
	}
	s.auto.mu.Lock()
	defer s.auto.mu.Unlock()
	return s.auto.why[id]
}
