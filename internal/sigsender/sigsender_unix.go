//go:build (darwin || linux) && cgo

package sigsender

/*
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <string.h>
#include <unistd.h>

typedef struct { int sig; int pid; int uid; } agt_sigev;

static int agt_pipe[2] = {-1, -1};

// Async-signal-safe: only write(2) and errno.
static void agt_handler(int sig, siginfo_t *info, void *uctx) {
	int saved = errno;
	agt_sigev ev;
	ev.sig = sig;
	ev.pid = info ? (int)info->si_pid : 0;
	ev.uid = info ? (int)info->si_uid : 0;
	ssize_t r = write(agt_pipe[1], &ev, sizeof ev);
	(void)r;
	errno = saved;
}

static int agt_open_pipe(void) {
	if (agt_pipe[0] >= 0) return 0;
	if (pipe(agt_pipe) != 0) return -1;
	fcntl(agt_pipe[0], F_SETFD, FD_CLOEXEC);
	fcntl(agt_pipe[1], F_SETFD, FD_CLOEXEC);
	// Never block inside the handler; a full pipe just drops the event.
	fcntl(agt_pipe[1], F_SETFL, fcntl(agt_pipe[1], F_GETFL) | O_NONBLOCK);
	return 0;
}

static int agt_install(int sig) {
	struct sigaction sa;
	memset(&sa, 0, sizeof sa);
	sa.sa_sigaction = agt_handler;
	// SA_ONSTACK: Go threads run signal handlers on their alternate stack.
	sa.sa_flags = SA_SIGINFO | SA_ONSTACK | SA_RESTART;
	sigemptyset(&sa.sa_mask);
	return sigaction(sig, &sa, NULL);
}

static int agt_read_fd(void) { return agt_pipe[0]; }
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
)

var (
	mu     sync.Mutex
	out    chan Event
	reader sync.Once
)

// Notify installs a handler for sigs that reports each delivery, with its
// sender, on the returned channel. Callers must NOT also pass these signals
// to os/signal.Notify: that would reinstall Go's handler over this one.
func Notify(sigs ...syscall.Signal) (<-chan Event, bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if C.agt_open_pipe() != 0 {
		return nil, false, fmt.Errorf("sigsender: pipe failed")
	}
	if out == nil {
		out = make(chan Event, 8)
	}
	for _, s := range sigs {
		if rc, err := C.agt_install(C.int(s)); rc != 0 {
			return nil, false, fmt.Errorf("sigsender: sigaction(%d): %v", s, err)
		}
	}
	reader.Do(func() {
		f := os.NewFile(uintptr(C.agt_read_fd()), "sigsender")
		go readLoop(f, out)
	})
	return out, true, nil
}

func readLoop(r io.Reader, ch chan<- Event) {
	buf := make([]byte, 12) // three C ints
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		ev := Event{
			Signal: syscall.Signal(int32(binary.NativeEndian.Uint32(buf[0:4]))),
			PID:    int(int32(binary.NativeEndian.Uint32(buf[4:8]))),
			UID:    int(int32(binary.NativeEndian.Uint32(buf[8:12]))),
		}
		select {
		case ch <- ev:
		default: // nobody listening fast enough; drop rather than block
		}
	}
}
