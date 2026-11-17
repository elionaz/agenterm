package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/version"
)

const testAppcast = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>agenterm</title>
    <item>
      <title>agenterm 0.3.0</title>
      <sparkle:version>0.3.0</sparkle:version>
      <sparkle:shortVersionString>0.3.0</sparkle:shortVersionString>
      <enclosure url="https://example.invalid/a.zip" length="1" type="application/octet-stream" sparkle:edSignature="x"/>
    </item>
    <item>
      <title>agenterm 0.4.0-rc.1</title>
      <sparkle:shortVersionString>0.4.0-rc.1</sparkle:shortVersionString>
    </item>
    <item>
      <title>old style</title>
      <enclosure url="https://example.invalid/b.zip" sparkle:shortVersionString="0.2.5"/>
    </item>
    <item><sparkle:shortVersionString>garbage</sparkle:shortVersionString></item>
  </channel>
</rss>`

func serveAppcast(t *testing.T, body string, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestLatestRelease(t *testing.T) {
	srv, _ := serveAppcast(t, testAppcast, http.StatusOK)
	v, err := latestRelease(context.Background(), srv.Client(), srv.URL)
	if err != nil || v != "0.3.0" {
		t.Fatalf("latestRelease = %q, %v; want 0.3.0 (pre-releases and garbage skipped)", v, err)
	}

	bad, _ := serveAppcast(t, "not found", http.StatusNotFound)
	if _, err := latestRelease(context.Background(), bad.Client(), bad.URL); err == nil {
		t.Error("404 should fail")
	}
	junk, _ := serveAppcast(t, "<rss><channel><item>", http.StatusOK)
	if _, err := latestRelease(context.Background(), junk.Client(), junk.URL); err == nil {
		t.Error("truncated XML should fail")
	}
}

func TestNewerRelease(t *testing.T) {
	for _, c := range []struct {
		latest, current string
		want            bool
	}{
		{"0.3.0", "0.2.1", true},
		{"0.2.1", "0.2.1", false},
		{"0.2.0", "0.2.1", false},
		{"0.3.0", "0.3.0-rc.1", true},
		{"0.4.0-rc.1", "0.3.0", false}, // pre-releases are not offered
		{"0.3.0", "dev", false},
		{"", "0.2.1", false},
		{"0.3.0", "garbage", false},
	} {
		if got := newerRelease(c.latest, c.current); got != c.want {
			t.Errorf("newerRelease(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestFeedURL(t *testing.T) {
	if u := feedURL(bundleInfo{}); u != defaultFeedURL {
		t.Errorf("no SUFeedURL: %q", u)
	}
	if u := feedURL(bundleInfo{FeedURL: "https://example.com/appcast.xml"}); u != "https://example.com/appcast.xml" {
		t.Errorf("SUFeedURL ignored: %q", u)
	}
}

func TestWatchFeed(t *testing.T) {
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = "0.2.1"

	srv, hits := serveAppcast(t, testAppcast, http.StatusOK)
	a := NewApp()
	a.upd.reason = updHomebrew
	rec := &recorder{}
	a.emit = rec.emit
	// A fake wall clock: ticks come every millisecond, but a check is only
	// due once 24 h have passed on it (as after a night of sleep).
	var clock atomic.Int64
	clock.Store(time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC).UnixNano())
	s := feedSchedule{
		first: time.Millisecond,
		tick:  time.Millisecond,
		every: 24 * time.Hour,
		now:   func() time.Time { return time.Unix(0, clock.Load()) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.watchFeed(ctx, srv.URL, s)
		close(done)
	}()
	waitHits := func(n int32) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for hits.Load() < n && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		if hits.Load() < n {
			t.Fatalf("appcast fetched %d times, want %d", hits.Load(), n)
		}
	}
	waitHits(1)
	time.Sleep(50 * time.Millisecond) // many ticks, no time passed
	if h := hits.Load(); h != 1 {
		t.Fatalf("fetched %d times before 24 h passed", h)
	}
	clock.Add(int64(23 * time.Hour))
	time.Sleep(20 * time.Millisecond)
	if h := hits.Load(); h != 1 {
		t.Fatalf("fetched %d times after 23 h", h)
	}
	clock.Add(int64(time.Hour))
	waitHits(2)
	cancel()
	<-done
	st := a.UpdaterState()
	if st.Available == nil || *st.Available != (UpdateInfo{Version: "0.3.0", Brew: true}) {
		t.Fatalf("Available = %+v", st.Available)
	}
	// Repeated checks finding the same release emit once.
	rec.mu.Lock()
	n := 0
	for _, e := range rec.evs {
		if e.name == evUpdateAvailable {
			n++
		}
	}
	rec.mu.Unlock()
	if n != 1 {
		t.Errorf("%d %s events, want 1", n, evUpdateAvailable)
	}

	// Development builds never check.
	version.Version = "dev"
	devSrv, devHits := serveAppcast(t, testAppcast, http.StatusOK)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	NewApp().watchFeed(ctx2, devSrv.URL, feedSchedule{first: time.Millisecond, tick: time.Millisecond, every: time.Millisecond, now: time.Now})
	if devHits.Load() != 0 {
		t.Errorf("dev build checked the appcast")
	}
}

func TestSetAvailable(t *testing.T) {
	a := NewApp()
	rec := &recorder{}
	a.emit = rec.emit
	a.setAvailable(nil) // unchanged: no event
	a.setAvailable(&UpdateInfo{Version: "0.3.0"})
	a.setAvailable(&UpdateInfo{Version: "0.3.0"})
	a.setAvailable(&UpdateInfo{Version: "0.3.0", Ready: true})
	a.setAvailable(nil)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.evs) != 3 {
		t.Fatalf("events %+v, want 3 (found, ready, gone)", rec.evs)
	}
	if u := rec.evs[1].data[0].(*UpdateInfo); !u.Ready {
		t.Errorf("second event %+v", u)
	}
	if u := rec.evs[2].data[0].(*UpdateInfo); u != nil {
		t.Errorf("last event %+v, want nil", u)
	}
}
