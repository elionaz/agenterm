package status

import (
	"testing"
	"time"
)

func newStallMachine(t *testing.T, mode Mode, stall time.Duration) (*Machine, *fakeClock, *[]bool) {
	t.Helper()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	var flips []bool
	m := New(Options{Mode: mode, IdleAfter: 3 * time.Second, Now: clk.Now, StallAfter: stall,
		OnStall: func(v bool) { flips = append(flips, v) }})
	return m, clk, &flips
}

func TestStallAfterQuietHookTurn(t *testing.T) {
	m, clk, flips := newStallMachine(t, ModeHooks, 180*time.Second)
	m.NotifyEvent(Event{State: Running, Turn: true})
	clk.Add(179 * time.Second)
	m.Tick()
	if m.Stalled() {
		t.Fatal("stalled too early")
	}
	m.Output() // output resets the clock
	clk.Add(179 * time.Second)
	m.Tick()
	if m.Stalled() {
		t.Fatal("output should reset the stall timer")
	}
	clk.Add(time.Second)
	m.Tick()
	if !m.Stalled() || m.State() != Running {
		t.Fatalf("want stalled running, got stalled=%v %s", m.Stalled(), m.State())
	}
	if tm := m.Timers(); tm.StalledAt != clk.Now() {
		t.Fatalf("stalledAt %v", tm.StalledAt)
	}
	m.Tick() // no second flip
	// Any event clears it.
	m.NotifyEvent(Event{State: Running})
	if m.Stalled() {
		t.Fatal("event should clear stall")
	}
	clk.Add(200 * time.Second)
	m.Tick()
	m.Output()
	if m.Stalled() {
		t.Fatal("output should clear stall")
	}
	want := []bool{true, false, true, false}
	if len(*flips) != len(want) {
		t.Fatalf("flips %v", *flips)
	}
	for i := range want {
		if (*flips)[i] != want[i] {
			t.Fatalf("flips %v", *flips)
		}
	}
}

func TestStallClearedWhenTurnEnds(t *testing.T) {
	m, clk, _ := newStallMachine(t, ModeHooks, time.Minute)
	m.NotifyEvent(Event{State: Running})
	clk.Add(2 * time.Minute)
	m.Tick()
	if !m.Stalled() {
		t.Fatal("want stalled")
	}
	m.NotifyEvent(Event{State: Idle})
	if m.Stalled() {
		t.Fatal("idle clears stall")
	}
	clk.Add(10 * time.Minute)
	m.Tick()
	if m.Stalled() {
		t.Fatal("idle sessions never stall")
	}
}

func TestNoStallWithoutHookTurn(t *testing.T) {
	// Heuristic sessions: quiet means idle, never stalled.
	m, clk, flips := newStallMachine(t, ModeHeuristic, time.Minute)
	m.Output()
	clk.Add(5 * time.Minute)
	m.Tick()
	if m.Stalled() || len(*flips) != 0 {
		t.Fatal("heuristic session stalled")
	}
	// Hooks profile before its first event: no open turn either.
	m2, clk2, _ := newStallMachine(t, ModeHooks, time.Minute)
	clk2.Add(2 * time.Second)
	m2.Output()
	clk2.Add(5 * time.Minute)
	m2.Tick()
	if m2.Stalled() {
		t.Fatal("no open turn yet")
	}
	// Disabled.
	m3, clk3, _ := newStallMachine(t, ModeHooks, 0)
	m3.NotifyEvent(Event{State: Running})
	clk3.Add(time.Hour)
	m3.Tick()
	if m3.Stalled() {
		t.Fatal("stall disabled")
	}
}

func TestTurnTimers(t *testing.T) {
	m, clk, _ := newMachine(t, ModeHooks)
	start := clk.Now()
	if tm := m.Timers(); tm.TurnStartedAt != start || !tm.TurnEndedAt.IsZero() {
		t.Fatalf("initial timers %+v", tm)
	}
	clk.Add(10 * time.Second)
	m.NotifyEvent(Event{State: Running, Turn: true}) // UserPromptSubmit while running
	t1 := clk.Now()
	if tm := m.Timers(); tm.TurnStartedAt != t1 || tm.LastEventAt != t1 {
		t.Fatalf("turn restart %+v", tm)
	}
	clk.Add(time.Minute)
	m.NotifyEvent(Event{State: WaitingInput, Kind: KindPermission})
	clk.Add(time.Minute)
	m.NotifyEvent(Event{State: Running}) // approved: same turn
	if tm := m.Timers(); tm.TurnStartedAt != t1 {
		t.Fatalf("approval restarted the turn: %+v", tm)
	}
	clk.Add(time.Minute)
	m.NotifyEvent(Event{State: Idle})
	end := clk.Now()
	if tm := m.Timers(); tm.TurnEndedAt != end || tm.TurnStartedAt != t1 {
		t.Fatalf("turn end %+v", tm)
	}
	clk.Add(time.Minute)
	m.NotifyEvent(Event{State: Running})
	if tm := m.Timers(); tm.TurnStartedAt != clk.Now() || !tm.TurnEndedAt.IsZero() {
		t.Fatalf("idle → running starts a turn: %+v", tm)
	}
}

func TestHeuristicTurnTimersAndOutput(t *testing.T) {
	m, clk, changes := newMachine(t, ModeHeuristic)
	clk.Add(4 * time.Second)
	m.Tick() // idle
	idleAt := clk.Now()
	clk.Add(time.Second)
	m.Output()
	tm := m.Timers()
	if tm.TurnStartedAt != clk.Now() || tm.LastOutputAt != clk.Now() || !tm.TurnEndedAt.IsZero() {
		t.Fatalf("timers %+v", tm)
	}
	last := (*changes)[len(*changes)-1]
	if last.Timers.TurnStartedAt != clk.Now() {
		t.Fatalf("change carries timers: %+v", last.Timers)
	}
	if (*changes)[0].Timers.TurnEndedAt != idleAt {
		t.Fatalf("idle change: %+v", (*changes)[0].Timers)
	}
}
