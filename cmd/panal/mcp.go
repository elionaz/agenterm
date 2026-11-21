package main

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/mcpserver"
)

// cmdMcp runs panal's MCP server on stdin/stdout (what agent sessions
// get as the "panal" MCP server): tools to coordinate with the other
// sessions of the project and to read panal's state.
func cmdMcp(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	sock := fs.String("sock", "", "daemon socket (agent integrations pass it)")
	if err := parseInterspersed(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: panal mcp [--sock S]")
	}
	if *sock != "" {
		c = client.New(*sock)
	}
	return mcpserver.New(c).Serve(ctx, os.Stdin, os.Stdout)
}
