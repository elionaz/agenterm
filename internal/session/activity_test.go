package session

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/termsig"
)

// lastLine feeds raw terminal chunks through termsig (as the session does)
// and returns the activity candidate.
func lastLine(chunks ...string) string {
	var p termsig.Parser
	var l lineTracker
	for _, c := range chunks {
		p.Feed([]byte(c), nil, l.feed)
	}
	s, _ := l.candidate()
	return s
}

func TestOutputActivityLine(t *testing.T) {
	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{"plain", []string{"compiling\nrunning tests\n"}, "running tests"},
		{"partial line wins", []string{"done\nDownloading pkg"}, "Downloading pkg"},
		{"ansi stripped", []string{"\x1b[1;32m✓\x1b[0m \x1b[2mBuilt\x1b[0m in 2s\r\n"}, "Built in 2s"},
		{"cr overwrite", []string{"Progress 10%\rProgress 55%"}, "Progress 55%"},
		{"cr overwrite split", []string{"Progress 10%\rProg", "ress 99%\r"}, "Progress 99%"},
		{"spinner prefix", []string{"⠋ Thinking\r⠙ Thinking\r⠹ Thinking"}, "Thinking"},
		{"spinner only keeps previous", []string{"Fetching deps\n⠋\r⠙\r⠹\r"}, "Fetching deps"},
		{"box drawing skipped", []string{"real line\n╭──────────╮\n│          │\n╰──────────╯\n"}, "real line"},
		{"text inside box", []string{"╭────╮\n│ > Write a test │\n╰────╯\n"}, "Write a test"},
		{"cursor moves split lines", []string{"\x1b[5;1Hfirst\x1b[6;1Hsecond line"}, "second line"},
		{"osc title not text", []string{"hello world\n\x1b]0;my title\x07"}, "hello world"},
		{"progress osc not text", []string{"step 1\n\x1b]9;4;1;40\x07"}, "step 1"},
		{"whitespace collapsed", []string{"  a    lot   of\tspace  \n"}, "a lot of space"},
		{"one letter is noise", []string{"useful\n$ \n"}, "useful"},
		{"utf8 split", []string{"caf\xc3", "\xa9 listo\n"}, "café listo"},
	}
	for _, c := range cases {
		if got := lastLine(c.chunks...); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	long := strings.Repeat("palabra ", 40)
	if got := lastLine(long + "\n"); len([]rune(got)) != maxActivityRunes || !strings.HasSuffix(got, "…") {
		t.Errorf("long line not clipped: %q", got)
	}
}

func TestShellPromptIsNotActivity(t *testing.T) {
	prompts := []string{
		"elionaz@mac ~ % ",
		"elionaz@mac agenterm %",
		"elionaz@mac:~/Dev/agenterm$ ",
		"[elionaz@mac agenterm]$ ",
		"(.venv) elionaz@mac:~/x$ ",
		"root@box:/# ",
		"bash-5.2$ ",
		"~/Dev/agenterm ❯ ",
		"❯ ",
		"agenterm ➜ ",
		"➜  agenterm git:(main) ✗ ",
		"~/Dev/agenterm $ ",
		"$ ",
		"% ",
		"PS C:\\> ",
		"/tmp/>",
	}
	for _, p := range prompts {
		if !looksLikePrompt(p) {
			t.Errorf("%q should look like a prompt", p)
		}
		// After real output, the prompt does not replace it.
		if got := lastLine("go test ./...\r\nok  pkg 0.2s\r\n" + p); got != "ok pkg 0.2s" {
			t.Errorf("after %q: activity %q", p, got)
		}
	}
	notPrompts := []string{
		"Downloading 45%",
		"Progress 55%",
		"<div class=x>",
		"cost: $",
		"see issue #",
		"elionaz@mac ~ % ls -la", // a typed command is input, not idle
		"Compiling agenterm v0.2.0 -> target/debug/agenterm and running a very long build step that ends in >", // too long
		"Deploying to prod --> staging",
		"Thinking",
	}
	for _, p := range notPrompts {
		if looksLikePrompt(p) {
			t.Errorf("%q should not look like a prompt", p)
		}
	}
}

func TestTrackerThrottle(t *testing.T) {
	var tr tracker
	t0 := time.Unix(1_700_000_000, 0)
	if !tr.setOutput("a", t0) || !tr.due(t0, false) {
		t.Fatal("first change is emitted at once")
	}
	// A storm of changes within the interval: one trailing emission.
	emits := 0
	for i := 1; i <= 100; i++ {
		now := t0.Add(time.Duration(i) * 10 * time.Millisecond) // 1 s total
		tr.setOutput(strings.Repeat("x", i+1), now)
		if tr.due(now, false) {
			emits++
		}
	}
	if emits != 2 { // at 500 ms and 1000 ms
		t.Fatalf("emits in 1s = %d, want 2", emits)
	}
	now := t0.Add(1100 * time.Millisecond)
	tr.setOutput("final", now)
	if tr.due(now, false) {
		t.Fatal("too soon")
	}
	now = t0.Add(1500 * time.Millisecond)
	if !tr.due(now, false) || tr.snapshot().Activity != "final" {
		t.Fatal("trailing change must be flushed")
	}
	if tr.due(now.Add(time.Second), false) {
		t.Fatal("nothing pending")
	}
	// Same text: no change.
	if tr.setOutput("final", now) {
		t.Fatal("unchanged text reported as change")
	}
	// Output heartbeat every 5 s even without text changes.
	tr.output()
	if tr.due(now.Add(time.Second), false) {
		t.Fatal("heartbeat too soon")
	}
	if !tr.due(now.Add(heartbeatInterval), false) {
		t.Fatal("heartbeat due")
	}
	// force (stall flip) bypasses the limit.
	if !tr.due(now.Add(heartbeatInterval+time.Millisecond), true) {
		t.Fatal("force")
	}
}

func TestTrackerHookWins(t *testing.T) {
	var tr tracker
	t0 := time.Unix(1_700_000_000, 0)
	tr.setOutput("some output", t0)
	tr.setHook("Leyendo main.go", t0.Add(time.Second))
	if tr.setOutput("more output", t0.Add(2*time.Second)) {
		t.Fatal("output must not override hook activity")
	}
	if tr.evalDue(t0.Add(10 * time.Second)) {
		t.Fatal("no output evaluation once hooks report")
	}
	l := tr.snapshot()
	if l.Activity != "Leyendo main.go" || l.ActivitySource != ActivityHook || !l.ActivityAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("%+v", l)
	}
}

func TestTrackerProgress(t *testing.T) {
	var tr tracker
	t0 := time.Unix(1_700_000_000, 0)
	tr.setProgress(termsig.Progress{State: termsig.ProgressNormal, Value: 30})
	if !tr.due(t0, false) || tr.snapshot().Progress.Value != 30 {
		t.Fatal("progress change emitted")
	}
	tr.setProgress(termsig.Progress{State: termsig.ProgressNormal, Value: 30})
	if tr.due(t0.Add(time.Second), false) {
		t.Fatal("same progress is not a change")
	}
	tr.setProgress(termsig.Progress{State: termsig.ProgressNone, Value: 99})
	if p := tr.snapshot().Progress; p != (termsig.Progress{}) {
		t.Fatalf("cleared progress %+v", p)
	}
}

func TestTailLines(t *testing.T) {
	raw := "\x1b[?25l\x1b[1mtitle\x1b[0m\r\n\r\n\r\n" +
		"line 1\r\nProgress 10%\rProgress 80%\r\n" +
		"\x1b]0;window title\x07tabbed\tcol\r\n" +
		"partial"
	got := TailLines([]byte(raw), 8)
	want := []string{"title", "", "line 1", "Progress 80%", "tabbed  col", "partial"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := TailLines([]byte(raw), 2); !reflect.DeepEqual(got, want[4:]) {
		t.Fatalf("n=2: %q", got)
	}
	if got := TailLines(nil, 8); got == nil || len(got) != 0 {
		t.Fatalf("empty: %#v", got)
	}
	// CR overwrite keeps the longer tail of the old line (like a terminal).
	if got := TailLines([]byte("abcdef\rXY\n"), 8); !reflect.DeepEqual(got, []string{"XYcdef"}) {
		t.Fatalf("overwrite: %q", got)
	}
	// Large streams: only the end is parsed.
	big := strings.Repeat("filler line\n", 20000) + "last\n"
	if got := TailLines([]byte(big), 1); !reflect.DeepEqual(got, []string{"last"}) {
		t.Fatalf("big: %q", got)
	}
}

func TestRingTail(t *testing.T) {
	r := NewRing(8)
	r.Write([]byte("abcdef"))
	r.Write([]byte("ghij")) // wraps
	if got := string(r.Tail(5)); got != "fghij" {
		t.Fatalf("tail %q", got)
	}
	if got := string(r.Tail(100)); got != "cdefghij" {
		t.Fatalf("tail all %q", got)
	}
}
