package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"time"

	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/session"
	"github.com/elionaz/agenterm/internal/settings"
	"github.com/elionaz/agenterm/internal/status"
)

func (s *Server) dispatch(req protocol.Request) (any, error) {
	switch req.Method {
	case protocol.MethodDaemonPing:
		return map[string]any{"ok": true}, nil
	case protocol.MethodDaemonShutdown:
		s.stopOnce.Do(func() { close(s.stop) })
		return map[string]any{"ok": true}, nil

	case protocol.MethodProjectAdd:
		p, err := decode[protocol.ProjectAddParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.projects.Add(p.Path, p.Name)
	case protocol.MethodProjectList:
		return protocol.ProjectList{Projects: s.projects.List()}, nil
	case protocol.MethodProjectRemove:
		p, err := decode[protocol.ProjectRef](req.Params)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, s.projects.Remove(p.ID)

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
		if err != nil {
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
		return st, nil

	case protocol.MethodSessionSpawn:
		p, err := decode[protocol.SpawnParams](req.Params)
		if err != nil {
			return nil, err
		}
		return s.spawn(p)
	case protocol.MethodSessionList:
		return s.list(), nil
	case protocol.MethodSessionKill:
		p, err := decode[protocol.SessionRef](req.Params)
		if err != nil {
			return nil, err
		}
		return s.kill(p.SessionID)
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
		return sessionInfo(sess), nil
	case protocol.MethodSessionNotify:
		p, err := decode[protocol.NotifyParams](req.Params)
		if err != nil {
			return nil, err
		}
		if p.State == "" && p.Activity != "" {
			sess, err := s.lookup(p.SessionID)
			if err != nil {
				return nil, err
			}
			sess.SetActivity(p.Activity)
			return sessionInfo(sess), nil
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
		if err := sess.NotifyEvent(status.Event{State: st, Kind: kind, Message: p.Message, Activity: p.Activity, Turn: p.Turn}); err != nil {
			return nil, err
		}
		return sessionInfo(sess), nil
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
		return sessionInfo(sess), nil
	}
	return nil, fmt.Errorf("unknown method %q", req.Method)
}

func profileList(ps []profile.Profile, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return protocol.ProfileList{Profiles: ps}, nil
}

func (s *Server) spawn(p protocol.SpawnParams) (protocol.SessionInfo, error) {
	proj, err := s.projects.Get(p.ProjectID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	profiles, err := s.profiles.List()
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	prof, ok := profile.Find(profiles, p.ProfileID)
	if !ok {
		return protocol.SessionInfo{}, fmt.Errorf("profile not found: %s", p.ProfileID)
	}
	if prof.Error != "" {
		return protocol.SessionInfo{}, fmt.Errorf("profile %s: %s", prof.ID, prof.Error)
	}
	if !prof.Installed {
		msg := fmt.Sprintf("profile %s: command %q not found in PATH", prof.ID, prof.Command())
		if prof.Install != "" {
			msg += " (install: " + prof.Install + ")"
		}
		return protocol.SessionInfo{}, errors.New(msg)
	}
	// Detection also searches the login shell's PATH; if this process
	// cannot find the command itself, launch the detected binary.
	cmd := prof.Command()
	if _, err := exec.LookPath(cmd); err != nil {
		cmd = prof.Path
	}
	patterns, err := prof.Patterns()
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	inj, err := hooks.Setup(prof.SetupHooks, s.cfg.Paths.HooksDir(), s.cfg.Exe)
	if err != nil {
		return protocol.SessionInfo{}, fmt.Errorf("profile %s: %w", prof.ID, err)
	}
	idle := s.cfg.IdleAfter
	if prof.IdleSeconds > 0 {
		idle = time.Duration(prof.IdleSeconds * float64(time.Second))
	}
	st, err := settings.Load(s.cfg.Paths.Settings())
	if err != nil {
		s.log.Warn("settings", "err", err)
	}

	s.mu.Lock()
	id := newID()
	for s.sessions[id] != nil {
		id = newID()
	}
	s.mu.Unlock()

	sess, err := session.Spawn(session.Options{
		ID:          id,
		ProjectID:   proj.ID,
		ProjectDir:  proj.Path,
		ProjectName: proj.Name,
		ProfileID:   prof.ID,
		Cmd:         cmd,
		Args:        append(inj.Args, prof.Args...),
		Env:         append(prof.Environ(), inj.Env...),
		SockPath:    s.sock,
		Cols:        p.Cols,
		Rows:        p.Rows,
		RingSize:    s.cfg.RingSize,
		StatusMode:  prof.Status,
		IdleAfter:   idle,
		OnState:     func(c status.Change) { s.publishState(id, proj.Name, prof.ID, c) },
		OnActivity:  s.publishActivity,
		StallAfter:  st.Activity.StallAfter(),

		AttentionPatterns: patterns,
	})
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.order = append(s.order, id)
	s.mu.Unlock()
	s.log.Info("session spawned", "id", id, "project", proj.Name, "profile", prof.ID, "pid", sess.Pid)

	info := sessionInfo(sess)
	s.bus.publish(protocol.Event{Type: protocol.EventSessionSpawned, SessionID: id, State: info.State, At: time.Now().UTC(),
		ProjectName: proj.Name, ProfileID: prof.ID})
	go func() {
		<-sess.Done()
		code, _ := sess.ExitCode()
		s.log.Info("session exited", "id", id, "code", code)
	}()
	return info, nil
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
		out.Sessions = append(out.Sessions, sessionInfo(sess))
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
		s.bus.publish(protocol.Event{Type: protocol.EventSessionRemoved, SessionID: id, At: time.Now().UTC()})
		return sessionInfo(sess), nil
	}
	sess.Kill(s.cfg.KillGrace)
	return sessionInfo(sess), nil
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
	if p.Cols > 0 && p.Rows > 0 {
		_ = sess.Resize(p.Cols, p.Rows) // fails only if already exited
	}
	snapshot, sub := sess.Attach()
	defer sub.Close()
	if err := writeResponse(conn, req.ID, sessionInfo(sess), nil); err != nil {
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
				if _, err := sess.Write(f.Payload); err != nil {
					return
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
		if code, exited := sess.ExitCode(); exited {
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
