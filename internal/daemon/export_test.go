package daemon

import (
	"time"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/status"
)

// SetAutoTiming shortens the automatic-delivery delays for a test; the
// returned func restores them.
func SetAutoTiming(delay, quiet, retry time.Duration) func() {
	d, q, r := autoDelay, autoQuiet, autoRetry
	autoDelay, autoQuiet, autoRetry = delay, quiet, retry
	return func() { autoDelay, autoQuiet, autoRetry = d, q, r }
}

// AutoChainMax is the automatic-delivery chain limit.
const AutoChainMax = autoChainMax

// CheckLiveness runs the daemon's live-session checks once (the shutdown
// one, a worktree's, and the worktree list's), for lock-order tests.
func (s *Server) CheckLiveness(projectID, worktreeID string) error {
	_ = s.liveSessions()
	_ = s.liveSessionIn(worktreeID)
	_, err := s.worktreeList(protocol.WorktreeListParams{ProjectID: projectID})
	return err
}

// NotifyState changes session id's state as a hook would, without the RPC.
func (s *Server) NotifyState(id string, st status.State) error {
	sess, err := s.lookup(id)
	if err != nil {
		return err
	}
	return sess.Notify(st)
}

// SetInstallCommand makes profile.install run cmd for every catalog agent
// (never a real install in tests); the returned func restores it.
func SetInstallCommand(cmd string) func() {
	prev := catalogInstall
	catalogInstall = func(id string) (string, string) {
		name, real := prev(id)
		if real == "" {
			return "", ""
		}
		return name, cmd
	}
	return func() { catalogInstall = prev }
}

// No test daemon finds the machine's browser unless it asks (SetBrowser).
func init() { browserFind = func(string, func(string) (string, error)) string { return "" } }

// SetBrowser makes the daemon find exe as the agents' browser; it returns
// the restore func.
func SetBrowser(exe string) func() {
	old := browserFind
	browserFind = func(string, func(string) (string, error)) string { return exe }
	return func() { browserFind = old }
}
