package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/settings"
	"github.com/elionaz/agenterm/internal/version"
)

// After an in-app update the window runs the new build while the daemon
// (which outlives the window, by design) is still the old one. The
// frontend asks DaemonStatus on every (re)connect and offers RestartDaemon.

// DaemonStatus compares the daemon's build with the app's.
type DaemonStatus struct {
	AppVersion string `json:"appVersion"`
	// DaemonVersion is "" for daemons older than 0.2.0 (they do not say).
	DaemonVersion string `json:"daemonVersion"`
	Mismatch      bool   `json:"mismatch"`
}

// daemonMismatch: development builds ("dev") never ask for a restart, and
// commits are not compared (a rebuild of the same version is compatible).
func daemonMismatch(app, daemon string) bool {
	return app != "dev" && daemon != app
}

func (a *App) DaemonStatus() (DaemonStatus, error) {
	var r protocol.PingResult
	err := a.call(func(ctx context.Context) (err error) {
		r, err = a.client.PingInfo(ctx)
		return err
	})
	if err != nil {
		return DaemonStatus{AppVersion: version.Version}, err
	}
	return DaemonStatus{AppVersion: version.Version, DaemonVersion: r.Version,
		Mismatch: daemonMismatch(version.Version, r.Version)}, nil
}

// RestartDaemon replaces the running daemon with one from this executable:
// it stops it with force (live sessions are recorded as suspended first),
// starts the new one and, when sessions.restore is "ask", resumes every
// suspended session (with "always" the new daemon does it by itself; with
// "never" there is nothing to resume).
func (a *App) RestartDaemon() (protocol.SessionBatch, error) {
	out := protocol.SessionBatch{Sessions: []protocol.SessionInfo{}}
	ctx, cancel := context.WithTimeout(a.ctx, gitTO)
	defer cancel()
	policy := settings.RestoreAsk
	if st, err := a.client.Settings(ctx); err == nil {
		policy = st.Sessions.Restore
	}
	if err := a.replaceDaemon(ctx); err != nil {
		return out, err
	}
	if policy != settings.RestoreAsk && policy != "" {
		return out, nil
	}
	b, err := a.client.Resume(ctx, "", true)
	b.Sessions = nonNil(b.Sessions)
	return b, err
}

// replaceDaemon shuts the daemon down (force), waits for its socket to go
// away and starts a new one. startMu keeps the event loop, which notices
// the old daemon going away, from starting one of its own meanwhile.
func (a *App) replaceDaemon(ctx context.Context) error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	if err := a.client.Shutdown(ctx, true); err != nil && a.client.Ping(ctx) == nil {
		return err
	}
	for a.client.Ping(ctx) == nil {
		select {
		case <-ctx.Done():
			return errors.New("daemon still running after shutdown request")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := a.ensureDaemonLocked(ctx); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	return nil
}
