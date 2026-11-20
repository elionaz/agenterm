//go:build linux

package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
)

// daemonExe is what the app starts the daemon with on Linux: the agenterm
// CLI shipped next to it (the .deb: /usr/lib/agenterm/agenterm; the
// AppImage: usr/bin/agenterm inside it), so the daemon and the agents'
// hooks run the cgo-free CLI. An AppImage is mounted only while it runs,
// and the daemon outlives the window: its CLI is copied first to
// ~/.local/share/agenterm/bin/agenterm, a path that stays. Without a CLI
// next to it the app runs the daemon itself.
func daemonExe(app string) string {
	cli := filepath.Join(filepath.Dir(app), "agenterm")
	if fi, err := os.Stat(cli); err != nil || !fi.Mode().IsRegular() || cli == app {
		return app
	}
	if os.Getenv("APPIMAGE") == "" {
		return cli
	}
	stable, err := stableCopy(cli)
	if err != nil {
		log.Printf("agenterm: copy the CLI out of the AppImage: %v (the daemon runs from the mount)", err)
		return cli
	}
	return stable
}

func stableCopy(cli string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "agenterm", "bin")
	dst := filepath.Join(dir, "agenterm")
	src, err := os.ReadFile(cli)
	if err != nil {
		return "", err
	}
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, src) {
		return dst, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, src, 0o755); err != nil {
		return "", err
	}
	// A running daemon keeps its (unlinked) old copy; the rename is atomic.
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}
