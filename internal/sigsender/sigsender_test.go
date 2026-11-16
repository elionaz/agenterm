package sigsender

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// SIGUSR2 keeps the test runner's own SIGTERM/SIGINT handling intact.
func TestNotifyReportsSender(t *testing.T) {
	ch, ok, err := Notify(syscall.SIGUSR2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("sigsender unsupported on this platform/build")
	}

	// From ourselves.
	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatal(err)
	}
	ev := recv(t, ch)
	if ev.Signal != syscall.SIGUSR2 || ev.PID != os.Getpid() || ev.UID != os.Getuid() {
		t.Fatalf("self: got %+v, want sig=%d pid=%d uid=%d", ev, syscall.SIGUSR2, os.Getpid(), os.Getuid())
	}

	// From another process: the sender is kill(1), not us.
	cmd := exec.Command("/bin/kill", "-USR2", strconv.Itoa(os.Getpid()))
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	ev = recv(t, ch)
	if ev.PID == os.Getpid() || ev.PID != cmd.Process.Pid {
		t.Fatalf("external: got pid %d, want kill's pid %d", ev.PID, cmd.Process.Pid)
	}
}

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no signal event received")
		return Event{}
	}
}
