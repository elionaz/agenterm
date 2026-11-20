package main

import (
	"context"
	"errors"
	"log"
	goruntime "runtime"
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
// Homebrew installs update through Sparkle too (`brew upgrade` also works).

// Why the updater is off (UpdaterState.Reason).
const (
	updNone        = ""
	updUnsupported = "unsupported" // built without Sparkle (dev builds, other OSes)
	updNoBundle    = "nobundle"    // not running from agenterm.app, or Sparkle.framework missing
	updNoKey       = "nokey"       // no SUPublicEDKey (built without SPARKLE_PUBLIC_ED_KEY)
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
	// A newer release found by Sparkle; nil when none.
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
	// URL: its download page, for builds that don't install updates
	// themselves (Linux, reason "manual").
	URL string `json:"url,omitempty"`
}

// bundleInfo describes the running bundle (from NSBundle).
type bundleInfo struct {
	Path        string // …/agenterm.app ("" when not in a bundle)
	Version     string // CFBundleShortVersionString
	PublicEDKey string // SUPublicEDKey
	HasSparkle  bool   // Sparkle.framework loaded
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
	lastFeed  time.Time   // last feed check (reason "manual")
}

const updStarting = "starting"

// startUpdater decides whether Sparkle may run and starts it; it must be
// called once, after the Wails app is up. It also installs the "Check for
// Updates…" menu item, which shows a hint instead when the updater is off.
func (a *App) startUpdater(p paths.Paths) {
	if goruntime.GOOS == "linux" {
		a.upd.mu.Lock()
		a.upd.reason = updManual
		a.upd.mu.Unlock()
		if a.wailsCtx != nil {
			go a.feedLoop(a.wailsCtx)
		}
		return
	}
	b := sparkleBundle()
	reason := decideUpdater(sparkleBuilt, b)
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
	// The language is read now, not at launch: a change made meanwhile
	// skipped the menu (reason was still "starting").
	title, hint := updatesMenuText(uiLang(p), reason)
	installUpdatesMenu(title, hint)
}

func updatesMenuText(lang i18n.Lang, reason string) (title, hint string) {
	switch reason {
	case updNone:
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
func decideUpdater(built bool, b bundleInfo) string {
	switch {
	case !built:
		return updUnsupported
	case b.Path == "" || !b.HasSparkle:
		return updNoBundle
	case b.PublicEDKey == "" || strings.HasPrefix(b.PublicEDKey, "@"):
		// "@SPARKLE_PUBLIC_ED_KEY@": the Info.plist placeholder, unsubstituted.
		return updNoKey
	}
	return updNone
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
	last := a.upd.lastFeed
	a.upd.mu.Unlock()
	if st.Reason == updManual {
		st.CanCheck = true
		if !last.IsZero() {
			st.LastCheck = last.UTC().Format(time.RFC3339)
		}
		return st
	}
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
// the result).
func (a *App) CheckForUpdates() error {
	if a.updaterReason() == updManual {
		return a.checkFeed(context.Background())
	}
	if r := a.updaterReason(); r != updNone {
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
