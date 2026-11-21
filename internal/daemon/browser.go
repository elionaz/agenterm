package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/elionaz/panal/internal/browser"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/settings"
)

// Each session's own Chrome (package browser): a profile folder under
// <runtime>/browsers/<sessionId> and a DevTools port from 9222, reserved
// when the session starts (PANAL_BROWSER_URL) and kept while it lives.
// Nothing runs until an agent needs it: the browser MCP server launches it
// on its first tool call, or browser.open (panal browser) does. The
// daemon watches the port (session.browser events), stops the browser when
// the session exits and deletes the profile when the session is removed.

const (
	browserPortRange = 200
	browserWatchTick = 3 * time.Second
	browserOpenTO    = 20 * time.Second
	browserStopGrace = 3 * time.Second
)

type browserState struct {
	mu    sync.Mutex
	ports map[string]int  // session id → reserved DevTools port
	open  map[string]bool // last seen answering
}

var validSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (s *Server) browserSettings() settings.Browser {
	st, _ := settings.Load(s.cfg.Paths.Settings())
	return st.Browser
}

func (s *Server) browsersDir() string { return filepath.Join(s.cfg.Paths.RuntimeDir, "browsers") }

func (s *Server) browserProfile(id string) string { return filepath.Join(s.browsersDir(), id) }

// browserExe is the browser sessions get ("" none).
func (s *Server) browserExe(cfg settings.Browser) string {
	return browserFind(cfg.Path, func(name string) (string, error) {
		if p, ok := profile.LookPathIn(name, s.cfg.SearchPath()); ok {
			return p, nil
		}
		return "", os.ErrNotExist
	})
}

// npxPath is the npx that runs the browser MCP server ("" none).
func (s *Server) npxPath() string {
	p, _ := profile.LookPathIn("npx", s.cfg.SearchPath())
	return p
}

func (s *Server) browserStatus() protocol.BrowserStatus {
	cfg := s.browserSettings()
	return protocol.BrowserStatus{Enabled: cfg.Enabled, Headless: cfg.Headless, Exe: s.browserExe(cfg),
		NPX: s.npxPath(), MCP: browser.MCPPackage}
}

// browserReady reports whether sessions get the browser MCP server: on,
// with a browser and npx found.
func (s *Server) browserReady() bool {
	st := s.browserStatus()
	return st.Enabled && st.Exe != "" && st.NPX != ""
}

// reserveBrowserPort is session id's DevTools port: the one it has, else
// the lowest from BasePort no other session holds and nothing listens on.
func (s *Server) reserveBrowserPort(id string) int {
	s.browsers.mu.Lock()
	defer s.browsers.mu.Unlock()
	if s.browsers.ports == nil {
		s.browsers.ports, s.browsers.open = map[string]int{}, map[string]bool{}
	}
	if p := s.browsers.ports[id]; p != 0 {
		return p
	}
	held := map[int]bool{}
	for _, p := range s.browsers.ports {
		held[p] = true
	}
	for p := browser.BasePort; p < browser.BasePort+browserPortRange; p++ {
		if !held[p] && browserPortFree(p) {
			s.browsers.ports[id] = p
			return p
		}
	}
	return 0
}

// browserPortFree and browserFind are package browser's (tests replace
// them: no test may find the machine's Chrome).
var (
	browserPortFree = browser.PortFree
	browserFind     = browser.Find
)

func (s *Server) browserPort(id string) int {
	s.browsers.mu.Lock()
	defer s.browsers.mu.Unlock()
	return s.browsers.ports[id]
}

// browserEnv is what a session gets: where its Chrome answers once open.
func (s *Server) browserEnv(id string) []string {
	if !s.browserSettings().Enabled {
		return nil
	}
	p := s.reserveBrowserPort(id)
	if p == 0 {
		return nil
	}
	return []string{fmt.Sprintf("PANAL_BROWSER_URL=http://127.0.0.1:%d", p)}
}

func (s *Server) browserSpec(id string) (browser.Spec, error) {
	if !validSessionID.MatchString(id) {
		return browser.Spec{}, fmt.Errorf("invalid session id %q", id)
	}
	if _, err := s.lookup(id); err != nil {
		return browser.Spec{}, err
	}
	cfg := s.browserSettings()
	if !cfg.Enabled {
		return browser.Spec{}, fmt.Errorf("the agents' browser is off (Settings → General, or browser.enabled in settings.yaml)")
	}
	port := s.reserveBrowserPort(id)
	if port == 0 {
		return browser.Spec{}, fmt.Errorf("no free port for the browser from %d", browser.BasePort)
	}
	return browser.Spec{Exe: s.browserExe(cfg), Profile: s.browserProfile(id), Port: port, Headless: cfg.Headless}, nil
}

func browserInfo(id string, sp browser.Spec, running bool) protocol.BrowserInfo {
	return protocol.BrowserInfo{SessionID: id, URL: sp.URL(), Port: sp.Port, Profile: sp.Profile, Exe: sp.Exe,
		Headless: sp.Headless, Running: running}
}

// browserPrepare reserves session id's Chrome without starting it.
func (s *Server) browserPrepare(id string) (protocol.BrowserInfo, error) {
	sp, err := s.browserSpec(id)
	if err != nil {
		return protocol.BrowserInfo{}, err
	}
	return browserInfo(id, sp, browser.Alive(s.bg, sp.Port)), nil
}

// browserOpen starts session id's Chrome unless it runs.
func (s *Server) browserOpen(id string) (protocol.BrowserInfo, error) {
	sp, err := s.browserSpec(id)
	if err != nil {
		return protocol.BrowserInfo{}, err
	}
	if !browser.Alive(s.bg, sp.Port) {
		if sp.Exe == "" {
			return protocol.BrowserInfo{}, fmt.Errorf("no Chrome-compatible browser found: install Google Chrome or set browser.path")
		}
		ctx, cancel := context.WithTimeout(s.bg, browserOpenTO)
		defer cancel()
		if _, err := browser.Launch(ctx, sp, browserOpenTO); err != nil {
			return protocol.BrowserInfo{}, err
		}
		s.log.Info("browser opened", "session", id, "port", sp.Port)
	}
	s.noteBrowser(id, true)
	return browserInfo(id, sp, true), nil
}

// browserClose stops session id's Chrome (whoever started it).
func (s *Server) browserClose(id string) {
	if !validSessionID.MatchString(id) {
		return
	}
	prof := s.browserProfile(id)
	if _, err := os.Stat(prof); err != nil {
		return // never opened
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserStopGrace+2*time.Second)
	defer cancel()
	browser.Stop(ctx, prof, browserStopGrace)
	s.noteBrowser(id, false)
}

// browserForget closes session id's Chrome, deletes its profile and frees
// its port (the session was removed).
func (s *Server) browserForget(id string) {
	s.browserClose(id)
	if validSessionID.MatchString(id) {
		_ = os.RemoveAll(s.browserProfile(id))
	}
	s.browsers.mu.Lock()
	delete(s.browsers.ports, id)
	delete(s.browsers.open, id)
	s.browsers.mu.Unlock()
}

// noteBrowser records whether session id's Chrome answers, publishing a
// change.
func (s *Server) noteBrowser(id string, open bool) {
	s.browsers.mu.Lock()
	if s.browsers.open == nil {
		s.browsers.ports, s.browsers.open = map[string]int{}, map[string]bool{}
	}
	changed := s.browsers.open[id] != open
	if open {
		s.browsers.open[id] = true
	} else {
		delete(s.browsers.open, id)
	}
	port := s.browsers.ports[id]
	s.browsers.mu.Unlock()
	if !changed {
		return
	}
	ev := protocol.Event{Type: protocol.EventSessionBrowser, SessionID: id, At: time.Now().UTC()}
	if open && port != 0 {
		ev.BrowserURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	s.bus.publish(ev)
}

// browserURL is session id's DevTools URL while its Chrome runs.
func (s *Server) browserURL(id string) string {
	s.browsers.mu.Lock()
	defer s.browsers.mu.Unlock()
	if !s.browsers.open[id] {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", s.browsers.ports[id])
}

// browserWatch polls the reserved ports until ctx ends (an MCP server
// opens and closes its Chrome without telling the daemon).
func (s *Server) browserWatch(ctx context.Context) {
	t := time.NewTicker(browserWatchTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.browsers.mu.Lock()
		ports := make(map[string]int, len(s.browsers.ports))
		for id, p := range s.browsers.ports {
			ports[id] = p
		}
		s.browsers.mu.Unlock()
		for id, p := range ports {
			s.noteBrowser(id, browser.Alive(ctx, p))
		}
	}
}

// browserSweep deletes the profiles of sessions that no longer exist (at
// start, after the restored ones are known).
func (s *Server) browserSweep() {
	ents, err := os.ReadDir(s.browsersDir())
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() || !validSessionID.MatchString(e.Name()) {
			continue
		}
		if _, err := s.lookup(e.Name()); err != nil {
			s.log.Info("removing a stale browser profile", "session", e.Name())
			browser.Stop(context.Background(), s.browserProfile(e.Name()), time.Second)
			_ = os.RemoveAll(s.browserProfile(e.Name()))
		}
	}
}

// browserCloseAll stops every session's Chrome (daemon shutdown); the
// profiles stay for the sessions that are resumed.
func (s *Server) browserCloseAll() {
	ents, err := os.ReadDir(s.browsersDir())
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, e := range ents {
		if e.IsDir() && validSessionID.MatchString(e.Name()) {
			wg.Go(func() { s.browserClose(e.Name()) })
		}
	}
	wg.Wait()
}

// browserNote is the instruction that steers agents to their own browser.
const browserNote = "## Browser\n\nFor browser work (opening pages, testing a web app, screenshots), use your own " +
	"isolated Chrome for this session: the `panal-browser` MCP tools, or run `panal browser` for its " +
	"DevTools URL (also in $PANAL_BROWSER_URL, e.g. for Playwright's connectOverCDP). Never drive the " +
	"user's own Chrome (no Claude in Chrome or other extension tools): it has their tabs and logins."
