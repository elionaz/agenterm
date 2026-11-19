package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"syscall"

	"github.com/elionaz/agenterm/internal/browser"
	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/protocol"
)

const browserUsage = "usage: agenterm browser [--session ID] [open|url|close|status|mcp]"

// cmdBrowser: the session's own Chrome (inside a session: its own).
//
//	open (default)  start it unless it runs; print its DevTools URL
//	url             its DevTools URL, without starting it
//	close           stop it
//	status          the agents' browser settings and what was found
//	mcp             run the browser MCP server bound to it (stdio; what
//	                Claude Code and codex sessions get)
func cmdBrowser(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("browser", flag.ContinueOnError)
	sessionID := fs.String("session", os.Getenv("AGENTERM_SESSION"), "session id")
	sock := fs.String("sock", "", "daemon socket (agent integrations pass it)")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if *sock != "" {
		c = client.New(*sock)
	}
	sub := "open"
	switch fs.NArg() {
	case 0:
	case 1:
		sub = fs.Arg(0)
	default:
		return errors.New(browserUsage)
	}
	if sub == "status" {
		st, err := c.BrowserStatus(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("enabled   %v\nheadless  %v\nbrowser   %s\nnpx       %s\nmcp       %s\n",
			st.Enabled, st.Headless, orNone(st.Exe), orNone(st.NPX), st.MCP)
		return nil
	}
	if *sessionID == "" {
		return errors.New("not inside an agenterm session: pass --session ID")
	}
	switch sub {
	case "open":
		info, err := c.Browser(ctx, protocol.MethodBrowserOpen, *sessionID)
		if err != nil {
			return err
		}
		fmt.Println(info.URL)
		return nil
	case "url":
		info, err := c.Browser(ctx, protocol.MethodBrowserPrepare, *sessionID)
		if err != nil {
			return err
		}
		fmt.Println(info.URL)
		return nil
	case "close":
		_, err := c.Browser(ctx, protocol.MethodBrowserClose, *sessionID)
		return err
	case "mcp":
		return browserMCP(ctx, c, *sessionID)
	}
	return errors.New(browserUsage)
}

// browserMCP replaces this process with the browser MCP server (stdout is
// its JSON-RPC channel: nothing else may be printed there).
func browserMCP(ctx context.Context, c *client.Client, sessionID string) error {
	st, err := c.BrowserStatus(ctx)
	if err != nil {
		return err
	}
	if st.NPX == "" {
		return errors.New("npx not found: install Node.js to give agents the browser MCP server")
	}
	info, err := c.Browser(ctx, protocol.MethodBrowserPrepare, sessionID)
	if err != nil {
		return err
	}
	if info.Exe == "" && !info.Running {
		return errors.New("no Chrome-compatible browser found: install Google Chrome or set browser.path")
	}
	spec := browser.Spec{Exe: info.Exe, Profile: info.Profile, Port: info.Port, Headless: info.Headless}
	argv := append([]string{"npx"}, spec.MCPArgs(info.Running)...)
	return syscall.Exec(st.NPX, argv, os.Environ())
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
