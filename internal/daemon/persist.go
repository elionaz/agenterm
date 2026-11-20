package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/i18n"
	"github.com/elionaz/agenterm/internal/peercred"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/sessionstore"
	"github.com/elionaz/agenterm/internal/settings"
)

// Persistence & resume.
//
// Every spawned session gets a record in sessions.json (sessionstore) and
// its scrollback is copied to scrollback/<id>.log while it produces output
// (at most every scrollbackEvery) and once more when the daemon stops.
// A session that exits, or that the user kills or discards, loses both. A
// daemon that stops (signal, `stop --force`, crash) leaves the records of
// its live sessions behind; the next one brings them back as suspended
// sessions — no process, just their scrollback — and then, following
// settings sessions.restore, resumes them (always), waits for the user
// (ask) or discards them (never).
//
// Resuming respawns the profile in the same folder (worktree included),
// seeded with the saved scrollback plus a "session restored" line: claude
// and codex continue the same conversation through the session id their
// hooks reported (`claude --resume <id>`, `codex resume <id>`); other
// profiles get their resume_args (opencode: --continue), if any.

// scrollbackEvery is how often a session's changed scrollback is saved.
const scrollbackEvery = 2 * time.Second

// suspInfo is a suspended session's record and last resume failure.
type suspInfo struct {
	rec         sessionstore.Record
	mode        string // protocol.Resume*
	errMsg      string
	errCode     string
	suspendedAt time.Time
}

// openStore loads sessions.json, moving a corrupt one aside.
func openStore(p string, log interface{ Warn(string, ...any) }) (*sessionstore.Store, error) {
	st, backup, err := sessionstore.Open(p)
	if err != nil {
		return nil, err
	}
	if backup != "" {
		log.Warn("sessions.json is corrupt: moved aside, starting without sessions to restore", "backup", backup)
	}
	return st, nil
}

// restoreSessions turns the records left by the previous daemon into
// suspended sessions and applies sessions.restore. It runs once, before
// any client is served.
func (s *Server) restoreSessions() {
	st, err := settings.Load(s.cfg.Paths.Settings())
	if err != nil {
		s.log.Warn("settings", "err", err)
	}
	policy := st.Sessions.Restore
	// Sessions the app's "Restart now" suspended are kept whatever the
	// policy says and are not resumed here: the app resumes them itself.
	restart := s.takeRestartMarker()
	recs := s.store.List()
	keep := map[string]bool{}
	var restored, auto []string
	for _, rec := range recs {
		if policy == settings.RestoreNever && !restart[rec.ID] {
			continue
		}
		sess, info := s.restoreOne(rec)
		s.mu.Lock()
		if s.sessions[rec.ID] != nil { // cannot happen: ids are unique
			s.mu.Unlock()
			continue
		}
		s.sessions[rec.ID] = sess
		s.order = append(s.order, rec.ID)
		s.susp[rec.ID] = info
		s.git[rec.ID] = info.git()
		s.mu.Unlock()
		s.setName(rec.ID, normName(rec.Name))
		if m := normModel(rec.Model); m != "" {
			s.namesMu.Lock()
			s.models[rec.ID] = m
			s.namesMu.Unlock()
		}
		if rec.ContinuedFrom != "" {
			s.namesMu.Lock()
			s.contFrom[rec.ID] = rec.ContinuedFrom
			s.namesMu.Unlock()
		}
		if rec.Port > 0 && rec.Port <= 65535 {
			s.setPort(rec.ID, sessPort{Port: rec.Port, Env: rec.PortEnv})
		}
		keep[rec.ID] = true
		restored = append(restored, rec.ID)
		if !restart[rec.ID] {
			auto = append(auto, rec.ID)
		}
	}
	var drop []string
	for _, rec := range recs {
		if !keep[rec.ID] {
			drop = append(drop, rec.ID)
		}
	}
	if len(drop) > 0 {
		if err := s.store.Delete(drop...); err != nil {
			s.log.Warn("sessions.json", "err", err)
		}
		s.log.Info("sessions from the previous run discarded", "count", len(drop), "policy", policy)
	}
	sessionstore.PruneScrollback(s.cfg.Paths.ScrollbackDir(), keep)
	if len(restored) == 0 {
		return
	}
	s.log.Info("sessions from the previous run restored as suspended", "count", len(restored), "policy", policy)
	if policy == settings.RestoreAlways && len(auto) > 0 {
		go func() {
			for _, id := range auto {
				if _, err := s.resume(id); err != nil {
					s.log.Warn("automatic resume failed", "id", id, "err", err)
				}
			}
		}()
	}
}

// takeRestartMarker reads and removes paths.RestartFile (see
// RestartMarker). A missing or unreadable marker means no restart.
func (s *Server) takeRestartMarker() map[string]bool {
	f := s.cfg.Paths.RestartFile()
	b, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	_ = os.Remove(f)
	var m protocol.RestartMarker
	if err := json.Unmarshal(b, &m); err != nil {
		s.log.Warn("restart marker", "err", err)
		return nil
	}
	out := make(map[string]bool, len(m.Resume))
	for _, id := range m.Resume {
		out[id] = true
	}
	s.log.Info("daemon restarted by the app", "sessions", len(out))
	return out
}

func (s *Server) restoreOne(rec sessionstore.Record) (*session.Session, *suspInfo) {
	dir := s.cfg.Paths.ScrollbackDir()
	ringSize := s.cfg.RingSize
	if ringSize <= 0 {
		ringSize = session.DefaultRingSize
	}
	sb, err := sessionstore.LoadScrollback(dir, rec.ID, ringSize)
	if err != nil {
		s.log.Warn("scrollback", "id", rec.ID, "err", err)
	}
	at := rec.SuspendedAt
	if at.IsZero() { // the daemon died without saying: last saved output
		if fi, err := os.Stat(sessionstore.ScrollbackFile(dir, rec.ID)); err == nil {
			at = fi.ModTime().UTC()
		} else {
			at = time.Now().UTC()
		}
	}
	proj, _ := s.projects.Get(rec.ProjectID)
	sess := session.Restore(session.Options{
		ID: rec.ID, ProjectID: rec.ProjectID, ProjectDir: proj.Path, ProjectName: proj.Name, ProfileID: rec.ProfileID,
		Cmd: rec.Cmd, Args: rec.Args, Cwd: rec.Cwd, Cols: rec.Cols, Rows: rec.Rows, RingSize: s.cfg.RingSize,
		Scrollback: sb, CreatedAt: rec.CreatedAt,
	})
	info := &suspInfo{rec: rec, suspendedAt: at, mode: protocol.ResumeFresh}
	// Without detection (no login shell PATH lookups at startup): only
	// the profile's resume settings matter here.
	if ps, err := profile.Load(s.cfg.Paths.Profiles(), nil); err == nil {
		if prof, ok := profile.Find(ps, rec.ProfileID); ok {
			_, info.mode = restartArgs(prof, nil, rec.Resume)
		}
	}
	if rec.SuspendedAt.IsZero() || rec.State == "" {
		_ = s.store.Update(rec.ID, func(r *sessionstore.Record) bool {
			r.SuspendedAt = at
			return true
		})
	}
	return sess, info
}

// git is the suspended session's git context, as it was.
func (i *suspInfo) git() sessGit {
	if i.rec.WorktreeID != "" {
		return sessGit{Mode: protocol.ModeWorktree, WorktreeID: i.rec.WorktreeID, Dir: i.rec.Cwd}
	}
	return sessGit{Mode: protocol.ModeSame, Dir: i.rec.Cwd}
}

// recordSession writes a freshly spawned (or resumed) session's record.
func (s *Server) recordSession(sess *session.Session, g sessGit, resume *sessionstore.Resume) {
	cols, rows := sess.Size()
	rec := sessionstore.Record{
		ID: sess.ID, ProjectID: sess.ProjectID, ProfileID: sess.ProfileID, Name: s.sessionName(sess.ID), Model: s.sessionModel(sess.ID),
		Port: s.sessionPort(sess.ID).Port, PortEnv: s.sessionPort(sess.ID).Env,
		Mode: g.Mode, WorktreeID: g.WorktreeID,
		Cwd: sess.Cwd, Cmd: sess.Cmd, Args: sess.Args, Pid: sess.Pid, Cols: cols, Rows: rows,
		CreatedAt: sess.CreatedAt, State: string(sess.State()), Resume: resume,
	}
	if err := s.store.Put(rec); err != nil {
		s.log.Warn("sessions.json", "id", sess.ID, "err", err)
	}
	s.persistMu.Lock()
	s.saved[sess.ID] = -1 // save the (seeded) scrollback on the next tick
	s.persistMu.Unlock()
}

// forgetRecord drops an exited session's record and scrollback, unless
// the daemon is stopping (then its sessions are suspended, not gone).
func (s *Server) forgetRecord(sess *session.Session) {
	if s.stopping.Load() {
		return
	}
	s.mu.Lock()
	current := s.sessions[sess.ID] == sess
	s.mu.Unlock()
	if !current {
		return
	}
	s.dropPersisted(sess.ID)
}

func (s *Server) dropPersisted(id string) {
	s.persistMu.Lock()
	delete(s.saved, id)
	if err := s.store.Delete(id); err != nil {
		s.log.Warn("sessions.json", "id", id, "err", err)
	}
	if err := sessionstore.RemoveScrollback(s.cfg.Paths.ScrollbackDir(), id); err != nil {
		s.log.Warn("scrollback", "id", id, "err", err)
	}
	s.persistMu.Unlock()
}

// learnResume records the agent's own session id (from its hooks).
func (s *Server) learnResume(sess *session.Session, agent, id, transcript string) {
	if !sess.Alive() || id == "" {
		return
	}
	err := s.store.Update(sess.ID, func(r *sessionstore.Record) bool {
		if r.Resume != nil && r.Resume.Agent == agent && r.Resume.SessionID == id && r.Resume.TranscriptPath == transcript {
			return false
		}
		r.Resume = &sessionstore.Resume{Agent: agent, SessionID: id, TranscriptPath: transcript, At: time.Now().UTC()}
		return true
	})
	if err != nil {
		s.log.Warn("sessions.json", "id", sess.ID, "err", err)
		return
	}
	s.log.Debug("agent session learned", "id", sess.ID, "agent", agent, "agentSession", id)
}

// persistLoop saves changed scrollbacks every scrollbackEvery until stop
// is closed.
func (s *Server) persistLoop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	t := time.NewTicker(scrollbackEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.saveScrollbacks()
		}
	}
}

// saveScrollbacks writes the scrollback of every live session whose
// output changed since it was last saved.
func (s *Server) saveScrollbacks() {
	s.mu.Lock()
	list := make([]*session.Session, 0, len(s.sessions))
	for _, id := range s.order {
		if sess := s.sessions[id]; sess != nil && !sess.Suspended() {
			list = append(list, sess)
		}
	}
	s.mu.Unlock()
	dir := s.cfg.Paths.ScrollbackDir()
	for _, sess := range list {
		s.persistMu.Lock()
		last, known := s.saved[sess.ID]
		total := sess.OutputTotal()
		if known && last != total {
			if err := sessionstore.SaveScrollback(dir, sess.ID, sess.Scrollback(), 0); err != nil {
				s.log.Warn("scrollback", "id", sess.ID, "err", err)
			} else {
				s.saved[sess.ID] = total
			}
		}
		s.persistMu.Unlock()
	}
}

// suspendAll runs when the daemon stops, before it kills the sessions: it
// saves every live session's scrollback and marks its record suspended,
// so the next daemon can restore it.
func (s *Server) suspendAll() {
	s.stopping.Store(true)
	if s.persistStop != nil {
		close(s.persistStop)
		<-s.persistDone
	}
	s.saveScrollbacks()
	now := time.Now().UTC()
	for _, sess := range s.liveSessions() {
		cols, rows := sess.Size()
		state := string(sess.State())
		if err := s.store.Update(sess.ID, func(r *sessionstore.Record) bool {
			r.State, r.SuspendedAt, r.Pid, r.Cols, r.Rows = state, now, sess.Pid, cols, rows
			return true
		}); err != nil {
			s.log.Warn("sessions.json", "id", sess.ID, "err", err)
		}
	}
}

// batch applies fn to one suspended session or (All) to every one.
func (s *Server) batch(p protocol.SuspendedRef, fn func(id string) (protocol.SessionInfo, error)) (protocol.SessionBatch, error) {
	out := protocol.SessionBatch{Sessions: []protocol.SessionInfo{}}
	if !p.All {
		if p.SessionID == "" {
			return out, errors.New("missing sessionId (or all)")
		}
		info, err := fn(p.SessionID)
		if err != nil {
			return out, err
		}
		out.Sessions = append(out.Sessions, info)
		return out, nil
	}
	for _, id := range s.suspendedIDs() {
		info, err := fn(id)
		if err != nil {
			be := protocol.BatchError{SessionID: id, Error: err.Error()}
			if code, data, ok := apierr.Code(err); ok {
				be.ErrorCode = code
				if data != nil {
					be.ErrorData, _ = json.Marshal(data)
				}
			}
			out.Failed = append(out.Failed, be)
			continue
		}
		out.Sessions = append(out.Sessions, info)
	}
	return out, nil
}

func (s *Server) suspendedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for _, id := range s.order {
		if s.susp[id] != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func resumeErr(id, reason string, extra map[string]any, format string, args ...any) error {
	data := map[string]any{"sessionId": id, "reason": reason}
	for k, v := range extra {
		data[k] = v
	}
	return apierr.New(apierr.ResumeFailed, data, format, args...)
}

// resume respawns suspended session id in place. On failure the session
// stays suspended and remembers the error (SessionInfo.ResumeError).
func (s *Server) resume(id string) (protocol.SessionInfo, error) {
	s.mu.Lock()
	old, info := s.sessions[id], s.susp[id]
	switch {
	case old == nil:
		s.mu.Unlock()
		return protocol.SessionInfo{}, fmt.Errorf("session not found: %s", id)
	case info == nil:
		s.mu.Unlock()
		return protocol.SessionInfo{}, resumeErr(id, "not_suspended", nil, "session %s is not suspended", id)
	case s.resuming[id]:
		s.mu.Unlock()
		return protocol.SessionInfo{}, resumeErr(id, "busy", nil, "session %s is already being resumed", id)
	}
	s.resuming[id] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.resuming, id)
		s.mu.Unlock()
	}()

	res, err := s.respawn(old, info.rec)
	if err != nil {
		code, _, _ := apierr.Code(err)
		s.mu.Lock()
		if cur := s.susp[id]; cur != nil {
			cur.errMsg, cur.errCode = err.Error(), code
		}
		s.mu.Unlock()
		s.log.Warn("resume failed", "id", id, "err", err)
		s.bus.publish(protocol.Event{Type: protocol.EventSessionAttention, SessionID: id, State: string(old.State()), At: time.Now().UTC(),
			ProjectName: s.projectName(old.ProjectID, old.ProjectName), ProfileID: old.ProfileID, SessionName: s.sessionName(id)})
		return protocol.SessionInfo{}, err
	}
	return res, nil
}

func (s *Server) respawn(old *session.Session, rec sessionstore.Record) (protocol.SessionInfo, error) {
	id := rec.ID
	if rec.Pid > 0 && processAlive(rec.Pid, rec.Cmd) {
		return protocol.SessionInfo{}, resumeErr(id, "still_running", map[string]any{"pid": rec.Pid},
			"session %s: its previous process (pid %d) is still running; stop it or discard the session", id, rec.Pid)
	}
	proj, err := s.projects.Get(rec.ProjectID)
	if err != nil {
		return protocol.SessionInfo{}, resumeErr(id, "project_missing", map[string]any{"projectId": rec.ProjectID},
			"session %s: project %s is no longer registered", id, rec.ProjectID)
	}
	lp, err := s.resolveLaunch(rec.ProfileID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	cwd, g := rec.Cwd, sessGit{Mode: protocol.ModeSame, Dir: rec.Cwd}
	if rec.WorktreeID != "" {
		wrec, err := s.wts.Get(rec.WorktreeID)
		if err != nil {
			return protocol.SessionInfo{}, apierr.New(apierr.DirMissing, map[string]any{"dir": rec.Cwd},
				"the worktree of session %s no longer exists: %s", id, rec.Cwd)
		}
		if _, err := os.Stat(wrec.Path); err != nil {
			return protocol.SessionInfo{}, apierr.New(apierr.DirMissing, map[string]any{"dir": wrec.Path},
				"the worktree folder no longer exists: %s", wrec.Path)
		}
		g = recGit(wrec)
		if _, err := os.Stat(cwd); err != nil {
			cwd = recCwd(wrec)
		}
	} else if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return protocol.SessionInfo{}, apierr.New(apierr.DirMissing, map[string]any{"dir": cwd},
			"the folder of session %s no longer exists: %s", id, cwd)
	}
	args, mode := restartArgs(lp.prof, s.agentArgs(lp, proj), rec.Resume)
	s.mu.Lock()
	s.git[id] = g
	s.mu.Unlock()
	cols, rows := old.Size()
	s.log.Info("resuming session", "id", id, "profile", lp.prof.ID, "mode", mode, "cwd", cwd)
	return s.startSession(startSpec{
		id: id, proj: proj, lp: lp, args: args, cwd: cwd, g: g, cols: cols, rows: rows,
		scrollback: append(old.Scrollback(), s.restoredBanner()...), createdAt: old.CreatedAt, resume: rec.Resume,
	})
}

// discard forgets suspended session id (record, scrollback and all).
func (s *Server) discard(id string) (protocol.SessionInfo, error) {
	s.mu.Lock()
	sess, info := s.sessions[id], s.susp[id]
	if sess == nil {
		s.mu.Unlock()
		return protocol.SessionInfo{}, fmt.Errorf("session not found: %s", id)
	}
	if info == nil || s.resuming[id] {
		s.mu.Unlock()
		return protocol.SessionInfo{}, resumeErr(id, "not_suspended", nil, "session %s is not suspended", id)
	}
	s.mu.Unlock()
	last := s.sessionInfo(sess)
	s.mu.Lock()
	delete(s.sessions, id)
	delete(s.susp, id)
	s.order = slices.DeleteFunc(s.order, func(x string) bool { return x == id })
	s.mu.Unlock()
	s.forgetGit(id)
	s.setName(id, "")
	s.forgetModel(id)
	s.forgetPort(id)
	go s.browserForget(id)
	s.forgetHandoff(id)
	s.forgetLineage(id)
	s.forgetMessages(sess)
	sess.Release()
	s.dropPersisted(id)
	s.log.Info("suspended session discarded", "id", id)
	s.bus.publish(protocol.Event{Type: protocol.EventSessionRemoved, SessionID: id, At: time.Now().UTC()})
	if info.rec.WorktreeID != "" {
		s.publishWorktrees(info.rec.ProjectID)
	}
	return last, nil
}

// restartArgs is the argv (after the command) a restored session is
// respawned with, and how it resumes (protocol.Resume*):
//
//   - claude with a known session id: <hooks> --resume <id> <profile args>;
//   - codex with a known thread id: resume <hooks> <profile args> <id>;
//   - otherwise: <hooks> <profile args> <resume_args> (args mode), or just
//     <hooks> <profile args> without resume_args (fresh).
//
// inj is the hook injection (hooks.Injection.Args).
func restartArgs(prof profile.Profile, inj []string, r *sessionstore.Resume) ([]string, string) {
	known := r != nil && r.SessionID != "" && r.Agent == prof.SetupHooks
	switch {
	case known && prof.SetupHooks == hooks.AgentClaude:
		return concat(inj, []string{"--resume", r.SessionID}, prof.Args), protocol.ResumeAgent
	case known && prof.SetupHooks == hooks.AgentCodex:
		return concat([]string{"resume"}, inj, prof.Args, []string{r.SessionID}), protocol.ResumeAgent
	case len(prof.ResumeArgs) > 0:
		return concat(inj, prof.Args, prof.ResumeArgs), protocol.ResumeArgs
	}
	return concat(inj, prof.Args), protocol.ResumeFresh
}

func concat(parts ...[]string) []string {
	out := []string{}
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// processAlive reports whether pid still runs what a session launched:
// the process exists, leads its own process group (sessions are PTY
// session leaders) and its command mentions cmd's base name. The extra
// checks keep a recycled pid from blocking a resume.
func processAlive(pid int, cmd string) bool {
	if pid <= 0 {
		return false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if pg, err := syscall.Getpgid(pid); err != nil || pg != pid {
		return false
	}
	proc, err := peercred.Lookup(pid)
	if err != nil {
		return false
	}
	base := filepath.Base(cmd)
	return base != "" && base != "." && strings.Contains(proc.Command, base)
}

// Terminal lines written around a restored scrollback. The modes a
// full-screen program left on (alternate screen, hidden cursor, mouse
// reporting, bracketed paste, application keys) are reset first so the
// line, and whatever comes next, shows up in a sane terminal.
const termReset = "\x1b[0m\x1b[?1049l\x1b[?25h\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[?1l\x1b>"

func (s *Server) termLine(key string, at time.Time) []byte {
	st, _ := settings.Load(s.cfg.Paths.Settings())
	text := i18n.T(st.UI.Lang(), key, at.Local().Format("2006-01-02 15:04"))
	return []byte(termReset + "\r\n\x1b[2m── " + text + " ──\x1b[0m\r\n")
}

// restoredBanner separates a resumed session's old scrollback from the
// new process' output (it becomes part of the scrollback).
func (s *Server) restoredBanner() []byte { return s.termLine(i18n.TermRestored, time.Now()) }

// suspendedBanner is shown after a suspended session's replay (not stored).
func (s *Server) suspendedBanner(id string) []byte {
	s.mu.Lock()
	info := s.susp[id]
	s.mu.Unlock()
	if info == nil {
		return nil
	}
	return s.termLine(i18n.TermSuspended, info.suspendedAt)
}

// suspendedFields fills a suspended session's SessionInfo extras.
func (s *Server) suspendedFields(info *protocol.SessionInfo) {
	s.mu.Lock()
	si := s.susp[info.ID]
	var at time.Time
	var mode, msg, code string
	if si != nil {
		at, mode, msg, code = si.suspendedAt, si.mode, si.errMsg, si.errCode
	}
	s.mu.Unlock()
	if si == nil {
		return
	}
	info.SuspendedAt = timePtr(at)
	info.ResumeMode, info.ResumeError, info.ResumeErrorCode = mode, msg, code
	info.Pid = 0
}
