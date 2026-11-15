package shellenv

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

func TestJoinPaths(t *testing.T) {
	if got := JoinPaths("/a:/b", "", "/b::/c:/a"); got != "/a:/b:/c" {
		t.Fatalf("got %q", got)
	}
}
