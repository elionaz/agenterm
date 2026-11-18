package session

import (
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/elionaz/agenterm/internal/termsig"
)

// Activity sources.
const (
	ActivityHook   = "hook"   // reported by an agent hook (`notify --activity`)
	ActivityOutput = "output" // last meaningful line of output
)

// Activity coalescing: at most one OnActivity per activityInterval, the
// output-derived line is recomputed at most that often too, and an output
// heartbeat (refreshing lastOutputAt in UIs) is sent at most every
// heartbeatInterval.
const (
	activityInterval  = 500 * time.Millisecond
	heartbeatInterval = 5 * time.Second
	maxActivityRunes  = 120
	maxLineBytes      = 1024
)

// Live is a snapshot of what a session is doing.
type Live struct {
	Activity       string
	ActivityKind   string // structured hook activity (protocol.Live)
	ActivityTarget string
	ActivityAt     time.Time
	ActivitySource string
	Progress       termsig.Progress
}

// tracker holds the activity line and progress and decides when a
// (coalesced) activity notification is due. Methods take the current time so
// tests can drive it.
type tracker struct {
	mu       sync.Mutex
	live     Live
	hookSeen bool // hook activity wins over output for the session's life (set from the start for hook-driven sessions)
	dirty    bool // changed since the last emission
	outSince bool // output seen since the last emission (heartbeat)
	lastEmit time.Time
	lastEval time.Time // last output-line evaluation
}

// setHook records activity reported by a hook. It reports a change.
func (t *tracker) setHook(text, kind, target string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hookSeen = true
	return t.setLocked(text, kind, target, ActivityHook, now)
}

// setOutput records the output-derived line, unless hooks report activity.
func (t *tracker) setOutput(text string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hookSeen || text == "" {
		return false
	}
	return t.setLocked(text, "", "", ActivityOutput, now)
}

func (t *tracker) setLocked(text, kind, target, src string, now time.Time) bool {
	if text == "" || (text == t.live.Activity && src == t.live.ActivitySource && kind == t.live.ActivityKind && target == t.live.ActivityTarget) {
		return false
	}
	t.live.Activity, t.live.ActivitySource, t.live.ActivityAt = text, src, now
	t.live.ActivityKind, t.live.ActivityTarget = kind, target
	t.dirty = true
	return true
}

func (t *tracker) setProgress(p termsig.Progress) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p.State == termsig.ProgressNone {
		p.Value = 0
	}
	if p != t.live.Progress {
		t.live.Progress = p
		t.dirty = true
	}
}

// touch forces the next emission (timers changed without a state change).
func (t *tracker) touch() {
	t.mu.Lock()
	t.dirty = true
	t.mu.Unlock()
}

// output notes PTY activity for the heartbeat.
func (t *tracker) output() {
	t.mu.Lock()
	t.outSince = true
	t.mu.Unlock()
}

// evalDue reports whether the output line should be recomputed now (and
// records that it was).
func (t *tracker) evalDue(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hookSeen || now.Sub(t.lastEval) < activityInterval {
		return false
	}
	t.lastEval = now
	return true
}

// due reports whether an activity notification should be emitted now, and
// if so marks it as emitted. force bypasses the rate limit (stall flips).
func (t *tracker) due(now time.Time, force bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	since := now.Sub(t.lastEmit)
	ok := force ||
		(t.dirty && since >= activityInterval) ||
		(t.outSince && since >= heartbeatInterval)
	if !ok {
		return false
	}
	t.dirty, t.outSince, t.lastEmit = false, false, now
	return true
}

func (t *tracker) snapshot() Live {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.live
}

// lineTracker follows the plain-text output (from termsig) and keeps the
// last meaningful line: CR and LF end a line (so a CR-redrawn progress line
// keeps its latest version), spinner-only and box-drawing-only lines are
// skipped.
type lineTracker struct {
	mu    sync.Mutex
	cur   []byte
	last  string
	dirty bool
}

func (l *lineTracker) feed(b []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range b {
		switch c {
		case '\r', '\n':
			l.commit()
		case '\b':
			l.cur = eraseLast(l.cur)
		default:
			if len(l.cur) < maxLineBytes {
				l.cur = append(l.cur, c)
			}
		}
	}
	l.dirty = true
}

// eraseLast applies a backspace to plain text: it drops the last rune of b
// (none after a line break).
func eraseLast(b []byte) []byte {
	if len(b) == 0 || b[len(b)-1] == '\n' {
		return b
	}
	_, size := utf8.DecodeLastRune(b)
	return b[:len(b)-size]
}

func (l *lineTracker) commit() {
	if s := meaningfulLine(string(l.cur)); s != "" {
		l.last = s
	}
	l.cur = l.cur[:0]
}

// candidate returns the line in progress if meaningful, else the last
// complete one. ok is false when nothing was fed since the last call.
func (l *lineTracker) candidate() (s string, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return "", false
	}
	l.dirty = false
	if s := meaningfulLine(string(l.cur)); s != "" {
		return s, true
	}
	return l.last, true
}

// decorative runes: spinners, box drawing, blocks, geometric shapes and
// dingbats that TUIs put around (or instead of) text.
func decorative(r rune) bool {
	switch {
	case unicode.IsSpace(r), r == utf8.RuneError:
		return true
	case r >= 0x2500 && r <= 0x25FF, // box drawing, blocks, geometric shapes
		r >= 0x2800 && r <= 0x28FF, // braille (spinners)
		r >= 0x2700 && r <= 0x27BF, // dingbats (✻ ✶ ✢ ✳ …)
		r >= 0x2190 && r <= 0x21FF, // arrows
		r >= 0xE000 && r <= 0xF8FF: // private use (powerline / nerd font glyphs)
		return true
	}
	return strings.ContainsRune("|/-\\*·•●○◐◓◑◒>❯›»", r)
}

// meaningfulLine cleans a line of plain output for the activity line: it
// trims decoration, collapses whitespace and requires at least two letters
// or digits. It returns "" for lines that are not worth showing.
func meaningfulLine(s string) string {
	s = strings.ToValidUTF8(s, "")
	if looksLikePrompt(s) {
		return ""
	}
	s = strings.TrimFunc(s, decorative)
	alnum := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alnum++
		}
	}
	if alnum < 2 {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxActivityRunes {
		s = string(r[:maxActivityRunes-1]) + "…"
	}
	return s
}

// maxPromptRunes bounds what may be taken for a shell prompt: longer lines
// are output even if they end like one. A user@host prompt shows the whole
// directory, and worktree folders are long, so it gets a larger bound.
const (
	maxPromptRunes         = 80
	maxUserHostPromptRunes = 240
)

// userHost matches the user@host (or user@host:dir) start of common prompts
// (optionally after a "(venv)" marker); shellName bash's default "bash-5.2".
var (
	userHost  = regexp.MustCompile(`^\s*(\(\S+\)\s+)?\[?[\w.-]+@[\w.-]+[\]:\s]`)
	shellName = regexp.MustCompile(`^\s*[a-z]*sh(-[\d.]+)?$`)
	dirToken  = regexp.MustCompile(`^\s*[\w.~/-]+$`)
)

// looksLikePrompt reports whether a line is (heuristically) an idle shell
// prompt, so shell sessions show their last output rather than "user@mac ~ %":
// a short line ending in $ % # > ❯ ➜ (plus optional spaces) where, for the
// ASCII ones, the symbol stands alone, follows a single directory-like word
// and a space, a path-ish character (~ / \ ] ) }), "bash-5.2", or the line
// starts with user@host — so "50%", "cost: $" or "<div>" are not prompts. A
// line starting with ➜ (oh-my-zsh) counts too. A prompt with a command typed
// after it is not one: that is the user's input, worth showing.
func looksLikePrompt(s string) bool {
	t := strings.TrimRightFunc(s, unicode.IsSpace)
	r := []rune(strings.TrimLeftFunc(t, unicode.IsSpace))
	if len(r) == 0 || len(r) > maxUserHostPromptRunes {
		return false
	}
	if len(r) > maxPromptRunes {
		// Only an unmistakable user@host:dir$ (no spaces in the directory).
		last := r[len(r)-1]
		body := string(r[:len(r)-1])
		return strings.ContainsRune("$%#", last) && !strings.ContainsFunc(body, unicode.IsSpace) &&
			userHost.MatchString(body+" ")
	}
	if r[0] == '➜' {
		return true
	}
	switch r[len(r)-1] {
	case '❯', '➜':
		return true
	case '$', '%', '#', '>':
	default:
		return false
	}
	body := strings.TrimRightFunc(string(r[:len(r)-1]), unicode.IsSpace)
	if body == "" || userHost.MatchString(body+" ") || shellName.MatchString(body) {
		return true
	}
	if spaced := len(body) < len(string(r[:len(r)-1])); spaced && dirToken.MatchString(body) {
		return true // "agenterm %", "~/Dev $"
	}
	prev, _ := utf8.DecodeLastRuneInString(body)
	return strings.ContainsRune("~/\\])}", prev)
}

// Tail limits.
const (
	DefaultTailLines = 8
	MaxTailLines     = 200
	tailBytes        = 64 << 10
	maxTailRunes     = 240
)

// TailLines renders the end of a raw terminal stream as plain text: escape
// sequences removed, CR overwriting the line from column 0, runs of blank
// lines collapsed. It returns at most n lines, oldest first.
func TailLines(data []byte, n int) []string {
	if n <= 0 {
		n = DefaultTailLines
	}
	n = min(n, MaxTailLines)
	if len(data) > tailBytes {
		data = data[len(data)-tailBytes:]
		// Start at a line boundary so a cut escape sequence doesn't leak.
		if i := strings.IndexByte(string(data[:min(len(data), 4096)]), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	var text []byte
	var p termsig.Parser
	p.Feed(data, nil, func(b []byte) { text = append(text, b...) })

	var lines []string
	var cur []rune
	col := 0
	push := func() {
		line := strings.TrimRightFunc(string(cur), unicode.IsSpace)
		if r := []rune(line); len(r) > maxTailRunes {
			line = string(r[:maxTailRunes-1]) + "…"
		}
		if line == "" && (len(lines) == 0 || lines[len(lines)-1] == "") {
			// collapse blank runs (and drop leading blanks)
		} else {
			lines = append(lines, line)
		}
		cur, col = cur[:0], 0
	}
	for _, r := range string(strings.ToValidUTF8(string(text), "")) {
		switch r {
		case '\n':
			push()
		case '\r':
			col = 0
		case '\b':
			col = max(col-1, 0)
		case '\t':
			for range 4 - col%4 {
				cur, col = put(cur, col, ' ')
			}
		default:
			cur, col = put(cur, col, r)
		}
	}
	push()
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if lines == nil {
		lines = []string{}
	}
	return lines
}

func put(cur []rune, col int, r rune) ([]rune, int) {
	if col < len(cur) {
		cur[col] = r
	} else {
		cur = append(cur, r)
	}
	return cur, col + 1
}
