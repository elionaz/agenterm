//go:build darwin

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/elionaz/panal/internal/macnotify"
	"github.com/elionaz/panal/internal/paths"
)

// renameLegacyBundle renames agenterm.app to panal.app and relaunches the
// app from there; true when this process should exit (the new one runs).
// Any failure (a read-only folder…) keeps running from the old name.
func renameLegacyBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	bundle := macnotify.BundlePath(exe)
	target := renameTarget(bundle, pathExists)
	if target == "" {
		return false
	}
	if err := os.Rename(bundle, target); err != nil {
		log.Printf("panal: rename %s to %s: %v", bundle, target, err)
		return false
	}
	log.Printf("panal: renamed %s to %s", bundle, target)
	if home, err := os.UserHomeDir(); err == nil {
		for _, l := range relink(relinkDirs(home), bundle, target) {
			log.Printf("panal: %s now points into %s", l, target)
		}
	}
	// LaunchServices learns the new path; open -n starts it even though
	// this instance still runs (it exits right after). The home override
	// goes along, like a notification click's relaunch.
	args := []string{"-n", target}
	if h := os.Getenv(paths.EnvHome); h != "" {
		args = append([]string{"--env", paths.EnvHome + "=" + h}, args...)
	}
	if out, err := exec.Command("open", args...).CombinedOutput(); err != nil {
		log.Printf("panal: relaunch %s: %v %s", target, err, out)
		return false
	}
	return true
}
