package profile

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Lookup resolves a command (already $VAR-expanded) to an executable path.
type Lookup func(cmd string) (path string, ok bool)

// Detector finds installed agents. The search PATH is expensive to compute
// (it may come from the login shell), so it is cached for TTL; lookups
// themselves are a few stat calls and always run fresh.
type Detector struct {
	// SearchPath returns the PATH list to search. nil means $PATH.
	SearchPath func() string
	// TTL bounds how long SearchPath's result is reused (0: 5 minutes).
	TTL time.Duration

	mu   sync.Mutex
	path string
	at   time.Time
}

// Path returns the (cached) search PATH.
func (d *Detector) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	ttl := d.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if d.at.IsZero() || time.Since(d.at) > ttl {
		if d.SearchPath != nil {
			d.path = d.SearchPath()
		} else {
			d.path = os.Getenv("PATH")
		}
		d.at = time.Now()
	}
	return d.path
}

// Rescan drops the cached search PATH.
func (d *Detector) Rescan() {
	d.mu.Lock()
	d.at = time.Time{}
	d.mu.Unlock()
}

// Lookup implements Lookup against Path().
func (d *Detector) Lookup(cmd string) (string, bool) {
	return LookPathIn(cmd, d.Path())
}

// LookPathIn is exec.LookPath against an explicit PATH list. Commands with
// a slash are checked as-is (relative ones are never resolved).
func LookPathIn(cmd, pathList string) (string, bool) {
	if cmd == "" {
		return "", false
	}
	if strings.Contains(cmd, "/") {
		if filepath.IsAbs(cmd) && isExecutable(cmd) {
			return cmd, true
		}
		return "", false
	}
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, cmd)
		if isExecutable(p) {
			return p, true
		}
	}
	return "", false
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}
