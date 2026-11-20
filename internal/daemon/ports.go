package daemon

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/project"
	"github.com/elionaz/panal/internal/repoconfig"
)

// A port per session, so two agents starting dev servers don't both want
// 3000. Every session gets PANAL_PORT; when the repository's
// .panal.yaml asks for ports (ports.base), PORT (or the variables in
// ports.env) is set to it as well and the UIs show it. The port is the
// lowest one from the base (3000 by default) that no other live session of
// the project holds and nothing listens on; a session keeps it when it is
// resumed.

const (
	defaultPortBase = 3000
	portRange       = 100 // ports tried from the base
)

// sessPort is a session's port and the variables set to it besides
// PANAL_PORT (none: the project did not ask for ports).
type sessPort struct {
	Port int
	Env  []string
}

func (s *Server) sessionPort(id string) sessPort {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	return s.ports[id]
}

func (s *Server) setPort(id string, p sessPort) {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	if p.Port == 0 {
		delete(s.ports, id)
		return
	}
	s.ports[id] = p
}

func (s *Server) forgetPort(id string) { s.setPort(id, sessPort{}) }

// portEnv is the environment a session gets for p.
func portEnv(p sessPort) []string {
	if p.Port == 0 {
		return nil
	}
	v := strconv.Itoa(p.Port)
	env := []string{"PANAL_PORT=" + v}
	for _, name := range p.Env {
		env = append(env, name+"="+v)
	}
	return env
}

// portFree reports whether nothing listens on port (any interface).
var portFree = func(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// assignPort picks session id's port in project projectID (see above).
func (s *Server) assignPort(projectID, id string, cfg repoconfig.Ports) sessPort {
	base := cfg.Base
	if base == 0 {
		base = defaultPortBase
	}
	taken := map[int]bool{}
	s.mu.Lock()
	var others []string
	for oid, sess := range s.sessions {
		if oid != id && sess.ProjectID == projectID {
			others = append(others, oid)
		}
	}
	s.mu.Unlock()
	for _, oid := range others {
		if p := s.sessionPort(oid); p.Port != 0 {
			taken[p.Port] = true
		}
	}
	for port := base; port < base+portRange && port <= 65535; port++ {
		if !taken[port] && portFree(port) {
			return sessPort{Port: port, Env: cfg.Env}
		}
	}
	return sessPort{}
}

// repoConfig reads the .panal.yaml of proj's repository (zero when it
// is not a repository, has none, or the file is invalid: logged).
func (s *Server) repoConfig(proj project.Project) repoconfig.Config {
	ctx, cancel := context.WithTimeout(s.bg, gitQueryTO)
	defer cancel()
	if !gitx.IsRepo(ctx, proj.Path) {
		return repoconfig.Config{}
	}
	top, err := gitx.RepoRoot(ctx, proj.Path)
	if err != nil {
		return repoconfig.Config{}
	}
	repo, err := gitx.MainWorktree(ctx, top)
	if err != nil {
		return repoconfig.Config{}
	}
	cfg, err := repoconfig.Load(repo)
	if err != nil {
		s.log.Warn("repository config", "project", proj.Name, "err", err)
		return repoconfig.Config{}
	}
	return cfg
}
