package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a fake Chrome: with AGENTERM_FAKE_CHROME set
// it serves /json/version on its --remote-debugging-port.
func TestMain(m *testing.M) {
	if os.Getenv("AGENTERM_FAKE_CHROME") != "" {
		fakeChrome()
		return
	}
	os.Exit(m.Run())
}

func fakeChrome() {
	port := ""
	for _, a := range os.Args[1:] {
		if v, ok := strings.CutPrefix(a, "--remote-debugging-port="); ok {
			port = v
		}
	}
	http.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"Browser":"Fake/1.0"}`)
	})
	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "chrome")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Find(exe, nil); got != exe {
		t.Fatalf("explicit path = %q", got)
	}
	if got := Find(filepath.Join(dir, "missing"), nil); got != "" {
		t.Fatalf("missing explicit path = %q", got)
	}
	plain := filepath.Join(dir, "plain")
	_ = os.WriteFile(plain, nil, 0o644)
	if got := Find(plain, nil); got != "" {
		t.Fatalf("non-executable = %q", got)
	}
}

func TestArgs(t *testing.T) {
	s := Spec{Exe: "/x/Chrome", Profile: "/p/s1", Port: 9333}
	a := s.ChromeArgs()
	for _, want := range []string{"--user-data-dir=/p/s1", "--remote-debugging-port=9333", "--remote-debugging-address=127.0.0.1", "--window-size=1280,800", "--no-first-run"} {
		if !slices.Contains(a, want) {
			t.Errorf("ChromeArgs lacks %s: %q", want, a)
		}
	}
	if slices.Contains(a, "--headless=new") {
		t.Error("headless by default")
	}
	s.Headless = true
	if !slices.Contains(s.ChromeArgs(), "--headless=new") {
		t.Error("headless missing")
	}
	if s.URL() != "http://127.0.0.1:9333" {
		t.Errorf("URL = %s", s.URL())
	}

	attach := s.MCPArgs(true)
	if !slices.Contains(attach, MCPPackage) || !slices.Contains(attach, "--no-usage-statistics") || !slices.Contains(attach, "--no-performance-crux") ||
		attach[len(attach)-2] != "--browserUrl" || attach[len(attach)-1] != s.URL() {
		t.Fatalf("attach = %q", attach)
	}
	launch := strings.Join(s.MCPArgs(false), " ")
	for _, want := range []string{"--executablePath /x/Chrome", "--userDataDir /p/s1", "--viewport 1280x800", "--chromeArg=--remote-debugging-port=9333", "--headless"} {
		if !strings.Contains(launch, want) {
			t.Errorf("launch lacks %q: %s", want, launch)
		}
	}
}

func TestMatchProfile(t *testing.T) {
	ps := "  101 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=/h/browsers/s1 --no-first-run\n" +
		"  102 /Applications/Google Chrome.app/Contents/Frameworks/Helper --type=renderer --user-data-dir=/h/browsers/s1\n" +
		"  103 Google Chrome --user-data-dir=/h/browsers/s10\n" +
		"  104 Google Chrome\n" +
		"  105 agenterm --user-data-dir=/h/browsers/s1\n"
	if got := matchProfile(ps, "/h/browsers/s1", 105); !slices.Equal(got, []int{101, 102}) {
		t.Fatalf("pids = %v", got)
	}
}

func TestLaunchStop(t *testing.T) {
	t.Setenv("AGENTERM_FAKE_CHROME", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s := Spec{Exe: exe, Profile: filepath.Join(t.TempDir(), "prof"), Port: freePort(t)}
	if Alive(ctx, s.Port) || !PortFree(s.Port) {
		t.Fatal("port busy before launch")
	}
	p, err := Launch(ctx, s, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !Alive(ctx, s.Port) {
		t.Fatal("not alive after launch")
	}
	if fi, err := os.Stat(s.Profile); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("profile dir: %v %v", fi, err)
	}
	if pids := ProfileProcs(ctx, s.Profile); !slices.Contains(pids, p.Pid) {
		t.Fatalf("ProfileProcs = %v, want %d", pids, p.Pid)
	}
	Stop(ctx, s.Profile, 3*time.Second)
	deadline := time.Now().Add(3 * time.Second)
	for Alive(ctx, s.Port) {
		if time.Now().After(deadline) {
			t.Fatal("still alive after Stop")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := Launch(ctx, Spec{Profile: s.Profile, Port: s.Port}, time.Second); err == nil {
		t.Fatal("launched without an executable")
	}
}
