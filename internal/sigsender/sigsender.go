// Package sigsender delivers termination signals together with the PID of the
// process that sent them (siginfo si_pid), which os/signal does not expose.
//
// It exists to diagnose unexplained daemon shutdowns: the daemon logs who sent
// the SIGTERM. On platforms without cgo support it reports ok=false and the
// caller falls back to os/signal.
package sigsender

import "syscall"

// Event is one received signal.
type Event struct {
	Signal syscall.Signal
	PID    int // sender PID; 0 if the kernel did not report one
	UID    int // sender real UID
}
