package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elionaz/agenterm/internal/atomicfile"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/status"
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

// daemonMismatch: the daemon is older than the app (an update installed a
// new app while the old daemon kept running). Development builds ("dev")
// never ask for a restart; commits are not compared (a rebuild of the same
// version is compatible) and a newer daemon is left alone (restarting it
// from this executable would downgrade it).
func daemonMismatch(app, daemon string) bool {
	if app == "dev" || daemon == app {
		return false
	}
	if daemon == "" { // daemons before 0.2.0 do not report a version
		return true
	}
	c, ok := compareVersions(daemon, app)
	return !ok || c < 0
}

// compareVersions compares two "X.Y.Z[-pre]" versions (a pre-release sorts
// before its release); ok is false when either does not parse.
func compareVersions(a, b string) (c int, ok bool) {
	pa, prea, oka := parseVersion(a)
	pb, preb, okb := parseVersion(b)
	if !oka || !okb {
		return 0, false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return cmp.Compare(pa[i], pb[i]), true
		}
	}
	switch {
	case prea == preb:
		return 0, true
	case prea == "":
		return 1, true
	case preb == "":
		return -1, true
	}
	return comparePre(prea, preb), true
}

// comparePre orders pre-release suffixes as semver does: dot-separated
// identifiers, numeric ones numerically (rc.9 < rc.10) and before
// alphanumeric ones, a shorter list first when it is a prefix.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, errx := strconv.Atoi(as[i])
		y, erry := strconv.Atoi(bs[i])
		switch {
		case errx == nil && erry == nil:
			if x != y {
				return cmp.Compare(x, y)
			}
		case errx == nil:
			return -1
		case erry == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return cmp.Compare(len(as), len(bs))
}

func parseVersion(v string) (n [3]int, pre string, ok bool) {
	v = strings.TrimPrefix(v, "v")
	v, pre, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return n, "", false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return n, "", false
		}
		n[i] = x
	}
	return n, pre, true
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

// RestartDaemon replaces the running daemon with one from this executable.
// The sessions that are live now are recorded in paths.RestartFile, so the
// new daemon restores them as suspended whatever sessions.restore says, and
// then exactly those are resumed (sessions the user had left suspended
// stay so).
func (a *App) RestartDaemon() (protocol.SessionBatch, error) {
	out := protocol.SessionBatch{Sessions: []protocol.SessionInfo{}}
	ctx, cancel := context.WithTimeout(a.ctx, gitTO)
	defer cancel()
	var live []string
	if ss, err := a.client.Sessions(ctx); err == nil {
		for _, s := range ss {
			if s.State != string(status.Exited) && s.State != string(status.Suspended) {
				live = append(live, s.ID)
			}
		}
	}
	marker := a.paths.RestartFile()
	if len(live) > 0 {
		b, _ := json.Marshal(protocol.RestartMarker{Resume: live})
		if err := atomicfile.Replace(marker, b, 0o600); err != nil {
			return out, fmt.Errorf("restart marker: %w", err)
		}
	}
	if err := a.replaceDaemon(ctx); err != nil {
		if a.client.Ping(ctx) == nil { // the old daemon is still there
			_ = os.Remove(marker)
		}
		return out, err
	}
	for _, id := range live {
		b, err := a.client.Resume(ctx, id, false)
		if err != nil {
			out.Failed = append(out.Failed, protocol.BatchError{SessionID: id, Error: err.Error()})
			continue
		}
		out.Sessions = append(out.Sessions, b.Sessions...)
		out.Failed = append(out.Failed, b.Failed...)
	}
	return out, nil
}

// restartWaitTO bounds the wait for the old daemon to exit (it gives each
// agent KillGrace to stop; startMu is held meanwhile).
const restartWaitTO = 20 * time.Second

// replaceDaemon shuts the daemon down (force), waits for its process to
// finish and starts a new one. Waiting for the process, not for the socket
// to stop answering, matters: a daemon stops answering long before it is
// done (it still kills its agents, and daemons before 0.2.0 remove the
// socket path as their very last step, which would orphan a new daemon
// already listening there). startMu keeps the event loop, which notices the
// old daemon going away, from starting one of its own meanwhile.
func (a *App) replaceDaemon(ctx context.Context) error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	pid := readPid(a.paths.PidFile())
	if err := a.client.Shutdown(ctx, true); err != nil && a.client.Ping(ctx) == nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, restartWaitTO)
	defer cancel()
	gone := func() bool {
		if pid > 0 {
			// The daemon removes its pid file last thing; a crashed one
			// leaves it, but its process is gone (or the pid was reused).
			return readPid(a.paths.PidFile()) != pid || !processAlive(pid) || !isDaemonProc(pid)
		}
		return a.client.Ping(wctx) != nil
	}
	for !gone() {
		select {
		case <-wctx.Done():
			// Stuck after suspending its sessions: kill it rather than
			// start a new daemon it could still orphan (startMu is
			// released on return and the event loop would start one).
			if !killStuckDaemon(pid) {
				return errors.New("daemon still running after shutdown request")
			}
			// Its pid file stays behind; the new daemon overwrites it.
			if err := a.ensureDaemonLocked(ctx); err != nil {
				return fmt.Errorf("start daemon: %w", err)
			}
			return nil
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := a.ensureDaemonLocked(ctx); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}
	return nil
}

func readPid(file string) int {
	b, err := os.ReadFile(file)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

func processAlive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// isDaemonProc: pid is (still) an agenterm process. An in-process daemon
// (tests) shares our pid; an unknown name counts as ours (keep waiting).
func isDaemonProc(pid int) bool {
	if pid == os.Getpid() {
		return true
	}
	n := procName(pid)
	return n == "" || strings.HasPrefix(n, "agenterm")
}

// killStuckDaemon SIGKILLs a daemon that did not exit in time and waits
// briefly for it to go; false if it cannot (not ours, or still there).
func killStuckDaemon(pid int) bool {
	if pid <= 0 || pid == os.Getpid() || !strings.HasPrefix(procName(pid), "agenterm") {
		return false
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	for range 40 {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
