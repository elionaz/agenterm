package main

import "testing"

func TestExtractMarked(t *testing.T) {
	out := []byte("motd noise\n" + pathMarker + "/opt/homebrew/bin:/usr/bin" + pathMarker + "\nbye")
	if got := extractMarked(out); got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("got %q", got)
	}
	if got := extractMarked([]byte("no markers")); got != "" {
		t.Fatalf("got %q", got)
	}
}
