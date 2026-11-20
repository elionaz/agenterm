// Package browser runs the isolated Chrome agenterm gives each session for
// agents' browser work (tests, screenshots, checking a page), so they never
// drive the user's own Chrome: its own profile (no tabs, cookies or
// logins of the user's), its own window size and its own DevTools port.
//
// Chrome starts only when an agent needs it: either the browser MCP server
// (chrome-devtools-mcp, run by `agenterm browser mcp`) launches it on its
// first tool call, or `agenterm browser` asks the daemon to launch it and
// prints its DevTools URL. Both use the session's profile folder and port,
// so they share one Chrome.
package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Defaults for a session's Chrome.
const (
	Width    = 1280
	Height   = 800
	BasePort = 9222
	// MCPPackage is the browser MCP server `agenterm browser mcp` runs
	// (pinned: a new release must not change what agents get unasked).
	MCPPackage = "chrome-devtools-mcp@1.13.0"
)

// candidates are the Chrome-compatible browsers looked for, in order.
func candidates() []string {
	if runtime.GOOS == "darwin" {
		var out []string
		for _, app := range []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Chromium.app/Contents/MacOS/Chromium",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
		} {
			out = append(out, filepath.Join("/Applications", app))
			if home, err := os.UserHomeDir(); err == nil {
				out = append(out, filepath.Join(home, "Applications", app))
			}
		}
		return out
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}
}

// Find is the browser to run: path when set (it must exist), else the
// first installed candidate (looked up in PATH by lookPath on Linux);
// "" when there is none.
func Find(path string, lookPath func(string) (string, error)) string {
	if path = strings.TrimSpace(path); path != "" {
		if isExec(path) {
			return path
		}
		return ""
	}
	for _, c := range candidates() {
		if filepath.IsAbs(c) {
			if isExec(c) {
				return c
			}
			continue
		}
		if p, err := lookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func isExec(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

// Spec is one session's Chrome.
type Spec struct {
	Exe      string
	Profile  string // user data dir
	Port     int    // DevTools port on 127.0.0.1
	Headless bool
}

// URL is the DevTools HTTP endpoint (what --browserUrl / Playwright's
// connectOverCDP take).
func (s Spec) URL() string { return "http://127.0.0.1:" + strconv.Itoa(s.Port) }

// ChromeArgs launch it.
func (s Spec) ChromeArgs() []string {
	args := []string{
		"--user-data-dir=" + s.Profile,
		"--remote-debugging-port=" + strconv.Itoa(s.Port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		fmt.Sprintf("--window-size=%d,%d", Width, Height),
	}
	if s.Headless {
		args = append(args, "--headless=new")
	}
	return append(args, "about:blank")
}

// MCPArgs are the npx arguments of the MCP server: attach to the
// session's Chrome when it already runs, else launch it on the first tool
// call with the same profile and port.
func (s Spec) MCPArgs(running bool) []string {
	// No usage statistics, no performance traces sent to Google's CrUX API.
	args := []string{"-y", MCPPackage, "--no-usage-statistics", "--no-performance-crux"}
	if running {
		return append(args, "--browserUrl", s.URL())
	}
	args = append(args,
		"--executablePath", s.Exe,
		"--userDataDir", s.Profile,
		"--viewport", fmt.Sprintf("%dx%d", Width, Height),
		"--chromeArg=--remote-debugging-port="+strconv.Itoa(s.Port),
		"--chromeArg=--remote-debugging-address=127.0.0.1",
		"--chromeArg=--disable-sync",
	)
	if s.Headless {
		args = append(args, "--headless")
	}
	return args
}

// Alive reports whether a DevTools endpoint answers on port.
func Alive(ctx context.Context, port int) bool {
	ctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+"/json/version", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// PortFree reports whether nothing listens on 127.0.0.1:port.
func PortFree(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// Launch starts Chrome for s, detached in its own process group, and
// waits until its DevTools endpoint answers.
func Launch(ctx context.Context, s Spec, timeout time.Duration) (*os.Process, error) {
	if s.Exe == "" {
		return nil, errors.New("no Chrome-compatible browser found")
	}
	if err := os.MkdirAll(s.Profile, 0o700); err != nil {
		return nil, err
	}
	cmd := exec.Command(s.Exe, s.ChromeArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }() // reap it whenever it exits
	deadline := time.Now().Add(timeout)
	for !Alive(ctx, s.Port) {
		if time.Now().After(deadline) || ctx.Err() != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			return nil, fmt.Errorf("the browser did not open its DevTools port %d in %s", s.Port, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cmd.Process, nil
}

// ProfileProcs are the pids of processes started with --user-data-dir=dir
// (the browser, whoever launched it, and its helpers); ps lists them.
func ProfileProcs(ctx context.Context, dir string) []int {
	out, err := exec.CommandContext(ctx, "ps", "axww", "-o", "pid=,command=").Output()
	if err != nil {
		return nil
	}
	return matchProfile(string(out), dir, os.Getpid())
}

func matchProfile(ps, dir string, self int) []int {
	want := "--user-data-dir=" + dir
	var pids []int
	for _, l := range strings.Split(ps, "\n") {
		pidStr, cmd, ok := strings.Cut(strings.TrimSpace(l), " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil || pid == self {
			continue
		}
		if strings.Contains(" "+strings.TrimSpace(cmd)+" ", " "+want+" ") {
			pids = append(pids, pid)
		}
	}
	return pids
}

// Stop ends every process using profile dir: SIGTERM, then SIGKILL for
// those still there after grace.
func Stop(ctx context.Context, dir string, grace time.Duration) {
	pids := ProfileProcs(ctx, dir)
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGTERM)
	}
	deadline := time.Now().Add(grace)
	for len(pids) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		pids = ProfileProcs(ctx, dir)
	}
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGKILL)
	}
}
