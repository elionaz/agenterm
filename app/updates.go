package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elionaz/agenterm/internal/i18n"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/version"
)

// In-app updates (Sparkle 2, updater_sparkle_darwin.go). Release builds are
// made with -tags sparkle and Sparkle.framework in Contents/Frameworks;
// every other build gets updater_stub.go and reports reason "unsupported".

// Why the updater is off (UpdaterState.Reason).
const (
	updNone        = ""
	updUnsupported = "unsupported" // built without Sparkle (dev builds, other OSes)
	updNoBundle    = "nobundle"    // not running from agenterm.app, or Sparkle.framework missing
	updNoKey       = "nokey"       // no SUPublicEDKey (built without SPARKLE_PUBLIC_ED_KEY)
	updHomebrew    = "homebrew"    // installed as a Homebrew cask: brew upgrades it
	updError       = "error"       // Sparkle refused to start (see the log)
)

// UpdaterState is what Settings → General shows.
type UpdaterState struct {
	Enabled            bool   `json:"enabled"`
	AutomaticChecks    bool   `json:"automaticChecks"`
	AutomaticDownloads bool   `json:"automaticDownloads"`
	LastCheck          string `json:"lastCheck"` // RFC 3339, "" if never
	CanCheck           bool   `json:"canCheck"`
	Reason             string `json:"reason"`
	Version            string `json:"version"` // this build's
	// A newer release found by Sparkle or, for Homebrew installs, by the
	// appcast check (appcast.go); nil when none.
	Available *UpdateInfo `json:"available,omitempty"`
}

// evUpdateAvailable carries an *UpdateInfo (null: no update any more).
const evUpdateAvailable = "updates.available"

// UpdateInfo is a newer release than the running one.
type UpdateInfo struct {
	Version string `json:"version"`
	// Downloaded by Sparkle: installed on quit, or now from the prompt
	// InstallUpdateNow brings up.
	Ready bool `json:"ready"`
	// Homebrew install: `brew upgrade --cask agenterm` updates it.
	Brew bool `json:"brew"`
}

// bundleInfo describes the running bundle (from NSBundle).
type bundleInfo struct {
	Path        string // …/agenterm.app ("" when not in a bundle)
	Version     string // CFBundleShortVersionString
	PublicEDKey string // SUPublicEDKey
	HasSparkle  bool   // Sparkle.framework loaded
	FeedURL     string // SUFeedURL
}

// sparkleStatus is SPUUpdater's state.
type sparkleStatus struct {
	auto, autoDownload, canCheck bool
	lastCheck                    time.Time
}

type updaterCtl struct {
	mu        sync.Mutex
	reason    string      // updStarting until startUpdater ran
	available *UpdateInfo // see UpdaterState.Available
	// Homebrew installs: the appcast watcher's request channel (nil when
	// it isn't running) and its last successful check.
	feedCheck chan chan error
	feedLast  time.Time
}

const updStarting = "starting"

// startUpdater decides whether Sparkle may run and starts it; it must be
// called once, after the Wails app is up. It also installs the "Check for
// Updates…" menu item, which shows a hint instead when the updater is off.
func (a *App) startUpdater(p paths.Paths) {
	b := sparkleBundle()
	reason := decideUpdater(sparkleBuilt, b, homebrewPrefixes(), dirExists)
	if reason == updNone {
		f := a.setAvailable
		onSparkleUpdate.Store(&f)
		if err := sparkleStart(); err != nil {
			log.Printf("agenterm: updater: %v", err)
			reason = updError
		}
	} else {
		log.Printf("agenterm: in-app updates off (%s)", reason)
	}
	a.upd.mu.Lock()
	a.upd.reason = reason
	a.upd.mu.Unlock()
	if reason == updHomebrew && a.wailsCtx != nil {
		go a.watchFeed(a.wailsCtx, feedURL(b), defaultFeedSchedule)
	}
	// The language is read now, not at launch: a change made meanwhile
	// skipped the menu (reason was still "starting").
	title, hint := updatesMenuText(uiLang(p), reason)
	installUpdatesMenu(title, hint)
}

func updatesMenuText(lang i18n.Lang, reason string) (title, hint string) {
	switch reason {
	case updNone:
	case updHomebrew:
		hint = i18n.T(lang, i18n.UpdatesHomebrew)
	default:
		hint = i18n.T(lang, i18n.UpdatesUnavailable)
	}
	return i18n.T(lang, i18n.MenuCheckUpdates), hint
}

// onSparkleUpdate is called by Sparkle's delegate (updater_sparkle_darwin.m)
// with the update found (nil: none any more), from another goroutine.
var onSparkleUpdate atomic.Pointer[func(*UpdateInfo)]

// setAvailable records the update found (nil: none) and tells the UI when
// it changed.
func (a *App) setAvailable(u *UpdateInfo) {
	a.upd.mu.Lock()
	old := a.upd.available
	same := (old == nil && u == nil) || (old != nil && u != nil && *old == *u)
	a.upd.available = u
	a.upd.mu.Unlock()
	if !same && a.emit != nil {
		a.emit(evUpdateAvailable, u)
	}
}

func (a *App) updaterReason() string {
	a.upd.mu.Lock()
	defer a.upd.mu.Unlock()
	return a.upd.reason
}

// decideUpdater is "" when Sparkle may start, else why not.
func decideUpdater(built bool, b bundleInfo, brewPrefixes []string, exists func(string) bool) string {
	switch {
	case !built:
		return updUnsupported
	case b.Path == "" || !b.HasSparkle:
		return updNoBundle
	case b.PublicEDKey == "" || strings.HasPrefix(b.PublicEDKey, "@"):
		// "@SPARKLE_PUBLIC_ED_KEY@": the Info.plist placeholder, unsubstituted.
		return updNoKey
	case homebrewCask(b.Path, b.Version, brewPrefixes, exists):
		return updHomebrew
	}
	return updNone
}

// homebrewCask reports whether the bundle at path (version v) was installed
// by `brew install --cask agenterm`. Homebrew moves the .app to the appdir
// (/Applications) and keeps <prefix>/Caskroom/agenterm/<version>/ as its
// record, so: the bundle resolves into a Caskroom, or that record exists
// for this very version (a newer copy installed by hand over a brew one is
// not brew's, and may update itself).
func homebrewCask(path, v string, prefixes []string, exists func(string) bool) bool {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	for _, p := range prefixes {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		room := filepath.Join(p, "Caskroom")
		if strings.HasPrefix(path, room+string(filepath.Separator)) {
			return true
		}
		if v != "" && exists(filepath.Join(room, "agenterm", v)) {
			return true
		}
	}
	return false
}

// homebrewPrefixes: $HOMEBREW_PREFIX (rarely set for GUI apps), then the
// defaults for Apple silicon and Intel.
func homebrewPrefixes() []string {
	out := []string{}
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		out = append(out, p)
	}
	return append(out, "/opt/homebrew", "/usr/local")
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// --- Bindings -------------------------------------------------------------------

// UpdaterState reports the in-app updater's settings and status.
func (a *App) UpdaterState() UpdaterState {
	a.upd.mu.Lock()
	st := UpdaterState{Reason: a.upd.reason, Version: version.Version}
	if u := a.upd.available; u != nil {
		c := *u
		st.Available = &c
	}
	if st.Reason == updHomebrew {
		// Sparkle is off, but the appcast watcher can check on demand.
		st.CanCheck = a.upd.feedCheck != nil
		if !a.upd.feedLast.IsZero() {
			st.LastCheck = a.upd.feedLast.UTC().Format(time.RFC3339)
		}
	}
	a.upd.mu.Unlock()
	if st.Reason != updNone {
		return st
	}
	s := sparkleState()
	st.Enabled = true
	st.AutomaticChecks, st.AutomaticDownloads, st.CanCheck = s.auto, s.autoDownload, s.canCheck
	if !s.lastCheck.IsZero() {
		st.LastCheck = s.lastCheck.UTC().Format(time.RFC3339)
	}
	return st
}

// CheckForUpdates runs a user-initiated check (Sparkle's own window shows
// the result). Homebrew installs check the appcast instead; a newer release
// shows up as the update banner.
func (a *App) CheckForUpdates() error {
	r := a.updaterReason()
	if r == updHomebrew {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		return a.checkFeedNow(ctx)
	}
	if r != updNone {
		_, hint := updatesMenuText(uiLang(a.paths), r)
		return errors.New(hint)
	}
	sparkleCheck()
	return nil
}

// InstallUpdateNow brings Sparkle's window with the update found to the
// front: for a downloaded update, its "Install and Relaunch" prompt (the
// daemon keeps running). It is false, and does nothing, while Sparkle is
// still downloading the update in the background (canCheckForUpdates NO).
func (a *App) InstallUpdateNow() (bool, error) {
	if r := a.updaterReason(); r != updNone {
		_, hint := updatesMenuText(uiLang(a.paths), r)
		return false, errors.New(hint)
	}
	return sparkleInstallNow(), nil
}

// SetAutomaticChecks turns Sparkle's daily background check on or off.
func (a *App) SetAutomaticChecks(on bool) UpdaterState {
	if a.updaterReason() == updNone {
		sparkleSetAutomaticChecks(on)
	}
	return a.UpdaterState()
}

// SetAutomaticDownloads lets Sparkle download and install updates silently
// (on quit) instead of asking.
func (a *App) SetAutomaticDownloads(on bool) UpdaterState {
	if a.updaterReason() == updNone {
		sparkleSetAutomaticDownloads(on)
	}
	return a.UpdaterState()
}
