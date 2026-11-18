// Package peercred identifies the process on the other end of a local
// (unix domain) socket connection, so the daemon can log who asked it to
// shut down.
package peercred

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrUnsupported is returned where the platform (or the connection type)
// offers no peer credentials.
var ErrUnsupported = errors.New("peer credentials not supported")

// ErrNoProcess: there is no such process.
var ErrNoProcess = errors.New("no such process")

// PeerPID returns the pid of the process that opened the other end of conn
// (a *net.UnixConn). The kernel records it at connect time.
func PeerPID(conn net.Conn) (int, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, ErrUnsupported
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pid int
	var perr error
	if err := raw.Control(func(fd uintptr) { pid, perr = peerPID(int(fd)) }); err != nil {
		return 0, err
	}
	return pid, perr
}

// Process describes a running process; zero values mean unknown.
type Process struct {
	PID     int
	PPID    int
	Command string // full command line (ps "command")
}

// Lookup returns pid's parent and command line via ps(1), bounded to one
// second. A process that already exited yields an error.
func Lookup(pid int) (Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return Process{PID: pid}, err
	}
	line := strings.TrimSpace(string(out))
	ppid, cmd, _ := strings.Cut(line, " ")
	p := Process{PID: pid, Command: strings.TrimSpace(cmd)}
	p.PPID, _ = strconv.Atoi(ppid)
	return p, nil
}
