package daemon_test

import (
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/status"
)

// The checks of which sessions are alive (the shutdown's, a worktree's
// before removing it, the worktree list's) must not take a session's
// status lock while holding the daemon's: a state change takes them the
// other way round (its callback publishes and asks for git info). #57 was
// the same bug in the message calls. They run here directly, without the
// RPCs (a refused shutdown runs ps, too slow for thousands of rounds);
// before the fix this deadlocked within a few thousand.
func TestLivenessChecksDuringStateChanges(t *testing.T) {
	srv, c, stop := startDaemonServer(t)
	defer stop()
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, tempRepo(t), "race")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		states := []status.State{status.Running, status.Idle, status.WaitingInput}
		for i := 0; i < 5000; i++ {
			if err := srv.NotifyState(sess.ID, states[i%len(states)]); err != nil {
				t.Errorf("notify: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			if err := srv.CheckLiveness(proj.ID, sess.WorktreeID); err != nil {
				t.Errorf("liveness: %v", err)
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("daemon deadlocked: liveness checks and state changes did not finish\n%s", agentermStacks())
	}
}
