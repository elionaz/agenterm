package status

import (
	"testing"
	"time"
)

func last(t *testing.T, changes *[]Change) Change {
	t.Helper()
	if len(*changes) == 0 {
		t.Fatal("no changes")
	}
	return (*changes)[len(*changes)-1]
}

func TestWaitingRaisesAttentionAndAckClearsUnread(t *testing.T) {
	m, _, changes := newMachine(t, ModeHooks)
	if err := m.NotifyEvent(Event{State: WaitingInput, Kind: KindPermission, Message: "Bash: rm -rf x"}); err != nil {
		t.Fatal(err)
	}
	c := last(t, changes)
	if !c.Raised || c.To != WaitingInput || c.Attention.Kind != KindPermission || c.Attention.Message != "Bash: rm -rf x" || !c.Attention.Unread {
		t.Fatalf("bad change %+v", c)
	}
	// The same wait reported again (Notification after PermissionRequest)
	// does not raise twice.
	n := len(*changes)
	_ = m.NotifyEvent(Event{State: WaitingInput, Kind: KindPermission, Message: "Claude needs your permission"})
	if len(*changes) != n {
		t.Fatalf("duplicate wait produced a change: %+v", last(t, changes))
	}
	if !m.Ack() {
		t.Fatal("ack should change")
	}
	c = last(t, changes)
	if c.Raised || c.From != c.To || c.Attention.Unread || c.Source != SourceAck || c.Attention.Kind != KindPermission {
		t.Fatalf("bad ack change %+v", c)
	}
	if m.Ack() {
		t.Fatal("second ack should be a no-op")
	}
	// Running clears the attention.
	_ = m.Notify(Running)
	if a := m.Attention(); a.Active() {
		t.Fatalf("attention survived running: %+v", a)
	}
}

func TestIdleAfterTurnIsDone(t *testing.T) {
	m, _, changes := newMachine(t, ModeHooks)
	_ = m.Notify(Running)
	if err := m.NotifyEvent(Event{State: Idle, Message: "Listo"}); err != nil {
		t.Fatal(err)
	}
	c := last(t, changes)
	if !c.Raised || c.To != Idle || c.Attention.Kind != KindDone || c.Attention.Message != "Listo" {
		t.Fatalf("bad change %+v", c)
	}
	// idle -> idle without a kind is not news.
	n := len(*changes)
	_ = m.Notify(Idle)
	if len(*changes) != n {
		t.Fatal("idle->idle raised")
	}
	// An explicit kind (one turn-complete per turn) raises again.
	_ = m.NotifyEvent(Event{State: Idle, Kind: KindDone, Message: "otra"})
	if c := last(t, changes); !c.Raised || c.Attention.Message != "otra" {
		t.Fatalf("bad change %+v", c)
	}
	_ = m.NotifyEvent(Event{State: Idle, Kind: KindError, Message: "rate_limit"})
	if c := last(t, changes); !c.Raised || c.Attention.Kind != KindError {
		t.Fatalf("bad change %+v", c)
	}
}

func TestHeuristicIdleIsNotAttention(t *testing.T) {
	m, clk, changes := newMachine(t, ModeHeuristic)
	clk.Add(5 * time.Second)
	m.Tick()
	if c := last(t, changes); c.To != Idle || c.Raised || c.Attention.Active() {
		t.Fatalf("heuristic idle raised attention: %+v", c)
	}
}

func TestSignalHeuristicMode(t *testing.T) {
	m, clk, changes := newMachine(t, ModeHeuristic)
	m.Signal("", "")
	c := last(t, changes)
	if !c.Raised || c.To != WaitingInput || c.Source != SourceSignal || c.Attention.Kind != KindQuestion {
		t.Fatalf("bad change %+v", c)
	}
	// A message arriving later fills it in without raising again.
	m.Signal(KindQuestion, "Need approval")
	c = last(t, changes)
	if c.Raised || c.Attention.Message != "Need approval" {
		t.Fatalf("bad change %+v", c)
	}
	// Output and ticks don't end the wait.
	m.Output()
	clk.Add(time.Hour)
	m.Tick()
	if m.State() != WaitingInput {
		t.Fatalf("got %s", m.State())
	}
	m.Input()
	if m.State() != Running || m.Attention().Active() {
		t.Fatalf("input should resume: %s %+v", m.State(), m.Attention())
	}
}

func TestSignalHooksMode(t *testing.T) {
	m, _, changes := newMachine(t, ModeHooks)
	_ = m.NotifyEvent(Event{State: Idle, Kind: KindDone, Message: "hook"})
	m.Signal(KindQuestion, "osc")
	if c := last(t, changes); c.Attention.Message != "hook" {
		t.Fatalf("signal overrode hook message: %+v", c)
	}
	m.Ack()
	n := len(*changes)
	m.Signal(KindQuestion, "late bell")
	if len(*changes) != n {
		t.Fatal("signal after hook idle should be ignored")
	}
	// While running (mid-turn) a signal is the only hint we get.
	_ = m.Notify(Running)
	m.Signal(KindQuestion, "approve?")
	if c := last(t, changes); !c.Raised || c.To != WaitingInput || c.Attention.Message != "approve?" {
		t.Fatalf("bad change %+v", c)
	}
	// Typing answers it.
	m.Input()
	if m.State() != Running {
		t.Fatalf("got %s", m.State())
	}
}

func TestInputAcksHookWait(t *testing.T) {
	m, _, changes := newMachine(t, ModeHooks)
	_ = m.NotifyEvent(Event{State: WaitingInput, Kind: KindPermission})
	m.Input()
	c := last(t, changes)
	if c.To != WaitingInput || c.Attention.Unread || c.Source != SourceInput {
		t.Fatalf("bad change %+v", c)
	}
}

func TestParseKind(t *testing.T) {
	for in, want := range map[string]Kind{"permission": KindPermission, "approval": KindPermission, "question": KindQuestion, "DONE": KindDone, "error": KindError, "": ""} {
		if got, err := ParseKind(in); err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseKind("nope"); err == nil {
		t.Error("expected error")
	}
}

func TestHeuristicDoneSurvivesFinalRedraw(t *testing.T) {
	m, clk, _ := newMachine(t, ModeHeuristic)
	_ = m.NotifyEvent(Event{State: Idle, Kind: KindDone, Message: "ok"})
	m.Output()
	if m.State() != Idle || !m.Attention().Unread {
		t.Fatalf("redraw swallowed done: %s", m.State())
	}
	clk.Add(DoneGrace)
	m.Output()
	if m.State() != Running || m.Attention().Active() {
		t.Fatalf("later output should resume running: %s", m.State())
	}
}
