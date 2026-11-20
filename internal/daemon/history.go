package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/history"
	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/session"
	"github.com/elionaz/panal/internal/sessionstore"
	"github.com/elionaz/panal/internal/settings"
)

// historyLineRunes cuts very long terminal lines in the archive.
const historyLineRunes = 2000

// archive keeps an ended agent session in the history (when enabled):
// its record, the user's prompts and its terminal text. Shells are left
// out, and so are sessions a stopping daemon suspends (they are not over).
// rec is the session's persisted record (sessions.json), read before it is
// dropped.
func (s *Server) archive(sess *session.Session, rec sessionstore.Record, ended time.Time) {
	if s.stopping.Load() || rec.ID == "" {
		return
	}
	st, _ := settings.Load(s.cfg.Paths.Settings())
	if !st.History.Enabled {
		return
	}
	if rec.ProfileID == "shell" || shells[filepath.Base(rec.Cmd)] {
		return
	}
	e := history.Entry{
		ID: rec.ID, ProjectID: rec.ProjectID, ProjectName: s.projectName(rec.ProjectID, rec.ProjectID),
		ProfileID: rec.ProfileID, Agent: rec.ProfileID, Name: s.sessionName(rec.ID), Model: s.sessionModel(rec.ID),
		Cwd: rec.Cwd, Mode: rec.Mode, WorktreeID: rec.WorktreeID,
		CreatedAt: rec.CreatedAt, EndedAt: ended.UTC().Truncate(time.Second),
	}
	if e.Name == "" {
		e.Name = rec.Name
	}
	if e.Model == "" {
		e.Model = rec.Model
	}
	if ps, err := s.profiles.List(); err == nil {
		if p, ok := profile.Find(ps, rec.ProfileID); ok && p.Name != "" {
			e.Agent = p.Name
		}
	}
	if code, ok := sess.ExitCode(); ok {
		e.ExitCode = code
	}
	if r := rec.Resume; r != nil && r.SessionID != "" {
		e.Resume = &history.Resume{Agent: r.Agent, SessionID: r.SessionID}
		if r.Agent == hooks.AgentClaude {
			e.Prompts = hooks.Prompts(r.TranscriptPath)
		}
	}
	g, _ := s.sessGitOf(rec.ID)
	e.Base = g.Base
	e.Branches = s.sessionBranches(rec.ID, g)

	raw := sess.Scrollback()
	ring := s.cfg.RingSize
	if ring <= 0 {
		ring = session.DefaultRingSize
	}
	lines := session.PlainLines(raw, historyLineRunes, len(raw) >= ring)
	if len(lines) == 0 && len(e.Prompts) == 0 {
		return // it never did anything
	}
	if err := s.history.Save(e, lines, st.History.Keep); err != nil {
		s.log.Warn("history", "id", rec.ID, "err", err)
		return
	}
	s.log.Info("session archived", "id", rec.ID, "prompts", len(e.Prompts), "lines", len(lines))
	s.bus.publish(protocol.Event{Type: protocol.EventHistory, SessionID: rec.ID, ProjectID: rec.ProjectID, At: time.Now().UTC()})
}

// sessGitOf is session id's git placement, if it has one.
func (s *Server) sessGitOf(id string) (sessGit, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.git[id]
	return g, ok
}

// sessionBranches are the branches session id worked on, the current one
// first: the one checked out last, the one it started on and, in a
// worktree, the others its HEAD visited.
func (s *Server) sessionBranches(id string, g sessGit) []string {
	var out []string
	add := func(b string) {
		if b != "" && !slices.Contains(out, b) {
			out = append(out, b)
		}
	}
	if gi := s.cachedGit(id); gi != nil {
		add(gi.Head)
	}
	add(g.Branch)
	if g.Mode == protocol.ModeWorktree && g.Dir != "" {
		if _, err := os.Stat(g.Dir); err == nil {
			ctx, cancel := context.WithTimeout(s.bg, gitQueryTO)
			for _, b := range gitx.VisitedBranches(ctx, g.Dir) {
				add(b)
			}
			cancel()
		}
	}
	return out
}

func (s *Server) historySearch(p protocol.HistorySearchParams) (protocol.HistorySearchResult, error) {
	q := history.Query{Text: p.Query, Limit: p.Limit}
	if p.ProjectID != "" {
		proj, err := s.projects.Get(p.ProjectID)
		if err != nil {
			return protocol.HistorySearchResult{}, err
		}
		q.ProjectID = proj.ID
	}
	hits := s.history.Search(q)
	if hits == nil {
		hits = []history.Hit{}
	}
	return protocol.HistorySearchResult{Hits: hits}, nil
}

func (s *Server) historyGet(p protocol.HistoryRef) (protocol.HistoryDetail, error) {
	e, text, err := s.history.Get(p.ID)
	if err != nil {
		return protocol.HistoryDetail{}, historyErr(p.ID, err)
	}
	return protocol.HistoryDetail{Entry: e, Text: text, CanResume: s.canResume(e) == nil}, nil
}

func (s *Server) historyDelete(p protocol.HistoryRef) error {
	e, _, err := s.history.Get(p.ID)
	if err != nil {
		return historyErr(p.ID, err)
	}
	if err := s.history.Delete(p.ID); err != nil {
		return historyErr(p.ID, err)
	}
	s.bus.publish(protocol.Event{Type: protocol.EventHistory, SessionID: p.ID, ProjectID: e.ProjectID, At: time.Now().UTC()})
	return nil
}

func historyErr(id string, err error) error {
	if errors.Is(err, history.ErrNotFound) {
		return apierr.New(apierr.HistoryNotFound, map[string]any{"id": id}, "no session %s in the history", id)
	}
	return err
}

// canResume reports why e cannot be reopened (nil: it can): its agent
// must have given its own session id, its project must still exist and
// so must its folder.
func (s *Server) canResume(e history.Entry) error {
	if e.Resume == nil || e.Resume.SessionID == "" {
		return apierr.New(apierr.HistoryNoResume, map[string]any{"id": e.ID}, "session %s left no agent conversation to resume", e.ID)
	}
	ps, _ := s.profiles.List()
	if p, ok := profile.Find(ps, e.ProfileID); !ok || p.SetupHooks != e.Resume.Agent {
		return apierr.New(apierr.HistoryNoResume, map[string]any{"id": e.ID, "profileId": e.ProfileID}, "profile %s can no longer resume %s conversations", e.ProfileID, e.Resume.Agent)
	}
	if _, err := s.projects.Get(e.ProjectID); err != nil {
		return apierr.New(apierr.HistoryNoResume, map[string]any{"id": e.ID, "projectId": e.ProjectID}, "the project of session %s is no longer registered", e.ID)
	}
	if fi, err := os.Stat(e.Cwd); err != nil || !fi.IsDir() {
		return apierr.New(apierr.DirMissing, map[string]any{"dir": e.Cwd}, "the folder of session %s no longer exists: %s", e.ID, e.Cwd)
	}
	return nil
}

// historyResume starts a new session that reopens e's agent conversation
// in its folder (its worktree, when that is still registered).
func (s *Server) historyResume(p protocol.HistoryResumeParams) (protocol.SessionInfo, error) {
	e, _, err := s.history.Get(p.ID)
	if err != nil {
		return protocol.SessionInfo{}, historyErr(p.ID, err)
	}
	if err := s.canResume(e); err != nil {
		return protocol.SessionInfo{}, err
	}
	proj, err := s.projects.Get(e.ProjectID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	lp, err := s.resolveLaunch(e.ProfileID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	g := sessGit{Mode: protocol.ModeSame, Dir: e.Cwd}
	if e.WorktreeID != "" {
		if wrec, err := s.wts.Get(e.WorktreeID); err == nil {
			g = recGit(wrec)
		}
	}
	resume := &sessionstore.Resume{Agent: e.Resume.Agent, SessionID: e.Resume.SessionID, At: time.Now().UTC()}
	args, mode := restartArgs(lp.prof, s.agentArgs(lp, proj), resume)
	s.mu.Lock()
	id := newID()
	for s.sessions[id] != nil {
		id = newID()
	}
	s.git[id] = g
	s.mu.Unlock()
	s.setPort(id, s.assignPort(proj.ID, id, s.repoConfig(proj).Ports))
	if e.Name != "" {
		s.setName(id, e.Name)
	}
	s.log.Info("resuming archived session", "from", e.ID, "id", id, "profile", lp.prof.ID, "mode", mode, "cwd", e.Cwd)
	info, err := s.startSession(startSpec{
		id: id, proj: proj, lp: lp, args: args, cwd: e.Cwd, g: g, cols: p.Cols, rows: p.Rows,
		scrollback: s.restoredBanner(), resume: resume,
	})
	if err != nil {
		s.mu.Lock()
		delete(s.git, id)
		s.mu.Unlock()
		s.forgetPort(id)
		s.setName(id, "")
		return protocol.SessionInfo{}, err
	}
	return info, nil
}
