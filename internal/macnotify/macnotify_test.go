package macnotify

import "testing"

func TestBundlePath(t *testing.T) {
	cases := map[string]string{
		"/Applications/agenterm.app/Contents/MacOS/agenterm": "/Applications/agenterm.app",
		"/x/My Apps/agenterm.app/Contents/MacOS/agenterm":    "/x/My Apps/agenterm.app",
		"/usr/local/bin/agenterm":                            "",
		"/x/agenterm.app/Contents/MacOS/sub/agenterm":        "",
		"/x/agenterm.app/Contents/Resources/agenterm":        "",
	}
	for exe, want := range cases {
		if got := BundlePath(exe); got != want {
			t.Errorf("BundlePath(%q) = %q, want %q", exe, got, want)
		}
	}
}

func TestSessionURL(t *testing.T) {
	u := SessionURL("a1b2c3d4")
	if u != "agenterm://session/a1b2c3d4" {
		t.Fatalf("url %q", u)
	}
	if id, ok := ParseSessionURL(u); !ok || id != "a1b2c3d4" {
		t.Fatalf("parse %q -> %q %v", u, id, ok)
	}
	for _, bad := range []string{"", "agenterm://session/", "agenterm://other/x", "http://session/x", "agenterm://session/../../etc", "agenterm://session/a%20b"} {
		if id, ok := ParseSessionURL(bad); ok {
			t.Errorf("%q parsed as %q", bad, id)
		}
	}
}

func TestRunMainLoopReturns(t *testing.T) {
	ran := false
	RunMainLoop(func() { ran = true }) // test goroutine: not the main thread
	if !ran {
		t.Fatal("work not run")
	}
}
