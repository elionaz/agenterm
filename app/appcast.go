package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/elionaz/agenterm/internal/version"
)

// Homebrew installs keep Sparkle off (brew upgrades them), so a light check
// of the same appcast tells them a newer release exists.

const (
	defaultFeedURL = "https://github.com/elionaz/agenterm/releases/latest/download/appcast.xml"
	feedFirstCheck = 10 * time.Second
	feedInterval   = 6 * time.Hour
	feedTick       = time.Hour
	feedTimeout    = 15 * time.Second
)

// feedURL is the bundle's SUFeedURL, else the release feed.
func feedURL(b bundleInfo) string {
	if strings.HasPrefix(b.FeedURL, "https://") {
		return b.FeedURL
	}
	return defaultFeedURL
}

type appcastFeed struct {
	Items []struct {
		Short     string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
		Enclosure struct {
			Short string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString,attr"`
		} `xml:"enclosure"`
	} `xml:"channel>item"`
}

// latestRelease returns the newest release (not a pre-release) the appcast
// at url lists ("" if none).
func latestRelease(ctx context.Context, c *http.Client, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, feedTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("appcast: %s", resp.Status)
	}
	var feed appcastFeed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return "", fmt.Errorf("appcast: %w", err)
	}
	latest := ""
	for _, it := range feed.Items {
		v := strings.TrimSpace(it.Short)
		if v == "" {
			v = strings.TrimSpace(it.Enclosure.Short)
		}
		if _, pre, ok := parseVersion(v); !ok || pre != "" {
			continue
		}
		if c, _ := compareVersions(v, latest); latest == "" || c > 0 {
			latest = v
		}
	}
	return latest, nil
}

// newerRelease: latest is a release newer than current. Development builds
// and unparseable versions never are.
func newerRelease(latest, current string) bool {
	if current == "dev" {
		return false
	}
	if _, pre, ok := parseVersion(latest); !ok || pre != "" {
		return false
	}
	c, ok := compareVersions(latest, current)
	return ok && c > 0
}

// feedSchedule: the first check after first, then one whenever every has
// passed on the wall clock since the last successful one, looked at each
// tick (monotonic timers stop while the Mac sleeps, so a plain 24 h timer
// could take days).
type feedSchedule struct {
	first, tick, every time.Duration
	now                func() time.Time
}

var defaultFeedSchedule = feedSchedule{first: feedFirstCheck, tick: feedTick, every: feedInterval, now: time.Now}

// watchFeed checks the appcast on schedule s until ctx ends; a newer release
// is reported as a Homebrew update. Failures are only logged and retried at
// the next tick (an hour later). checkFeedNow (Settings → Check now) asks
// for a check right away.
func (a *App) watchFeed(ctx context.Context, url string, s feedSchedule) {
	current := version.Version
	if current == "dev" {
		return
	}
	reqs := make(chan chan error)
	a.upd.mu.Lock()
	a.upd.feedCheck = reqs
	a.upd.mu.Unlock()
	defer func() {
		a.upd.mu.Lock()
		a.upd.feedCheck = nil
		a.upd.mu.Unlock()
	}()
	c := &http.Client{}
	t := time.NewTimer(s.first)
	defer t.Stop()
	var last time.Time // wall clock (Round(0) drops the monotonic reading)
	for {
		var reply chan error
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			t.Reset(s.tick)
		case reply = <-reqs:
		}
		now := s.now().Round(0)
		if reply == nil && !last.IsZero() && now.Sub(last) < s.every {
			continue
		}
		latest, err := latestRelease(ctx, c, url)
		switch {
		case err != nil:
			log.Printf("agenterm: update check: %v", err)
		case newerRelease(latest, current):
			a.setAvailable(&UpdateInfo{Version: latest, Brew: true})
		default:
			a.setAvailable(nil)
		}
		if err == nil {
			last = now
			a.upd.mu.Lock()
			a.upd.feedLast = now
			a.upd.mu.Unlock()
		}
		if reply != nil {
			reply <- err
		}
	}
}

// checkFeedNow runs an appcast check now and waits for it. It fails when no
// watcher runs (not a Homebrew install, or a development build).
func (a *App) checkFeedNow(ctx context.Context) error {
	a.upd.mu.Lock()
	reqs := a.upd.feedCheck
	a.upd.mu.Unlock()
	if reqs == nil {
		return errors.New("update checks are not running")
	}
	ctx, cancel := context.WithTimeout(ctx, feedTimeout+5*time.Second)
	defer cancel()
	reply := make(chan error, 1)
	select {
	case reqs <- reply:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
