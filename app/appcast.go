package main

import (
	"context"
	"encoding/xml"
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
	feedInterval   = 24 * time.Hour
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

// watchFeed checks the appcast after first, then every interval, until ctx
// ends; a newer release is reported as a Homebrew update. Failures are only
// logged (the next check is a day later).
func (a *App) watchFeed(ctx context.Context, url string, first, every time.Duration) {
	current := version.Version
	if current == "dev" {
		return
	}
	c := &http.Client{}
	t := time.NewTimer(first)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
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
		t.Reset(every)
	}
}
