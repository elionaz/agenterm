// Package termsig extracts "look at me" signals from a terminal byte stream:
// a bare BEL, and the desktop-notification escapes OSC 9 (iTerm2/Ghostty),
// OSC 777 (urxvt/foot/Ghostty) and OSC 99 (kitty). It also produces the
// stream's plain text (escape sequences removed) for pattern matching.
//
// The Parser is a streaming state machine: sequences may be split across
// any chunk boundary. It never modifies the stream; callers keep passing the
// original bytes to the terminal.
package termsig

import (
	"strings"
	"unicode/utf8"
)

// Signal is one notification found in the stream. When Progress is set the
// signal is a ConEmu progress report (OSC 9;4), not a request for attention.
type Signal struct {
	Bell     bool   // a bare BEL (no message)
	Title    string // optional (OSC 777 / OSC 99)
	Message  string
	Progress *Progress
}

// ProgressState is the st field of OSC 9;4.
type ProgressState uint8

const (
	ProgressNone          ProgressState = 0 // clear / hide
	ProgressNormal        ProgressState = 1
	ProgressError         ProgressState = 2
	ProgressIndeterminate ProgressState = 3
	ProgressPaused        ProgressState = 4
)

// String is the wire name ("" for ProgressNone).
func (s ProgressState) String() string {
	switch s {
	case ProgressNormal:
		return "normal"
	case ProgressError:
		return "error"
	case ProgressIndeterminate:
		return "indeterminate"
	case ProgressPaused:
		return "paused"
	}
	return ""
}

// Progress is a parsed `OSC 9 ; 4 ; st ; pr` (ConEmu / Windows Terminal /
// Ghostty progress bar). Value is 0-100 (meaningless when indeterminate).
type Progress struct {
	State ProgressState
	Value int
}

// Text returns a single-line description: "title: message", or whichever
// is present.
func (s Signal) Text() string {
	switch {
	case s.Title != "" && s.Message != "":
		return s.Title + ": " + s.Message
	case s.Title != "":
		return s.Title
	}
	return s.Message
}

type state uint8

const (
	ground   state = iota
	esc            // after ESC
	csi            // ESC [ … final
	osc            // ESC ] … BEL | ESC \
	oscEsc         // ESC inside OSC (maybe ST)
	str            // DCS/SOS/PM/APC … ESC \ (ignored)
	strEsc         // ESC inside str
	escInter       // ESC + intermediate bytes (e.g. ESC ( B)
)

// maxOSC bounds a buffered OSC payload; longer ones (images, hyperlinks
// with huge URLs) are skipped without being parsed.
const maxOSC = 4096

// Parser consumes terminal output. The zero value is ready to use. It is
// not safe for concurrent use.
type Parser struct {
	st       state
	buf      []byte // OSC payload
	overflow bool
	// kitty OSC 99 notifications may arrive in chunks (d=0) keyed by id.
	k99Title, k99Body string
}

// Feed parses p. onSignal (may be nil) is called for every signal; onText
// (may be nil) receives runs of printable text (with \r, \n, \t and \b
// kept), so a caller can keep an ANSI-free tail of the output. Backspace is
// kept because line editors redraw with it (zsh echoes "f\bfor").
func (p *Parser) Feed(b []byte, onSignal func(Signal), onText func([]byte)) {
	textStart := -1
	flush := func(i int) {
		if textStart >= 0 && onText != nil && i > textStart {
			onText(b[textStart:i])
		}
		textStart = -1
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch p.st {
		case ground:
			switch {
			case c == 0x1b:
				flush(i)
				p.st = esc
			case c == 0x07:
				flush(i)
				if onSignal != nil {
					onSignal(Signal{Bell: true})
				}
			case c >= 0x20 || c == '\n' || c == '\r' || c == '\t' || c == '\b':
				if textStart < 0 {
					textStart = i
				}
			default: // other C0 controls: drop
				flush(i)
			}
		case esc:
			switch {
			case c == '[':
				p.st = csi
			case c == ']':
				p.st = osc
				p.buf = p.buf[:0]
				p.overflow = false
			case c == 'P' || c == 'X' || c == '^' || c == '_':
				p.st = str
			case c >= 0x20 && c <= 0x2f:
				p.st = escInter
			case c == 0x1b:
				// ESC ESC: stay
			default:
				p.st = ground
			}
		case escInter:
			if c < 0x20 || c > 0x2f {
				p.st = ground
			}
		case csi:
			if c >= 0x40 && c <= 0x7e {
				p.st = ground
				if (c == 'H' || c == 'f') && onText != nil {
					onText([]byte{'\n'}) // cursor jumps usually start a new line of text
				}
			} else if c == 0x1b {
				p.st = esc
			} else if c == 0x18 || c == 0x1a { // CAN/SUB abort
				p.st = ground
			}
		case osc:
			switch c {
			case 0x07:
				p.endOSC(onSignal)
			case 0x1b:
				p.st = oscEsc
			case 0x18, 0x1a:
				p.st = ground
			default:
				if len(p.buf) < maxOSC {
					p.buf = append(p.buf, c)
				} else {
					p.overflow = true
				}
			}
		case oscEsc:
			if c == '\\' {
				p.endOSC(onSignal)
			} else {
				// Malformed: treat the ESC as terminating the OSC and
				// reprocess this byte as the start of a new escape.
				p.st = esc
				i--
			}
		case str:
			if c == 0x1b {
				p.st = strEsc
			} else if c == 0x18 || c == 0x1a {
				p.st = ground
			}
		case strEsc:
			if c == '\\' {
				p.st = ground
			} else if c != 0x1b {
				p.st = str
			}
		}
	}
	flush(len(b))
}

func (p *Parser) endOSC(onSignal func(Signal)) {
	p.st = ground
	if p.overflow {
		return
	}
	payload := string(p.buf)
	num, rest, ok := strings.Cut(payload, ";")
	if !ok {
		return
	}
	var sig Signal
	switch num {
	case "9":
		// ConEmu reuses OSC 9 with numeric sub-commands ("9;4;…" is the
		// progress bar Ghostty/Windows Terminal show): only free text is a
		// notification.
		if pr, ok := parseProgress(rest); ok {
			if onSignal != nil {
				onSignal(Signal{Progress: &pr})
			}
			return
		}
		if isConEmuSubcommand(rest) || strings.TrimSpace(rest) == "" {
			return
		}
		sig.Message = rest
	case "777":
		parts := strings.SplitN(rest, ";", 3)
		if len(parts) < 2 || parts[0] != "notify" {
			return
		}
		sig.Title = parts[1]
		if len(parts) == 3 {
			sig.Message = parts[2]
		}
	case "99":
		meta, body, _ := strings.Cut(rest, ";")
		done, isBody := true, false
		for _, kv := range strings.Split(meta, ":") {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case "d":
				done = v != "0"
			case "p":
				isBody = v == "body"
				if v != "" && v != "title" && v != "body" {
					return // icons, buttons, queries…
				}
			case "e":
				if v == "1" {
					return // base64 payload: not worth decoding here
				}
			}
		}
		if isBody {
			p.k99Body += body
		} else {
			p.k99Title += body
		}
		if !done {
			return
		}
		sig.Title, sig.Message = p.k99Title, p.k99Body
		if sig.Message == "" {
			sig.Title, sig.Message = "", sig.Title
		}
		p.k99Title, p.k99Body = "", ""
	default:
		return
	}
	sig.Title = clean(sig.Title)
	sig.Message = clean(sig.Message)
	if sig.Title == "" && sig.Message == "" {
		return
	}
	if onSignal != nil {
		onSignal(sig)
	}
}

// parseProgress parses the part after "9;" of `OSC 9;4;st;pr`. A missing
// pr means 0; out-of-range values are clamped; an unknown st is ignored.
func parseProgress(s string) (Progress, bool) {
	rest, ok := strings.CutPrefix(s, "4")
	if !ok || (rest != "" && rest[0] != ';') {
		return Progress{}, false
	}
	parts := strings.Split(strings.TrimPrefix(rest, ";"), ";")
	st, ok := atoi(parts[0])
	if !ok && parts[0] != "" {
		return Progress{}, false
	}
	if st > int(ProgressPaused) {
		return Progress{}, false
	}
	pr := 0
	if len(parts) > 1 {
		pr, _ = atoi(parts[1])
	}
	return Progress{State: ProgressState(st), Value: min(max(pr, 0), 100)}, true
}

func atoi(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func isConEmuSubcommand(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && (i == len(s) || s[i] == ';')
}

// clean drops invalid UTF-8 and control characters and trims the result.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
