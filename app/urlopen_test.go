package main

import "testing"

func TestOpenURLColdLaunchKeepsFocusForFrontend(t *testing.T) {
	a := NewApp()
	rec := &recorder{}
	a.emit = rec.emit

	// Before startup (cold launch by URL): only remembered.
	a.openURL("panal://session/a1b2c3d4")
	a.openURL("https://evil.example/x") // ignored
	if rec.has(evFocusSession) {
		t.Fatal("focus emitted before startup")
	}
	a.focusMu.Lock()
	a.started = true
	a.focusMu.Unlock()
	if id := a.TakePendingFocus(); id != "a1b2c3d4" {
		t.Fatalf("pending focus %q", id)
	}
	if id := a.TakePendingFocus(); id != "" {
		t.Fatalf("pending focus not cleared: %q", id)
	}

	// Frontend ready: focus right away, nothing pending.
	a.openURL("panal://session/beef0001")
	if !rec.has(evFocusSession) {
		t.Fatal("focus not emitted once running")
	}
	if id := a.TakePendingFocus(); id != "" {
		t.Fatalf("unexpected pending %q", id)
	}
}
