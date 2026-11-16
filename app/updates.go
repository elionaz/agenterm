package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/elionaz/agenterm/internal/i18n"
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
	mu     sync.Mutex
	reason string // updStarting until startUpdater ran
}

const updStarting = "starting"

// startUpdater decides whether Sparkle may run and starts it; it must be
// called once, after the Wails app is up. It also installs the "Check for
// Updates…" menu item, which shows a hint instead when the updater is off.
func (a *App) startUpdater(lang i18n.Lang) {
	b := sparkleBundle()
	reason := decideUpdater(sparkleBuilt, b, homebrewPrefixes(), dirExists)
	if reason == updNone {
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
	title, hint := updatesMenuText(lang, reason)
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
	st := UpdaterState{Reason: a.updaterReason(), Version: version.Version}
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
	if r := a.updaterReason(); r != updNone {
		_, hint := updatesMenuText(uiLang(a.paths), r)
		return errors.New(hint)
	}
	sparkleCheck()
	return nil
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
