package daemon_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/browser"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/protocol"
)

// fakeChrome serves /json/version on --remote-debugging-port (TestMain
// runs it when AGENTERM_FAKE_CHROME is set).
func fakeChrome() {
	port := ""
	for _, a := range os.Args[1:] {
		if v, ok := strings.CutPrefix(a, "--remote-debugging-port="); ok {
			port = v
		}
	}
	http.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{}`) })
	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestBrowser(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTERM_FAKE_CHROME", "1") // what Launch starts serves DevTools
	defer daemon.SetBrowser(exe)()
	// npx on the search PATH: sessions with an integration get the MCP server.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "npx"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	testExtraPath.Store(bin)
	defer testExtraPath.Store("")

	c, p, stop := startDaemon(t)
	defer stop()
	writeProfiles(t, p.Profiles(), "- id: hookedcodex\n  cmd: /bin/echo\n  setup_hooks: codex\n")
	ctx := context.Background()
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "web")
	if err != nil {
		t.Fatal(err)
	}

	st, err := c.BrowserStatus(ctx)
	if err != nil || !st.Enabled || st.Exe != exe || st.NPX == "" || st.MCP != browser.MCPPackage {
		t.Fatalf("status = %+v, %v", st, err)
	}

	// Claude gets the MCP server before --settings; codex as -c entries.
	cl, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hookedclaude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cl.Args) < 4 || cl.Args[0] != "--mcp-config" || cl.Args[2] != "--settings" {
		t.Fatalf("claude args = %q", cl.Args)
	}
	cfg, err := os.ReadFile(cl.Args[1])
	if err != nil || !strings.Contains(string(cfg), `"agenterm-browser"`) || !strings.Contains(string(cfg), `"browser","mcp","--sock"`) {
		t.Fatalf("mcp config %s: %v", cfg, err)
	}
	cx, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hookedcodex"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(cx.Args, func(a string) bool { return strings.HasPrefix(a, "mcp_servers.agenterm-browser.command=") }) ||
		!slices.ContainsFunc(cx.Args, func(a string) bool {
			return strings.HasPrefix(a, "developer_instructions=") && strings.Contains(a, "agenterm browser")
		}) {
		t.Fatalf("codex args = %q", cx.Args)
	}

	// A shell session: prepare reserves, open starts, close stops.
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := c.Browser(ctx, protocol.MethodBrowserPrepare, sh.ID)
	if err != nil || prep.Running || prep.Port < browser.BasePort || prep.Profile != filepath.Join(p.RuntimeDir, "browsers", sh.ID) {
		t.Fatalf("prepare = %+v, %v", prep, err)
	}
	other, _ := c.Browser(ctx, protocol.MethodBrowserPrepare, cl.ID)
	if other.Port == prep.Port {
		t.Fatalf("two sessions share port %d", prep.Port)
	}
	open, err := c.Browser(ctx, protocol.MethodBrowserOpen, sh.ID)
	if err != nil || !open.Running || open.URL != prep.URL || !browser.Alive(ctx, open.Port) {
		t.Fatalf("open = %+v, %v", open, err)
	}
	if s := findSession(t, c, sh.ID); s.BrowserURL != open.URL {
		t.Fatalf("session browserUrl = %q", s.BrowserURL)
	}
	if again, err := c.Browser(ctx, protocol.MethodBrowserOpen, sh.ID); err != nil || again.Port != open.Port {
		t.Fatalf("second open = %+v, %v", again, err)
	}
	if _, err := c.Browser(ctx, protocol.MethodBrowserClose, sh.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "browser closed", func() bool { return !browser.Alive(ctx, open.Port) })
	if s := findSession(t, c, sh.ID); s.BrowserURL != "" {
		t.Fatalf("closed browser still reported: %q", s.BrowserURL)
	}

	// The session exiting stops its browser; removing it deletes the profile.
	if _, err := c.Browser(ctx, protocol.MethodBrowserOpen, sh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Kill(ctx, sh.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "browser stopped on exit", func() bool { return !browser.Alive(ctx, open.Port) })
	waitFor(t, "session exited", func() bool { return findSession(t, c, sh.ID).State == "exited" })
	if _, err := os.Stat(open.Profile); err != nil {
		t.Fatalf("profile gone before removal: %v", err)
	}
	if _, err := c.Kill(ctx, sh.ID); err != nil { // removes the exited session
		t.Fatal(err)
	}
	waitFor(t, "profile removed", func() bool { _, err := os.Stat(open.Profile); return os.IsNotExist(err) })

	if _, err := c.Browser(ctx, protocol.MethodBrowserOpen, "nope"); err == nil {
		t.Fatal("opened a browser for an unknown session")
	}
}

func TestBrowserOff(t *testing.T) {
	c, p, stop := startDaemon(t)
	defer stop()
	if err := os.WriteFile(p.Settings(), []byte("browser:\n  enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "web")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Browser(ctx, protocol.MethodBrowserOpen, sh.ID); err == nil || !strings.Contains(err.Error(), "off") {
		t.Fatalf("open with the browser off: %v", err)
	}
	cl, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "hookedclaude"})
	if err != nil || slices.Contains(cl.Args, "--mcp-config") {
		t.Fatalf("claude args with the browser off = %q, %v", cl.Args, err)
	}
}
