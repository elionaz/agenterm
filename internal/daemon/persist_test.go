package daemon_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/daemon"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/sessionstore"
)

// restartDaemon starts a fresh daemon on p (after the previous one stopped).
func restartDaemon(t *testing.T, p paths.Paths) (*client.Client, func()) {
	t.Helper()
	return startDaemonIn(t, p, func(daemon.Notification) {})
}

func setRestore(t *testing.T, p paths.Paths, policy string) {
	t.Helper()
	if err := os.WriteFile(p.Settings(), []byte("sessions:\n  restore: "+policy+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// spawnWith starts a session of profileID and waits for want in its output.
func spawnWith(t *testing.T, c *client.Client, projectID, profileID, input, want string) protocol.SessionInfo {
	t.Helper()
	info, err := c.Spawn(ctxT(t), protocol.SpawnParams{ProjectID: projectID, ProfileID: profileID, Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	st, snap := attachSnapshot(t, c, info.ID)
	if want == "" || (input == "" && bytes.Contains(snap, []byte(want))) {
		st.Close()
		return info
	}
	if input != "" {
		if err := st.Input([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
	readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte(want)) })
	st.Close()
	return info
}

// waitScrollback waits for the persisted scrollback of id to contain want.
func waitScrollback(t *testing.T, p paths.Paths, id, want string) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for {
		b, _ := sessionstore.LoadScrollback(p.ScrollbackDir(), id, 0)
		if bytes.Contains(b, []byte(want)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scrollback of %s: %q, want %q", id, b, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSessionsSurviveRestartAsk(t *testing.T) {
	c, p, stop := startDaemon(t)
	proj, err := c.ProjectAdd(ctxT(t), t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	keep := spawnWith(t, c, proj.ID, "shell", "echo KEEP-$((40+2))\n", "KEEP-42")
	drop := spawnWith(t, c, proj.ID, "shell", "echo DROP-$((1+1))\n", "DROP-2")
	gone := spawnWith(t, c, proj.ID, "shell", "", "")
	// A session the user killed does not come back.
	if _, err := c.Kill(ctxT(t), gone.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, gone.ID, "exited")
	// Scrollback is saved while the session runs (debounced), not only at exit.
	waitScrollback(t, p, keep.ID, "KEEP-42")

	// stop --force: live sessions are killed but stay restorable.
	if err := c.Shutdown(ctxT(t), true); err != nil {
		t.Fatal(err)
	}
	stop()
	for _, f := range []string{p.Sessions(), sessionstore.ScrollbackFile(p.ScrollbackDir(), keep.ID)} {
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want 0600", f, fi.Mode().Perm())
		}
	}

	c, stop = restartDaemon(t, p)
	defer stop()
	list, err := c.Sessions(ctxT(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("sessions after restart: %+v", list)
	}
	for _, s := range list {
		if s.State != "suspended" || s.SuspendedAt == nil || s.ResumeMode != protocol.ResumeFresh || s.Pid != 0 {
			t.Fatalf("restored session: %+v", s)
		}
	}
	// Suspended sessions do not count as live for `stop`.
	if err := c.Shutdown(ctxT(t), false); err != nil {
		t.Fatalf("plain stop with only suspended sessions: %v", err)
	}
	stop()
	// They stay suspended across another restart.
	c, stop = restartDaemon(t, p)
	defer stop()
	if list := mustSessions(t, c); len(list) != 2 || list[0].State != "suspended" {
		t.Fatalf("second restart: %+v", list)
	}

	// Attaching replays the saved scrollback plus a "suspended" line, and
	// the stream stays open (no exit frame).
	st, snap := attachSnapshot(t, c, keep.ID)
	if !bytes.Contains(snap, []byte("KEEP-42")) || !bytes.Contains(snap, []byte("suspended since")) {
		t.Fatalf("suspended replay: %q", snap)
	}
	if err := st.Input([]byte("ignored\n")); err != nil {
		t.Fatal(err)
	}

	// Resume: the shell is respawned in the same folder, after the old
	// scrollback and a "session restored" line; the open stream is told to
	// reattach.
	res, err := c.Resume(ctxT(t), keep.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sessions) != 1 || res.Sessions[0].State == "suspended" || res.Sessions[0].Pid == 0 {
		t.Fatalf("resume: %+v", res)
	}
	readUntil(t, st, func(_ []byte, f protocol.Frame) bool {
		return f.Type == protocol.FrameError && strings.Contains(string(f.Payload), "reattach")
	})
	st2, snap2 := attachSnapshot(t, c, keep.ID)
	if !bytes.Contains(snap2, []byte("KEEP-42")) || !bytes.Contains(snap2, []byte("session restored")) {
		t.Fatalf("resumed replay: %q", snap2)
	}
	st2.Input([]byte("pwd; echo NEW-$((2+3))\n"))
	out := readUntil(t, st2, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte("NEW-5")) })
	if !bytes.Contains(out, []byte(filepath.Base(keep.Cwd))) {
		t.Fatalf("resumed shell not in %s: %q", keep.Cwd, out)
	}
	if again, err := c.Resume(ctxT(t), keep.ID, false); err == nil || !hasCode(err, "resume_failed") {
		t.Fatalf("second resume: %+v %v", again, err)
	}

	// Discard the other one: gone from the list and from disk.
	if _, err := c.Discard(ctxT(t), drop.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, s := range mustSessions(t, c) {
		if s.ID == drop.ID {
			t.Fatal("discarded session still listed")
		}
	}
	if _, err := os.Stat(sessionstore.ScrollbackFile(p.ScrollbackDir(), drop.ID)); !os.IsNotExist(err) {
		t.Fatalf("discarded scrollback: %v", err)
	}
	recs := readRecords(t, p)
	if len(recs) != 1 || recs[0].ID != keep.ID {
		t.Fatalf("records: %+v", recs)
	}
}

func mustSessions(t *testing.T, c *client.Client) []protocol.SessionInfo {
	t.Helper()
	list, err := c.Sessions(ctxT(t))
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func readRecords(t *testing.T, p paths.Paths) []sessionstore.Record {
	t.Helper()
	b, err := os.ReadFile(p.Sessions())
	if err != nil {
		t.Fatal(err)
	}
	var recs []sessionstore.Record
	if err := json.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	return recs
}

func TestRestorePolicies(t *testing.T) {
	for _, policy := range []string{"always", "never"} {
		t.Run(policy, func(t *testing.T) {
			c, p, stop := startDaemon(t)
			proj, err := c.ProjectAdd(ctxT(t), t.TempDir(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			s := spawnWith(t, c, proj.ID, "shell", "echo OLD-$((3+4))\n", "OLD-7")
			stop() // like SIGTERM
			setRestore(t, p, policy)

			c, stop = restartDaemon(t, p)
			defer stop()
			if policy == "never" {
				if list := mustSessions(t, c); len(list) != 0 {
					t.Fatalf("never: %+v", list)
				}
				if recs := readRecords(t, p); len(recs) != 0 {
					t.Fatalf("never: records %+v", recs)
				}
				if _, err := os.Stat(sessionstore.ScrollbackFile(p.ScrollbackDir(), s.ID)); !os.IsNotExist(err) {
					t.Fatalf("never: scrollback kept: %v", err)
				}
				return
			}
			info := waitState(t, c, s.ID, "idle") // resumed: a live shell again
			if info.Pid == 0 || info.Pid == s.Pid {
				t.Fatalf("always: %+v", info)
			}
			_, snap := attachSnapshot(t, c, s.ID)
			if !bytes.Contains(snap, []byte("OLD-7")) || !bytes.Contains(snap, []byte("session restored")) {
				t.Fatalf("always replay: %q", snap)
			}
		})
	}
}

// A claude session resumes its own conversation: the session id reported
// by its hooks ends up as --resume, after the --settings injection.
func TestClaudeResumeUsesHookSessionID(t *testing.T) {
	c, p, stop := startDaemon(t)
	script := filepath.Join(p.ConfigDir, "fakeclaude")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"ARGV: $*\"\nexec cat\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeProfiles(t, p.Profiles(), "- id: fakeclaude\n  cmd: "+script+"\n  args: [\"--model\", \"x\"]\n  setup_hooks: claude\n")
	if _, err := c.RescanProfiles(ctxT(t)); err != nil {
		t.Fatal(err)
	}
	proj, err := c.ProjectAdd(ctxT(t), t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	s := spawnWith(t, c, proj.ID, "fakeclaude", "", "ARGV: --settings")
	// What `notify --from-hook claude` sends for a SessionStart payload.
	if _, err := c.NotifyEvent(ctxT(t), protocol.NotifyParams{SessionID: s.ID, Agent: "claude",
		AgentSessionID: "0b5e7f2a-1111-2222-3333-444455556666", TranscriptPath: "/x/t.jsonl"}); err != nil {
		t.Fatal(err)
	}
	if r := readRecords(t, p); len(r) != 1 || r[0].Resume == nil || r[0].Resume.SessionID != "0b5e7f2a-1111-2222-3333-444455556666" {
		t.Fatalf("records: %+v", r)
	}
	stop()

	c, stop = restartDaemon(t, p)
	defer stop()
	if info := findSession(t, c, s.ID); info.State != "suspended" || info.ResumeMode != protocol.ResumeAgent {
		t.Fatalf("restored: %+v", info)
	}
	if _, err := c.Resume(ctxT(t), s.ID, false); err != nil {
		t.Fatal(err)
	}
	st, snap := attachSnapshot(t, c, s.ID)
	want := "ARGV: --settings " + filepath.Join(p.HooksDir(), "claude-settings.json") + " --resume 0b5e7f2a-1111-2222-3333-444455556666 --model x"
	if !bytes.Contains(snap, []byte(want)) {
		readUntil(t, st, func(out []byte, _ protocol.Frame) bool { return bytes.Contains(out, []byte(want)) })
	}
}

// Resume failures keep the session suspended with a coded error.
func TestResumeFailuresKeepSuspended(t *testing.T) {
	c, p, stop := startDaemon(t)
	dir := filepath.Join(t.TempDir(), "proj")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	proj, err := c.ProjectAdd(ctxT(t), dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	s := spawnWith(t, c, proj.ID, "shell", "", "")
	stop()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	c, stop = restartDaemon(t, p)
	defer stop()
	if _, err := c.Resume(ctxT(t), s.ID, false); !hasCode(err, "dir_missing") {
		t.Fatalf("resume in a deleted folder: %v", err)
	}
	info := findSession(t, c, s.ID)
	if info.State != "suspended" || info.ResumeErrorCode != "dir_missing" {
		t.Fatalf("after failed resume: %+v", info)
	}
	res, err := c.Resume(ctxT(t), "", true)
	if err != nil || len(res.Failed) != 1 || res.Failed[0].ErrorCode != "dir_missing" {
		t.Fatalf("resume --all: %+v %v", res, err)
	}
	// kill on a suspended session discards it.
	if _, err := c.Kill(ctxT(t), s.ID); err != nil {
		t.Fatal(err)
	}
	if list := mustSessions(t, c); len(list) != 0 {
		t.Fatalf("after kill: %+v", list)
	}
}

// A worktree session resumes in its worktree; once the worktree is gone,
// resuming fails and the session stays suspended.
func TestWorktreeSessionResumesInItsWorktree(t *testing.T) {
	c, p, stop := startDaemon(t)
	repo := tempRepo(t)
	proj, err := c.ProjectAdd(ctxT(t), repo, "repo")
	if err != nil {
		t.Fatal(err)
	}
	mk := func() protocol.SessionInfo {
		info, err := c.Spawn(ctxT(t), protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell", Mode: protocol.ModeWorktree})
		if err != nil {
			t.Fatal(err)
		}
		return info
	}
	a, b := mk(), mk()
	stop()
	if err := os.RemoveAll(b.WorktreePath); err != nil {
		t.Fatal(err)
	}

	c, stop = restartDaemon(t, p)
	defer stop()
	if s := findSession(t, c, a.ID); s.State != "suspended" || s.WorktreeID != a.WorktreeID {
		t.Fatalf("restored: %+v", s)
	}
	if _, err := c.Resume(ctxT(t), a.ID, false); err != nil {
		t.Fatal(err)
	}
	runIn(t, c, a.ID, "pwd > where.txt")
	if out, err := os.ReadFile(filepath.Join(a.WorktreePath, "where.txt")); err != nil || !strings.Contains(string(out), filepath.Base(a.WorktreePath)) {
		t.Fatalf("resumed outside its worktree: %q %v", out, err)
	}
	if _, err := c.Resume(ctxT(t), b.ID, false); !hasCode(err, "dir_missing") {
		t.Fatalf("resume without worktree: %v", err)
	}
	if s := findSession(t, c, b.ID); s.State != "suspended" {
		t.Fatalf("after failure: %+v", s)
	}
}

func TestCorruptSessionsFileStartsEmpty(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "agt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	p := paths.In(dir)
	writeProfiles(t, p.Profiles(), "")
	if err := os.WriteFile(p.Sessions(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, stop := restartDaemon(t, p)
	defer stop()
	if list := mustSessions(t, c); len(list) != 0 {
		t.Fatalf("sessions: %+v", list)
	}
	m, _ := filepath.Glob(p.Sessions() + ".corrupt-*")
	if len(m) != 1 {
		t.Fatalf("backup: %v", m)
	}
}
