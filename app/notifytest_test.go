package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/paths"
)

func TestTestNotification(t *testing.T) {
	dir := t.TempDir()
	a := NewApp()
	a.paths = paths.In(dir)
	fd := &fakeDesktop{}
	a.desk = fd

	// Allowed: posted with the UI's text, whatever window is in front.
	fd.active = true
	if err := a.TestNotification("agenterm", "It works"); err != nil {
		t.Fatal(err)
	}
	notes, _, _ := fd.snapshot()
	if len(notes) != 1 || notes[0].Title != "agenterm" || notes[0].Body != "It works" || notes[0].Osascript {
		t.Fatalf("notes = %+v", notes)
	}

	// Not allowed: says why, posts nothing.
	fd.reason = "not authorized (denied)"
	err := a.TestNotification("agenterm", "again")
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v", err)
	}
	if notes, _, _ := fd.snapshot(); len(notes) != 1 {
		t.Fatalf("posted while denied: %+v", notes)
	}

	// With the AppleScript fallback on, it goes that way instead.
	os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("notify:\n  fallback: osascript\n"), 0o600)
	if err := a.TestNotification("agenterm", "fallback"); err != nil {
		t.Fatal(err)
	}
	if notes, _, _ := fd.snapshot(); len(notes) != 2 || !notes[1].Osascript {
		t.Fatalf("fallback notes = %+v", notes)
	}

	// A post that fails is reported, not "sent".
	fd.reason = ""
	os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("notify:\n  sound: true\n"), 0o600)
	fd.postErr = errors.New("timed out posting notification")
	if err := a.TestNotification("agenterm", "lost"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("post error: %v", err)
	}
	// It plays the sound an approval or a question would.
	fd.postErr = nil
	if err := a.TestNotification("agenterm", "sound"); err != nil {
		t.Fatal(err)
	}
	if notes, _, _ := fd.snapshot(); !notes[len(notes)-1].Sound {
		t.Fatalf("test notification without the question sound: %+v", notes[len(notes)-1])
	}
}
