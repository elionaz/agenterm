package daemoncmd

import (
	"bytes"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/desktopnotify"
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
	n := newNotifier(log, "/usr/local/bin/panal")
	// Nor the desktop's on Linux: tests never post real notifications.
	if n.linux != nil {
		n.linux.Close()
		n.linux = nil
	}
	return n, &buf
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
	if !strings.Contains(out, wantReason()) || strings.Contains(out, "notification posted") {
		t.Fatalf("log:\n%s", out)
	}
}

func TestNotifierKillSwitch(t *testing.T) {
	t.Setenv(desktopnotify.EnvNotify, "off")
	n, buf := testNotifier(t)
	if !n.off {
		t.Fatal("PANAL_NOTIFY=off not honored")
	}
	// Even with the osascript fallback allowed nothing is posted.
	n.Notify(daemon.Notification{Title: "t", Body: "b", SessionID: "s1", Kind: "permission", Osascript: true})
	out := buf.String()
	if !strings.Contains(out, "notifications disabled") || strings.Contains(out, "notification posted") || strings.Contains(out, "not shown") {
		t.Fatalf("log:\n%s", out)
	}
}

// wantReason is why a test notifier drops notifications on this OS.
func wantReason() string {
	if runtime.GOOS == "linux" {
		return "no desktop notification service"
	}
	return "no app bundle"
}
