package termsig

import (
	"reflect"
	"strings"
	"testing"
)

func parseChunks(chunks ...string) ([]Signal, string) {
	var p Parser
	var sigs []Signal
	var text strings.Builder
	for _, c := range chunks {
		p.Feed([]byte(c), func(s Signal) { sigs = append(sigs, s) }, func(b []byte) { text.Write(b) })
	}
	return sigs, text.String()
}

// everySplit feeds s split at every possible position (and byte by byte).
func everySplit(t *testing.T, s string, want []Signal) {
	t.Helper()
	for i := 0; i <= len(s); i++ {
		got, _ := parseChunks(s[:i], s[i:])
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("split at %d of %q: got %+v want %+v", i, s, got, want)
		}
	}
	bytes := make([]string, len(s))
	for i := range s {
		bytes[i] = s[i : i+1]
	}
	if got, _ := parseChunks(bytes...); !reflect.DeepEqual(got, want) {
		t.Fatalf("byte by byte %q: got %+v want %+v", s, got, want)
	}
}

func TestBell(t *testing.T) {
	everySplit(t, "hi\a there", []Signal{{Bell: true}})
}

func TestOSC9(t *testing.T) {
	everySplit(t, "x\x1b]9;Need approval\x07y", []Signal{{Message: "Need approval"}})
	everySplit(t, "x\x1b]9;Need approval\x1b\\y", []Signal{{Message: "Need approval"}})
}

func TestOSC9Progress(t *testing.T) {
	pr := func(st ProgressState, v int) Signal { return Signal{Progress: &Progress{State: st, Value: v}} }
	everySplit(t, "a\x1b]9;4;1;50\x07b\x1b]9;4;0\x1b\\\x1b]9;1;100\x07", []Signal{pr(ProgressNormal, 50), pr(ProgressNone, 0)})
	everySplit(t, "\x1b]9;4;3\x07\x1b]9;4;2;250\x1b\\\x1b]9;4;4;7\x07", []Signal{pr(ProgressIndeterminate, 0), pr(ProgressError, 100), pr(ProgressPaused, 7)})
	// Unknown states and other ConEmu sub-commands are not progress (nor
	// notifications).
	everySplit(t, "\x1b]9;4;9;10\x07\x1b]9;42;1\x07", nil)
}

func TestProgressIsNotText(t *testing.T) {
	_, text := parseChunks("ok\x1b]9;4;1;5", "0\x07done")
	if text != "okdone" {
		t.Fatalf("text %q", text)
	}
}

func TestBellInsideOSCIgnored(t *testing.T) {
	// The BEL terminates the title OSC; it is not a bell.
	everySplit(t, "\x1b]0;my title\x07\x1b]2;x\x07", nil)
	// Hyperlinks (OSC 8) terminated by BEL.
	everySplit(t, "\x1b]8;;https://x\x07link\x1b]8;;\x07", nil)
}

func TestBellInsideDCSIgnored(t *testing.T) {
	everySplit(t, "\x1bPq#0\a;2\x1b\\", nil)
}

func TestOSC777(t *testing.T) {
	everySplit(t, "\x1b]777;notify;Claude;Wants to run rm\x07", []Signal{{Title: "Claude", Message: "Wants to run rm"}})
	everySplit(t, "\x1b]777;preexec\x07", nil)
}

func TestOSC99(t *testing.T) {
	everySplit(t, "\x1b]99;i=1:d=0;Hello\x1b\\\x1b]99;i=1:p=body;World\x1b\\", []Signal{{Title: "Hello", Message: "World"}})
	everySplit(t, "\x1b]99;;Just title\x1b\\", []Signal{{Message: "Just title"}})
}

func TestMixedAndText(t *testing.T) {
	sigs, text := parseChunks("\x1b[1;31mDo you want\x1b[0m to proceed?\r\n\x1b]9;Need approval\x07", "\a")
	want := []Signal{{Message: "Need approval"}, {Bell: true}}
	if !reflect.DeepEqual(sigs, want) {
		t.Fatalf("got %+v", sigs)
	}
	if text != "Do you want to proceed?\r\n" {
		t.Fatalf("text %q", text)
	}
}

func TestOversizedOSCSkipped(t *testing.T) {
	big := "\x1b]9;" + strings.Repeat("a", maxOSC+10) + "\x07\a"
	sigs, _ := parseChunks(big)
	if !reflect.DeepEqual(sigs, []Signal{{Bell: true}}) {
		t.Fatalf("got %d signals", len(sigs))
	}
}

func TestSignalText(t *testing.T) {
	if got := (Signal{Title: "a", Message: "b"}).Text(); got != "a: b" {
		t.Fatal(got)
	}
}
