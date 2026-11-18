package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Screenshots dragged from the macOS floating thumbnail arrive as a file in a
// temporary folder (…/TemporaryItems/NSIRD_screencaptureui_XXXX/) that macOS
// deletes as soon as the drag ends. By the time the frontend types the path
// into the terminal the file is gone, so Claude Code, which reads a dropped
// image right away, silently ignores it.
//
// So the app grabs such files the moment Wails reports the drop (grabDrops,
// on the Go side, well before the frontend hears of it): a hard link, which
// is instant and outlives the deletion, or a copy. StageDrops then hands the
// frontend the grabbed paths to type.

const screenshotTempPrefix = "NSIRD_screencaptureui_"

// dropsKeep is how long grabbed files are kept in the drops folder.
const dropsKeep = 24 * time.Hour

// dropWait bounds how long StageDrops waits for grabDrops (a variable for tests).
var dropWait = 2 * time.Second

// dropGrab is one temporary file being grabbed; done closes once path is set.
type dropGrab struct {
	done chan struct{}
	path string // the lasting copy; "" if it could not be grabbed
}

type dropGrabs struct {
	mu sync.Mutex
	m  map[string]*dropGrab
}

// get returns the grab of p, creating it: grabDrops and StageDrops race to it.
func (g *dropGrabs) get(p string) *dropGrab {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.m == nil {
		g.m = map[string]*dropGrab{}
	}
	e := g.m[p]
	if e == nil {
		e = &dropGrab{done: make(chan struct{})}
		g.m[p] = e
	}
	return e
}

func (g *dropGrabs) forget(p string) {
	g.mu.Lock()
	delete(g.m, p)
	g.mu.Unlock()
}

// grabDrops runs on every file drop Wails reports: it gives screenshot
// thumbnails a lasting copy in the drops folder before macOS deletes them.
func (a *App) grabDrops(paths []string) {
	dir := a.dropsDir()
	pruned := false
	for _, p := range paths {
		if !isScreenshotTemp(p) {
			continue
		}
		if !pruned {
			pruneDrops(dir, time.Now().Add(-dropsKeep))
			pruned = true
		}
		e := a.grabs.get(p)
		select {
		case <-e.done: // already grabbed (the same drop reported twice)
			continue
		default:
		}
		if staged, err := linkOrCopyInto(dir, p); err == nil {
			e.path = staged
		} else {
			log.Printf("agenterm: keep dropped screenshot: %v", err)
		}
		close(e.done)
	}
}

// StageDrops returns the paths a drop on a terminal should type: the dropped
// ones, with screenshot thumbnails replaced by their grabbed copy.
func (a *App) StageDrops(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p
		if !isScreenshotTemp(p) {
			continue
		}
		e := a.grabs.get(p)
		select {
		case <-e.done:
			if e.path != "" {
				out[i] = e.path
			}
		case <-time.After(dropWait):
		}
		a.grabs.forget(p)
	}
	return out
}

func (a *App) dropsDir() string { return filepath.Join(a.paths.RuntimeDir, "drops") }

// isScreenshotTemp reports whether p is a screenshot thumbnail's temporary file.
func isScreenshotTemp(p string) bool {
	return strings.HasPrefix(filepath.Base(filepath.Dir(p)), screenshotTempPrefix)
}

// linkOrCopyInto puts the regular file src into dir under its own name (with
// a numeric suffix if taken), as a hard link when possible, and returns the
// new path.
func linkOrCopyInto(dir, src string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", os.ErrInvalid
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(src)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; n < 1000; n++ {
		name := base
		if n > 1 {
			name = stem + " " + strconv.Itoa(n) + ext
		}
		dst := filepath.Join(dir, name)
		err := os.Link(src, dst)
		if err == nil {
			return dst, nil
		}
		if os.IsExist(err) {
			continue
		}
		// Another volume, or links not allowed: copy.
		err = copyFile(src, dst)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		return dst, nil
	}
	return "", os.ErrExist
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
	}
	return err
}

// pruneDrops removes grabbed files modified before before (a screenshot's
// modification time is when it was taken).
func pruneDrops(dir string, before time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && fi.Mode().IsRegular() && fi.ModTime().Before(before) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
