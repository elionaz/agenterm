package status

import (
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newMachine(t *testing.T, mode Mode) (*Machine, *fakeClock, *[]Change) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	var changes []Change
	m := New(Options{Mode: mode, IdleAfter: 3 * time.Second, Now: clk.Now, OnChange: func(c Change) {
		changes = append(changes, c)
	}})
	return m, clk, &changes
}

func TestHeuristicIdleAfterQuietPeriod(t *testing.T) {
	m, clk, changes := newMachine(t, ModeHeuristic)
	if m.State() != Running {
		t.Fatalf("initial state = %s", m.State())
	}
	clk.Add(2 * time.Second)
	m.Tick()
	if m.State() != Running {
		t.Fatalf("idle too early: %s", m.State())
	}
	m.Output() // resets timer
	clk.Add(2999 * time.Millisecond)
	m.Tick()
	if m.State() != Running {
		t.Fatalf("idle before timeout after output: %s", m.State())
	}
	clk.Add(time.Millisecond)
	m.Tick()
	if m.State() != Idle {
		t.Fatalf("want idle, got %s", m.State())
	}
	m.Output()
	if m.State() != Running {
		t.Fatalf("output should resume running, got %s", m.State())
	}
	if len(*changes) != 2 || (*changes)[0].To != Idle || (*changes)[1].To != Running {
		t.Fatalf("unexpected changes: %+v", *changes)
	}
	if (*changes)[0].Source != SourceHeuristic {
		t.Fatalf("source = %s", (*changes)[0].Source)
	}
}

func TestHeuristicModeEventsThenActivity(t *testing.T) {
	m, _, _ := newMachine(t, ModeHeuristic)
	if err := m.Notify(WaitingInput); err != nil {
		t.Fatal(err)
	}
	if m.State() != WaitingInput {
		t.Fatalf("got %s", m.State())
	}
	// Output alone (spinners, redraws) does not end a wait...
	m.Output()
	if m.State() != WaitingInput {
		t.Fatalf("got %s", m.State())
	}
	// ...user input does, and then the heuristic is back in charge.
	m.Input()
	if m.State() != Running {
		t.Fatalf("got %s", m.State())
	}
}

func TestHooksOverrideHeuristic(t *testing.T) {
	m, clk, _ := newMachine(t, ModeHooks)
	// Before any hook fires the heuristic acts as fallback.
	clk.Add(5 * time.Second)
	m.Tick()
	if m.State() != Idle {
		t.Fatalf("fallback heuristic: got %s", m.State())
	}
	m.Output()
	if m.State() != Running {
		t.Fatalf("fallback heuristic output: got %s", m.State())
	}

	if err := m.Notify(WaitingInput); err != nil {
		t.Fatal(err)
	}
	m.Output() // ignored now
	clk.Add(10 * time.Second)
	m.Tick() // ignored now
	if m.State() != WaitingInput {
		t.Fatalf("hook state overridden: %s", m.State())
	}
	if err := m.Notify(Running); err != nil {
		t.Fatal(err)
	}
	clk.Add(10 * time.Second)
	m.Tick()
	if m.State() != Running {
		t.Fatalf("hook running overridden by idle: %s", m.State())
	}
	if err := m.Notify(Idle); err != nil {
		t.Fatal(err)
	}
	m.Output()
	if m.State() != Idle {
		t.Fatalf("hook idle overridden by output: %s", m.State())
	}
}

func TestExitIsTerminal(t *testing.T) {
	m, clk, changes := newMachine(t, ModeHeuristic)
	m.Exit(3)
	m.Exit(4)
	m.Output()
	clk.Add(time.Hour)
	m.Tick()
	if err := m.Notify(Running); err == nil {
		t.Fatal("notify after exit should fail")
	}
	code, exited := m.ExitCode()
	if !exited || code != 3 || m.State() != Exited {
		t.Fatalf("code=%d exited=%v state=%s", code, exited, m.State())
	}
	last := (*changes)[len(*changes)-1]
	if last.To != Exited || last.ExitCode != 3 || last.Source != SourceExit {
		t.Fatalf("bad exit change %+v", last)
	}
	if len(*changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(*changes))
	}
}

func TestNotifyRejectsExited(t *testing.T) {
	m, _, _ := newMachine(t, ModeHooks)
	if err := m.Notify(Exited); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseState(t *testing.T) {
	cases := map[string]State{"running": Running, "waiting": WaitingInput, "waiting_input": WaitingInput, "done": Idle, "IDLE": Idle}
	for in, want := range cases {
		got, err := ParseState(in)
		if err != nil || got != want {
			t.Errorf("ParseState(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseState("bogus"); err == nil {
		t.Error("expected error")
	}
}

func TestSuspendedIsInert(t *testing.T) {
	var changes []Change
	m := New(Options{Suspended: true, OnChange: func(c Change) { changes = append(changes, c) }})
	if m.State() != Suspended {
		t.Fatalf("state %s", m.State())
	}
	m.Output()
	m.Input()
	m.Signal(KindQuestion, "?")
	m.Tick()
	m.Exit(1)
	if err := m.Notify(Running); err == nil {
		t.Fatal("notify accepted")
	}
	if _, exited := m.ExitCode(); exited || m.State() != Suspended || len(changes) != 0 {
		t.Fatalf("state %s exited %v changes %+v", m.State(), exited, changes)
	}
}
