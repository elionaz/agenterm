package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Installs updated in place from agenterm (Sparkle keeps the bundle's
// path) are still /Applications/agenterm.app, and macOS shows a bundle's
// file name, not its CFBundleName: the Dock and Finder would say
// "agenterm" forever. At launch, before any window, the app renames its
// own bundle to panal.app and relaunches from there; the daemon, still
// running the old path, is then restarted like after an update (its
// executable is gone: DaemonStatus.Moved).

const (
	legacyBundle = "agenterm.app"
	bundleName   = "panal.app"
)

// renameTarget is where bundle should move ("" to stay): only a bundle
// named agenterm.app, outside a disk image or App Translocation, with no
// panal.app next to it.
func renameTarget(bundle string, exists func(string) bool) string {
	if bundle == "" || filepath.Base(bundle) != legacyBundle {
		return ""
	}
	if strings.HasPrefix(bundle, "/Volumes/") || strings.Contains(bundle, "/AppTranslocation/") {
		return ""
	}
	target := filepath.Join(filepath.Dir(bundle), bundleName)
	if exists(target) {
		return ""
	}
	return target
}

// relinkDirs are where the CLI's symlinks into the bundle live (Homebrew's
// binary stanza, or ones the user made).
func relinkDirs(home string) []string {
	return []string{"/opt/homebrew/bin", "/usr/local/bin", filepath.Join(home, ".local", "bin"), filepath.Join(home, "bin")}
}

// relink points the symlinks in dirs whose target is inside oldBundle at
// the same file in newBundle (Homebrew's agenterm and panal commands would
// break otherwise). It returns the links it changed.
func relink(dirs []string, oldBundle, newBundle string) []string {
	var done []string
	prefix := oldBundle + "/"
	for _, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			link := filepath.Join(d, e.Name())
			target, err := os.Readlink(link)
			if err != nil || !strings.HasPrefix(target, prefix) {
				continue
			}
			next := newBundle + "/" + strings.TrimPrefix(target, prefix)
			tmp := link + ".panal-relink"
			_ = os.Remove(tmp)
			if os.Symlink(next, tmp) != nil {
				continue
			}
			if os.Rename(tmp, link) != nil {
				_ = os.Remove(tmp)
				continue
			}
			done = append(done, link)
		}
	}
	return done
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return !errors.Is(err, os.ErrNotExist)
}

// addPanalLinks makes a panal command next to every agenterm one that
// points into bundle (Homebrew's old agenterm cask linked only agenterm,
// so after the rename `panal` was not on the PATH). A dir that already has
// a panal entry, of any kind, is left alone. It returns the links made.
func addPanalLinks(dirs []string, bundle string) []string {
	cli := filepath.Join(bundle, "Contents", "Helpers", "panal")
	if bundle == "" || !pathExists(cli) {
		return nil
	}
	var done []string
	for _, d := range dirs {
		old := filepath.Join(d, "agenterm")
		target, err := os.Readlink(old)
		if err != nil || !strings.HasPrefix(target, bundle+"/") {
			continue
		}
		link := filepath.Join(d, "panal")
		if pathExists(link) {
			continue
		}
		if os.Symlink(cli, link) == nil {
			done = append(done, link)
		}
	}
	return done
}
