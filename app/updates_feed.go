package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elionaz/agenterm/internal/version"
)

// Without Sparkle (Linux), updates are not installed by the app: it reads
// the latest release's appcast.xml (the feed Sparkle uses on macOS) at
// start and every feedEvery, and when it names a newer version the banner
// offers its download page (reason "manual").

const (
	updManual    = "manual"
	releaseFeed  = "https://github.com/elionaz/agenterm/releases/latest/download/appcast.xml"
	releasePage  = "https://github.com/elionaz/agenterm/releases/tag/v"
	feedEvery    = 6 * time.Hour
	feedFirst    = 10 * time.Second
	feedTimeout  = 20 * time.Second
	feedMaxBytes = 1 << 20
)

var feedVersion = regexp.MustCompile(`<sparkle:shortVersionString>\s*([0-9]+\.[0-9]+\.[0-9]+)\s*</sparkle:shortVersionString>`)

// feedLatest is the highest version an appcast lists ("" none).
func feedLatest(xml string) string {
	best := ""
	for _, m := range feedVersion.FindAllStringSubmatch(xml, -1) {
		if best == "" || newerVersion(m[1], best) {
			best = m[1]
		}
	}
	return best
}

// newerVersion reports whether a (X.Y.Z) is newer than b; a b that is not
// X.Y.Z (a dev build) is never older.
func newerVersion(a, b string) bool {
	pa, okA := semver(a)
	pb, okB := semver(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func semver(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func fetchFeed(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, feedTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "agenterm/"+version.Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBytes))
	return string(b), err
}

// checkFeed looks for a newer release and records it (with its page).
func (a *App) checkFeed(ctx context.Context) error {
	xml, err := fetchFeed(ctx, releaseFeed)
	if err != nil {
		return err
	}
	a.upd.mu.Lock()
	a.upd.lastFeed = time.Now()
	a.upd.mu.Unlock()
	if v := feedLatest(xml); newerVersion(v, version.Version) {
		a.setAvailable(&UpdateInfo{Version: v, URL: releasePage + v})
	} else {
		a.setAvailable(nil)
	}
	return nil
}

// feedLoop checks at start and every feedEvery until ctx ends.
func (a *App) feedLoop(ctx context.Context) {
	t := time.NewTimer(feedFirst)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := a.checkFeed(ctx); err != nil {
			log.Printf("agenterm: update check: %v", err)
		}
		t.Reset(feedEvery)
	}
}
