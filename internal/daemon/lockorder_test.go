package daemon_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/protocol"
)

// Calls that ask whether sessions are alive (worktree.list, worktree.remove
// of a worktree in use, a shutdown refused for live sessions) must not take
// a session's status lock while holding the daemon's: its state changes
// take them the other way round (#57 was the same bug in the message
// calls). Before the fix this deadlocked within a few thousand iterations
// (-cpu 4).
func TestLivenessCallsDuringStateChanges(t *testing.T) {
	c, _, stop := startDaemon(t)
	defer stop()
	// Thousands of calls: more than ctxT's 10 s under -race.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	proj, err := c.ProjectAdd(ctx, tempRepo(t), "race")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	// The git-backed calls are slow (git runs for each): fewer of them.
	run := func(name string, n int, f func(i int) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				if err := f(i); err != nil {
					t.Errorf("%s: %v", name, err)
					return
				}
			}
		}()
	}
	states := []string{"running", "idle", "waiting_input"}
	run("notify", 3000, func(i int) error { _, err := c.Notify(ctx, sess.ID, states[i%len(states)]); return err })
	run("worktree.list", 40, func(int) error { _, err := c.Worktrees(ctx, proj.ID); return err })
	run("worktree.remove", 40, func(int) error {
		_, err := c.WorktreeRemove(ctx, protocol.WorktreeRemoveParams{ID: sess.WorktreeID})
		if err != nil && hasCode(err, "worktree_live") {
			return nil // refused: the session is alive, as expected
		}
		return err
	})
	run("shutdown", 3000, func(int) error {
		if err := c.Shutdown(ctx, false); err != nil && hasCode(err, "live_sessions") {
			return nil // refused for the live session, as expected
		}
		return err
	})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatalf("daemon deadlocked: liveness calls and state changes did not finish\n%s", agentermStacks())
	}
}
