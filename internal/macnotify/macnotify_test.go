package macnotify

import "testing"

func TestBundlePath(t *testing.T) {
	cases := map[string]string{
		"/Applications/panal.app/Contents/MacOS/panal": "/Applications/panal.app",
		"/x/My Apps/panal.app/Contents/MacOS/panal":    "/x/My Apps/panal.app",
		"/usr/local/bin/panal":                         "",
		"/x/panal.app/Contents/MacOS/sub/panal":        "",
		"/x/panal.app/Contents/Resources/panal":        "",
	}
	for exe, want := range cases {
		if got := BundlePath(exe); got != want {
			t.Errorf("BundlePath(%q) = %q, want %q", exe, got, want)
		}
	}
}

func TestSessionURL(t *testing.T) {
	u := SessionURL("a1b2c3d4")
	if u != "panal://session/a1b2c3d4" {
		t.Fatalf("url %q", u)
	}
	if id, ok := ParseSessionURL(u); !ok || id != "a1b2c3d4" {
		t.Fatalf("parse %q -> %q %v", u, id, ok)
	}
	for _, bad := range []string{"", "panal://session/", "panal://other/x", "http://session/x", "panal://session/../../etc", "panal://session/a%20b"} {
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
