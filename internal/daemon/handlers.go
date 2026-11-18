package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"time"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/fileformat"
	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/order"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/sessionstore"
	"github.com/elionaz/agenterm/internal/settings"
	"github.com/elionaz/agenterm/internal/status"
	"github.com/elionaz/agenterm/internal/version"
	"github.com/elionaz/agenterm/internal/worktree"
)

func (s *Server) dispatch(req protocol.Request) (any, error) {
	switch req.Method {
	case protocol.MethodDaemonPing:
		return protocol.PingResult{OK: true, Version: version.Version, Commit: version.Short(), Protocol: protocol.Version}, nil
	case protocol.MethodProjectAdd:
		p, err := decode[protocol.ProjectAddParams](req.Params)
		if err != nil {
			return nil, err
		}
		proj, err := s.projects.Add(p.Path, p.Name)
		if err == nil {
			s.projectsChanged()
		}
		return proj, err
	case protocol.MethodProjectReorder:
		p, err := decode[protocol.ProjectReorderParams](req.Params)
		if err != nil {
			return nil, err
		}
		list, err := s.projects.Reorder(p.IDs)
		if err != nil {
			return nil, err
		}
		s.projectsChanged()
		return protocol.ProjectList{Projects: list}, nil
	case protocol.MethodProjectUpdate:
		p, err := decode[protocol.ProjectUpdateParams](req.Params)
		if err != nil {
			return nil, err
		}
		cur, err := s.projects.Get(p.ID) // an id or a name, like spawn
		if err != nil {
			return nil, err
		}
		proj, err := s.projects.Update(cur.ID, project.Patch{Name: p.Name, Color: p.Color, Icon: p.Icon})
		if err != nil {
			return nil, err
		}
		s.projectsChanged()
		return proj, nil
	case protocol.MethodProjectList:
		return protocol.ProjectList{Projects: s.projects.List()}, nil
	case protocol.MethodProjectRemove:
		p, err := decode[protocol.ProjectRef](req.Params)
		if err != nil {
			return nil, err
		}
		if err := s.projects.Remove(p.ID); err != nil {
			return nil, err
		}
		s.projectsChanged()
		return map[string]any{"ok": true}, nil

	case protocol.MethodProfileList:
		return profileList(s.profiles.List())
	case protocol.MethodProfileRescan:
		s.detector.Rescan()
		return profileList(s.profiles.List())
	case protocol.MethodProfileCatalog:
		out := protocol.ProfileCatalog{Entries: []protocol.CatalogInfo{}}
		defaults := profile.Resolve(nil, s.detector.Lookup)
		for _, c := range profile.Catalog() {
			info := protocol.CatalogInfo{CatalogEntry: c}
			p, _ := profile.Find(defaults, c.ID)
			info.Command, info.Installed, info.Path = p.Cmd, p.Installed, p.Path
			out.Entries = append(out.Entries, info)
		}
		return out, nil
	case protocol.MethodProfileSave:
		p, err := decode[protocol.ProfileSaveParams](req.Params)
		if err != nil {
			return nil, err
		}
		return profileList(s.profiles.Save(p.Entry))
	case protocol.MethodProfileDelete:
		p, err := decode[protocol.ProfileRef](req.Params)
		if err != nil {
			return nil, err
		}
		return profileList(s.profiles.Delete(p.ID))
	case protocol.MethodProfileReorder:
		p, err := decode[protocol.ProfileReorderParams](req.Params)
		if err != nil {
			return nil, err
		}
		return profileList(s.profiles.Reorder(p.IDs))

	case protocol.MethodSettingsGet:
		st, err := settings.Load(s.cfg.Paths.Settings())
		switch {
		case fileformat.IsTooNew(err):
			// What this build understands is shown (and applied); saving
			// refuses, with the same message.
			s.log.Warn("settings", "err", err)
		case fileformat.Protected(err):
			return nil, err
		case err != nil:
			return nil, fmt.Errorf("settings.yaml: %w", err)
		}
		return st, nil
	case protocol.MethodSettingsSave:
		// Fields a (possibly older) client does not send keep their
		// current values.
		st, _ := settings.Load(s.cfg.Paths.Settings())
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &st); err != nil {
				return nil, fmt.Errorf("bad params: %w", err)
			}
		}
		if err := settings.Save(s.cfg.Paths.Settings(), st); err != nil {
			return nil, err
		}
		// What was written, normalized (e.g. an unknown ui.language is auto).
		if saved, err := settings.Load(s.cfg.Paths.Settings()); err == nil {
			st = saved
		}
		return st, nil

	case protocol.MethodSessionSpawn:
		p, err := decode[protocol.SpawnParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.spawn(p)
	case protocol.MethodSessionList:
		return s.list(), nil
	case protocol.MethodSessionReorder:
		p, err := decode[protocol.SessionReorderParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.reorderSessions(p.ProjectID, p.IDs)
	case protocol.MethodSessionKill:
		p, err := decode[protocol.SessionRef](req.Params)
		if err != nil {
			return nil, err
		}
		return s.kill(p.SessionID)
	case protocol.MethodSessionResume, protocol.MethodSessionDiscard:
		p, err := decode[protocol.SuspendedRef](req.Params)
		if err != nil {
			return nil, err
		}
		if req.Method == protocol.MethodSessionResume {
			return s.batch(p, s.resume)
		}
		return s.batch(p, s.discard)
	case protocol.MethodSessionRename:
		p, err := decode[protocol.RenameParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.rename(p)
	case protocol.MethodSessionResize:
		p, err := decode[protocol.ResizeParams](req.Params)
		if err != nil {
			return nil, err
		}
		sess, err := s.lookup(p.SessionID)
		if err != nil {
			return nil, err
		}
		if err := sess.Resize(p.Cols, p.Rows); err != nil {
			return nil, err
		}
		return s.sessionInfo(sess), nil
	case protocol.MethodSessionNotify:
		p, err := decode[protocol.NotifyParams](req.Params)
		if err != nil {
			return nil, err
		}
		if p.Model != "" {
			sess, err := s.lookup(p.SessionID)
			if err != nil {
				return nil, err
			}
			s.setModel(sess, p.Model)
			if p.State == "" && p.Activity == "" && p.AgentSessionID == "" {
				return s.sessionInfo(sess), nil
			}
		}
		if p.AgentSessionID != "" {
			sess, err := s.lookup(p.SessionID)
			if err != nil {
				return nil, err
			}
			s.learnResume(sess, p.Agent, p.AgentSessionID, p.TranscriptPath)
			if p.State == "" && p.Activity == "" {
				return s.sessionInfo(sess), nil
			}
		}
		if p.State == "" && p.Activity != "" {
			sess, err := s.lookup(p.SessionID)
			if err != nil {
				return nil, err
			}
			sess.SetActivity(p.Activity, p.ActivityKind, p.ActivityTarget)
			return s.sessionInfo(sess), nil
		}
		st, err := status.ParseState(p.State)
		if err != nil {
			return nil, err
		}
		kind, err := status.ParseKind(p.Kind)
		if err != nil {
			return nil, err
		}
		sess, err := s.lookup(p.SessionID)
		if err != nil {
			return nil, err
		}
		if err := sess.NotifyEvent(status.Event{State: st, Kind: kind, Message: p.Message, Activity: p.Activity,
			ActivityKind: p.ActivityKind, ActivityTarget: p.ActivityTarget, Turn: p.Turn}); err != nil {
			return nil, err
		}
		return s.sessionInfo(sess), nil
	case protocol.MethodSessionTail:
		p, err := decode[protocol.TailParams](req.Params)
		if err != nil {
			return nil, err
		}
		sess, err := s.lookup(p.SessionID)
		if err != nil {
			return nil, err
		}
		return protocol.TailResult{SessionID: sess.ID, Lines: sess.Tail(p.Lines)}, nil
	case protocol.MethodProjectGit:
		p, err := decode[protocol.ProjectGitParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.projectGit(p)
	case protocol.MethodSessionDiff, protocol.MethodWorktreeDiff:
		p, err := decode[protocol.DiffParams](req.Params)
		if err != nil {
			return nil, err
		}
		if req.Method == protocol.MethodSessionDiff && p.SessionID == "" {
			return nil, errors.New("falta sessionId")
		}
		if req.Method == protocol.MethodWorktreeDiff && p.WorktreeID == "" {
			return nil, errors.New("falta worktreeId")
		}
		return s.diff(p)
	case protocol.MethodWorktreeList:
		p, err := decode[protocol.WorktreeListParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.worktreeList(p)
	case protocol.MethodWorktreeRemove:
		p, err := decode[protocol.WorktreeRemoveParams](req.Params)
		if err != nil {
			return nil, err
		}
		res, err := s.worktreeRemove(p)
		if err != nil {
			return nil, err
		}
		return res, nil
	case protocol.MethodWorktreeMerge:
		p, err := decode[protocol.WorktreeRef](req.Params)
		if err != nil {
			return nil, err
		}
		return s.worktreeMerge(p)
	case protocol.MethodWorktreePR:
		p, err := decode[protocol.WorktreePRParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.worktreePR(p)
	case protocol.MethodForgeStatus:
		p, err := decode[protocol.ForgeRef](req.Params)
		if err != nil {
			return nil, err
		}
		return s.forgeStatus(p)
	case protocol.MethodForgeComments:
		p, err := decode[protocol.ForgeCommentsParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.forgeComments(p)
	case protocol.MethodForgeMerge:
		p, err := decode[protocol.ForgeMergeParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.forgeMerge(p)
	case protocol.MethodForgeCreate:
		p, err := decode[protocol.ForgeCreateParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.forgeCreate(p)
	case protocol.MethodWorktreeOpen:
		p, err := decode[protocol.WorktreeOpenParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.worktreeOpen(p)
	case protocol.MethodSessionAck:
		p, err := decode[protocol.SessionRef](req.Params)
		if err != nil {
			return nil, err
		}
		sess, err := s.lookup(p.SessionID)
		if err != nil {
			return nil, err
		}
		sess.Ack()
		return s.sessionInfo(sess), nil
	}
	return nil, apierr.New(apierr.UnknownMethod, map[string]string{"method": req.Method},
		"unknown method %q (daemon %s)", req.Method, version.Version)
}

func profileList(ps []profile.Profile, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return protocol.ProfileList{Profiles: ps}, nil
}

// launchProfile resolves profile id for launching: the command to run,
// its compiled patterns, the hook injection and the idle timeout.
type launchProfile struct {
	prof     profile.Profile
	cmd      string
	patterns []*regexp.Regexp
	inj      hooks.Injection
	idle     time.Duration
}

func (s *Server) resolveLaunch(profileID string) (launchProfile, error) {
	profiles, err := s.profiles.List()
	if err != nil {
		return launchProfile{}, err
	}
	prof, ok := profile.Find(profiles, profileID)
	if !ok {
		return launchProfile{}, fmt.Errorf("profile not found: %s", profileID)
	}
	if prof.Error != "" {
		return launchProfile{}, fmt.Errorf("profile %s: %s", prof.ID, prof.Error)
	}
	if !prof.Installed {
		msg := fmt.Sprintf("profile %s: command %q not found in PATH", prof.ID, prof.Command())
		if prof.Install != "" {
			msg += " (install: " + prof.Install + ")"
		}
		return launchProfile{}, &apierr.Error{Code: apierr.ProfileNotInstalled, Msg: msg,
			Data: map[string]any{"profileId": prof.ID, "cmd": prof.Command(), "install": prof.Install}}
	}
	// Detection also searches the login shell's PATH; if this process
	// cannot find the command itself, launch the detected binary.
	cmd := prof.Command()
	if _, err := exec.LookPath(cmd); err != nil {
		cmd = prof.Path
	}
	patterns, err := prof.Patterns()
	if err != nil {
		return launchProfile{}, err
	}
	inj, err := hooks.Setup(prof.SetupHooks, s.cfg.Paths.HooksDir(), s.cfg.Exe, s.sock)
	if err != nil {
		return launchProfile{}, fmt.Errorf("profile %s: %w", prof.ID, err)
	}
	idle := s.cfg.IdleAfter
	if prof.IdleSeconds > 0 {
		idle = time.Duration(prof.IdleSeconds * float64(time.Second))
	}
	return launchProfile{prof: prof, cmd: cmd, patterns: patterns, inj: inj, idle: idle}, nil
}

func (s *Server) spawn(p protocol.SpawnParams) (protocol.SessionInfo, error) {
	proj, err := s.projects.Get(p.ProjectID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	lp, err := s.resolveLaunch(p.ProfileID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	prof := lp.prof

	s.mu.Lock()
	id := newID()
	for s.sessions[id] != nil {
		id = newID()
	}
	s.mu.Unlock()

	// Where the session runs: the project folder, a new worktree or an
	// existing one.
	mode := p.Mode
	if p.WorktreeID != "" {
		mode = protocol.ModeWorktree
	}
	cwd := proj.Path
	g := sessGit{Mode: protocol.ModeSame, Dir: proj.Path}
	var created *worktree.Record
	switch {
	case p.WorktreeID != "":
		rec, err := s.wts.Get(p.WorktreeID)
		if err != nil {
			return protocol.SessionInfo{}, err
		}
		if rec.ProjectID != proj.ID {
			return protocol.SessionInfo{}, fmt.Errorf("worktree %s belongs to another project", rec.ID)
		}
		if _, err := os.Stat(rec.Path); err != nil {
			return protocol.SessionInfo{}, apierr.New(apierr.DirMissing, map[string]any{"dir": rec.Path}, "the worktree folder no longer exists: %s", rec.Path)
		}
		cwd, g = recCwd(rec), recGit(rec)
	case mode == protocol.ModeWorktree:
		rec, dir, err := s.createWorktree(proj, prof.ID, id, p.Branch, p.Base)
		if err != nil {
			return protocol.SessionInfo{}, err
		}
		created, cwd, g = &rec, dir, recGit(rec)
	case mode == "" || mode == protocol.ModeSame:
	default:
		return protocol.SessionInfo{}, fmt.Errorf("unknown mode %q (same or worktree)", p.Mode)
	}
	s.mu.Lock()
	s.git[id] = g
	s.mu.Unlock()

	info, err := s.startSession(startSpec{
		id: id, proj: proj, lp: lp, cwd: cwd, g: g, cols: p.Cols, rows: p.Rows,
		args: append(slices.Clone(lp.inj.Args), prof.Args...),
	})
	if err != nil {
		s.mu.Lock()
		delete(s.git, id)
		s.mu.Unlock()
		if created != nil {
			s.discardWorktree(*created)
		}
		return protocol.SessionInfo{}, err
	}
	return info, nil
}

// startSpec is what startSession launches. A resumed session reuses its
// id (replacing the suspended one), creation time and scrollback.
type startSpec struct {
	id         string
	proj       project.Project
	lp         launchProfile
	args       []string
	cwd        string
	g          sessGit
	cols, rows uint16
	scrollback []byte
	createdAt  time.Time
	resume     *sessionstore.Resume
}

// startSession spawns the process, registers the session (replacing a
// suspended one with the same id), records it for restarts and announces
// it. s.git[id] must already be set.
func (s *Server) startSession(sp startSpec) (protocol.SessionInfo, error) {
	id, proj, prof := sp.id, sp.proj, sp.lp.prof
	st, err := settings.Load(s.cfg.Paths.Settings())
	if err != nil {
		s.log.Warn("settings", "err", err)
	}
	sess, err := session.Spawn(session.Options{
		ID:          id,
		ProjectID:   proj.ID,
		ProjectDir:  proj.Path,
		ProjectName: proj.Name,
		ProfileID:   prof.ID,
		Cwd:         sp.cwd,
		Cmd:         sp.lp.cmd,
		Args:        sp.args,
		Env:         append(prof.Environ(), sp.lp.inj.Env...),
		SockPath:    s.sock,
		Home:        s.homeEnv(),
		Cols:        sp.cols,
		Rows:        sp.rows,
		RingSize:    s.cfg.RingSize,
		Scrollback:  sp.scrollback,
		CreatedAt:   sp.createdAt,
		StatusMode:  prof.Status,
		IdleAfter:   sp.lp.idle,
		OnState:     func(c status.Change) { s.publishState(id, s.projectName(proj.ID, proj.Name), prof.ID, c) },
		OnActivity:  s.publishActivity,
		StallAfter:  st.Activity.StallAfter(),

		AttentionPatterns: sp.lp.patterns,
	})
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	g := sp.g
	s.mu.Lock()
	old := s.sessions[id]
	s.sessions[id] = sess
	if old == nil {
		s.order = append(s.order, id)
	}
	delete(s.susp, id)
	s.mu.Unlock()
	if old != nil {
		old.Release() // attached viewers of the suspended session reattach
	}
	if m := argsModel(sp.args); m != "" && s.sessionModel(id) == "" {
		s.namesMu.Lock()
		s.models[id] = m
		s.namesMu.Unlock()
	}
	s.recordSession(sess, g, sp.resume)
	s.log.Info("session spawned", "id", id, "project", proj.Name, "profile", prof.ID, "pid", sess.Pid, "mode", g.Mode, "cwd", sp.cwd,
		"resumed", old != nil)
	s.requestGit(id)

	info := s.sessionInfo(sess)
	s.bus.publish(protocol.Event{Type: protocol.EventSessionSpawned, SessionID: id, State: info.State, At: time.Now().UTC(),
		ProjectName: proj.Name, ProfileID: prof.ID, SessionName: info.Name})
	if g.WorktreeID != "" {
		s.publishWorktrees(proj.ID)
	}
	go func() {
		<-sess.Done()
		code, _ := sess.ExitCode()
		s.log.Info("session exited", "id", id, "code", code)
		s.forgetRecord(sess)
		if g.WorktreeID != "" {
			s.publishWorktrees(proj.ID) // no longer live (the worktree stays)
		}
	}()
	return info, nil
}

func (s *Server) projectsChanged() {
	s.bus.publish(protocol.Event{Type: protocol.EventProjects, At: time.Now().UTC()})
}

// projectName is project id's current name (it can be renamed while its
// sessions run), or fallback once the project is gone.
func (s *Server) projectName(id, fallback string) string {
	if p, err := s.projects.Get(id); err == nil && p.ID == id {
		return p.Name
	}
	return fallback
}

// reorderSessions moves ids to the front of projectID's sessions (others
// keep their positions), persists the order (restores follow it) and
// returns the whole list.
func (s *Server) reorderSessions(projectID string, ids []string) (protocol.SessionList, error) {
	proj, err := s.projects.Get(projectID)
	if err != nil {
		return protocol.SessionList{}, err
	}
	s.mu.Lock()
	next, err := order.Within(s.order, func(id string) bool {
		sess, ok := s.sessions[id]
		return ok && sess.ProjectID == proj.ID
	}, ids)
	if err != nil {
		s.mu.Unlock()
		return protocol.SessionList{}, fmt.Errorf("reorder sessions: %w", err)
	}
	s.order = next
	s.mu.Unlock()
	if err := s.store.Reorder(next); err != nil {
		s.log.Warn("persist session order", "err", err)
	}
	s.bus.publish(protocol.Event{Type: protocol.EventSessionsReordered, ProjectID: proj.ID, At: time.Now().UTC()})
	return s.list(), nil
}

func (s *Server) list() protocol.SessionList {
	s.mu.Lock()
	list := make([]*session.Session, 0, len(s.order))
	for _, id := range s.order {
		list = append(list, s.sessions[id])
	}
	s.mu.Unlock()
	out := protocol.SessionList{Sessions: make([]protocol.SessionInfo, 0, len(list))}
	for _, sess := range list {
		out.Sessions = append(out.Sessions, s.sessionInfo(sess))
	}
	return out
}

// kill terminates a live session. Killing an already-exited session removes
// it from the daemon.
func (s *Server) kill(id string) (protocol.SessionInfo, error) {
	sess, err := s.lookup(id)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	if sess.Suspended() {
		// Nothing to kill: forget it (as discard).
		return s.discard(id)
	}
	if _, exited := sess.ExitCode(); exited {
		s.mu.Lock()
		delete(s.sessions, id)
		for i, oid := range s.order {
			if oid == id {
				s.order = append(s.order[:i:i], s.order[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		info := s.sessionInfo(sess)
		s.setName(id, "")
		s.forgetModel(id)
		s.forgetMessages(sess)
		s.bus.publish(protocol.Event{Type: protocol.EventSessionRemoved, SessionID: id, At: time.Now().UTC()})
		return info, nil
	}
	sess.Kill(s.cfg.KillGrace)
	return s.sessionInfo(sess), nil
}

// handleAttach upgrades conn to the framed attach protocol: scrollback, a
// ReplayDone marker, then live output until exit or detach. Input and resize
// frames from the client are applied to the PTY.
func (s *Server) handleAttach(conn net.Conn, br *bufio.Reader, req protocol.Request) {
	p, err := decode[protocol.AttachParams](req.Params)
	if err != nil {
		writeResponse(conn, req.ID, nil, err)
		return
	}
	sess, err := s.lookup(p.SessionID)
	if err != nil {
		writeResponse(conn, req.ID, nil, err)
		return
	}
	// Input from a session (an agent running agenterm attach) is not the
	// user acting: it doesn't reset automatic delivery's chain.
	fromUser := s.callerSession(conn) == nil
	if p.Cols > 0 && p.Rows > 0 {
		_ = sess.Resize(p.Cols, p.Rows) // fails only if already exited
	}
	snapshot, sub := sess.Attach()
	defer sub.Close()
	if sess.Suspended() {
		snapshot = append(snapshot, s.suspendedBanner(sess.ID)...)
	}
	if err := writeResponse(conn, req.ID, s.sessionInfo(sess), nil); err != nil {
		return
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer sub.Close() // client went away: stop the writer
		for {
			f, err := protocol.ReadFrame(br)
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					s.log.Debug("attach read", "id", sess.ID, "err", err)
				}
				return
			}
			switch f.Type {
			case protocol.FrameInput:
				if _, err := sess.Write(f.Payload); err != nil && !sess.Suspended() {
					return
				}
				if fromUser && bytes.IndexByte(f.Payload, '\r') >= 0 {
					// The user submitted a prompt (not a newline in a draft).
					if draft, _ := sess.Draft(); !draft {
						s.userActed(sess.ID)
					}
				}
			case protocol.FrameResize:
				if cols, rows, err := protocol.ParseResize(f.Payload); err == nil {
					_ = sess.Resize(cols, rows)
				}
			}
		}
	}()

	ok := protocol.WriteChunked(conn, protocol.FrameOutput, snapshot) == nil &&
		protocol.WriteFrame(conn, protocol.FrameReplayDone, nil) == nil
	for chunk := range sub.C {
		if ok && protocol.WriteFrame(conn, protocol.FrameOutput, chunk) != nil {
			ok = false
			sub.Close()
		}
	}
	if ok {
		if sess.Suspended() {
			// Released: resumed (a new session has this id) or discarded.
			msg := "session discarded"
			if cur, err := s.lookup(sess.ID); err == nil && cur != sess {
				msg = "session resumed; reattach"
			}
			protocol.WriteFrame(conn, protocol.FrameError, []byte(msg))
		} else if _, exited := sess.ExitCode(); exited && s.stopping.Load() {
			// Killed by the daemon stopping: it is suspended, not gone, and
			// the next daemon resumes it under the same id. An exit frame
			// would make the client give the terminal up for good.
			protocol.WriteFrame(conn, protocol.FrameError, []byte("daemon stopping; reattach"))
		} else if code, exited := sess.ExitCode(); exited {
			protocol.WriteFrame(conn, protocol.FrameExit, protocol.ExitPayload(code))
		} else if sub.Dropped() {
			protocol.WriteFrame(conn, protocol.FrameError, []byte("client too slow; reattach to resync"))
		}
	}
	conn.Close()
	<-readerDone
}

// handleSubscribe streams events as JSON lines until the client disconnects.
func (s *Server) handleSubscribe(conn net.Conn, br *bufio.Reader, req protocol.Request) {
	p, err := decode[protocol.SubscribeParams](req.Params)
	if err != nil {
		writeResponse(conn, req.ID, nil, err)
		return
	}
	events, cancel := s.bus.subscribe(p.Client == protocol.ClientUI)
	defer cancel()
	if err := writeResponse(conn, req.ID, map[string]any{"ok": true}, nil); err != nil {
		return
	}
	go func() {
		_, _ = io.Copy(io.Discard, br) // returns when the client closes
		cancel()
	}()
	for ev := range events {
		b, _ := json.Marshal(ev)
		if _, err := conn.Write(append(b, '\n')); err != nil {
			return
		}
	}
}
