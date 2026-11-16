//go:build !((darwin || linux) && cgo)

package sigsender

import "syscall"

// Notify is unsupported here; callers fall back to os/signal.
func Notify(...syscall.Signal) (<-chan Event, bool, error) { return nil, false, nil }
