// Package transport abstracts the local IPC channel between the daemon and
// its clients. Today only unix domain sockets are implemented; a Windows
// named-pipe implementation can satisfy the same interface later.
package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Transport listens on and dials a local address.
type Transport interface {
	// Listen binds addr. Implementations must restrict access to the current user.
	Listen(addr string) (net.Listener, error)
	// Dial connects to addr.
	Dial(ctx context.Context, addr string) (net.Conn, error)
	// Cleanup removes any leftover artifact for addr (e.g. a stale socket file).
	Cleanup(addr string) error
}

// Default returns the transport for the current platform.
func Default() Transport { return Unix{} }

// Unix is a Transport over unix domain sockets.
type Unix struct{}

// Listen creates the socket with 0600 permissions.
func (Unix) Listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(addr, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}

// Dial connects to the socket at addr.
func (Unix) Dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}

// Cleanup removes the socket file if present.
func (Unix) Cleanup(addr string) error {
	if err := os.Remove(addr); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Alive reports whether something is accepting connections on addr.
func Alive(t Transport, addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	c, err := t.Dial(ctx, addr)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
