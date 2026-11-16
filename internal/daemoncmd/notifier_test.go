package daemoncmd

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/desktopnotify"
)

// TestMain: no test here posts a real notification (tests that need the
// switch on only use notify.fallback: none outside a bundle).
func TestMain(m *testing.M) {
	os.Setenv(desktopnotify.EnvNotify, "off")
	os.Exit(m.Run())
}

func testNotifier(t *testing.T) (*notifier, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	// Not inside an .app bundle: native notifications are unavailable.
	return newNotifier(log, "/usr/local/bin/agenterm"), &buf
}

func TestNotifierFallbackNoneDropsAndLogsOnce(t *testing.T) {
	t.Setenv(desktopnotify.EnvNotify, "")
	n, buf := testNotifier(t)
	if n.off || n.native {
		t.Fatalf("off=%v native=%v", n.off, n.native)
	}
	for range 3 {
		n.Notify(daemon.Notification{Title: "t", Body: "b", SessionID: "s1", Kind: "permission"})
	}
	out := buf.String()
	if c := strings.Count(out, "notifications not shown"); c != 1 {
		t.Fatalf("logged %d times:\n%s", c, out)
	}
	if !strings.Contains(out, "no app bundle") || strings.Contains(out, "notification posted") {
		t.Fatalf("log:\n%s", out)
	}
}

func TestNotifierKillSwitch(t *testing.T) {
	t.Setenv(desktopnotify.EnvNotify, "off")
	n, buf := testNotifier(t)
	if !n.off {
		t.Fatal("AGENTERM_NOTIFY=off not honored")
	}
	// Even with the osascript fallback allowed nothing is posted.
	n.Notify(daemon.Notification{Title: "t", Body: "b", SessionID: "s1", Kind: "permission", Osascript: true})
	out := buf.String()
	if !strings.Contains(out, "notifications disabled") || strings.Contains(out, "notification posted") || strings.Contains(out, "not shown") {
		t.Fatalf("log:\n%s", out)
	}
}
