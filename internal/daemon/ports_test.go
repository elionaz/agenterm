package daemon_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/protocol"
)

// envOf has run write the port variables of a shell session to a file
// and returns them as "PORT VITE_PORT AGENTERM_PORT" ("-" when unset).
func envOf(t *testing.T, run func(cmd string), dir, name string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	run(`printf '%s %s %s' "${PORT:--}" "${VITE_PORT:--}" "${AGENTERM_PORT:--}" > ` + out)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSessionPorts(t *testing.T) {
	c, p, stop := startDaemon(t)
	ctx := ctxT(t)
	repo := tempRepo(t)
	// Somewhere unlikely to be busy on a dev machine; base+1 is taken.
	base := 47310
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", base+1))
	if err != nil {
		t.Skipf("port %d busy: %v", base+1, err)
	}
	defer l.Close()
	os.WriteFile(filepath.Join(repo, ".agenterm.yaml"), []byte(fmt.Sprintf("ports:\n  base: %d\n  env: [PORT, VITE_PORT]\n", base)), 0o644)
	proj, err := c.ProjectAdd(ctx, repo, "ports")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	spawn := func(projectID string) protocol.SessionInfo {
		t.Helper()
		info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: projectID, ProfileID: "shell"})
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	vars := func(id, name string) string {
		t.Helper()
		return envOf(t, func(cmd string) { runIn(t, c, id, cmd) }, out, name)
	}

	a, b := spawn(proj.ID), spawn(proj.ID)
	if a.Port != base || b.Port != base+2 {
		t.Fatalf("ports %d, %d; want %d, %d (%d is busy)", a.Port, b.Port, base, base+2, base+1)
	}
	want := func(port int) string { return fmt.Sprintf("%d %d %d", port, port, port) }
	if got := vars(a.ID, "a"); got != want(base) {
		t.Fatalf("a env = %q", got)
	}
	if got := vars(b.ID, "b"); got != want(base+2) {
		t.Fatalf("b env = %q", got)
	}

	// A project that did not ask: AGENTERM_PORT only, nothing shown.
	plain, err := c.ProjectAdd(ctx, t.TempDir(), "plain")
	if err != nil {
		t.Fatal(err)
	}
	q := spawn(plain.ID)
	if q.Port != 0 {
		t.Fatalf("port shown without ports.base: %d", q.Port)
	}
	if got := vars(q.ID, "q"); !strings.HasPrefix(got, "- - ") || strings.HasSuffix(got, " -") {
		t.Fatalf("plain env = %q (want only AGENTERM_PORT)", got)
	}

	// Resumed after a daemon restart: the same port.
	stop()
	c, stop = restartDaemon(t, p)
	defer stop()
	if _, err := c.Resume(ctx, b.ID, false); err != nil {
		t.Fatal(err)
	}
	if s := findSession(t, c, b.ID); s.Port != base+2 {
		t.Fatalf("resumed port = %d", s.Port)
	}
	if got := vars(b.ID, "b2"); got != want(base+2) {
		t.Fatalf("resumed env = %q", got)
	}
}
