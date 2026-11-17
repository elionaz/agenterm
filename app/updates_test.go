package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elionaz/agenterm/internal/i18n"
)

func TestHomebrewCask(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "brew")
	apps := filepath.Join(root, "Applications")
	for _, d := range []string{filepath.Join(prefix, "Caskroom", "agenterm", "0.2.0"), filepath.Join(apps, "agenterm.app")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bundle := filepath.Join(apps, "agenterm.app")
	prefixes := []string{filepath.Join(root, "nobrew"), prefix}

	// The cask's record for this version: installed by brew.
	if !homebrewCask(bundle, "0.2.0", prefixes, dirExists) {
		t.Error("Caskroom/agenterm/0.2.0 not detected")
	}
	// A different (e.g. hand-installed, newer) version is not brew's.
	if homebrewCask(bundle, "0.3.0", prefixes, dirExists) {
		t.Error("0.3.0 detected as brew-installed")
	}
	if homebrewCask(bundle, "0.2.0", []string{filepath.Join(root, "nobrew")}, dirExists) {
		t.Error("detected without a Caskroom")
	}
	// A bundle inside (or symlinked into) a Caskroom.
	inRoom := filepath.Join(prefix, "Caskroom", "agenterm", "0.2.0", "agenterm.app")
	if err := os.MkdirAll(inRoom, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.app")
	if err := os.Symlink(inRoom, link); err != nil {
		t.Fatal(err)
	}
	if !homebrewCask(link, "", prefixes, dirExists) {
		t.Error("symlink into the Caskroom not detected")
	}
}

func TestDecideUpdater(t *testing.T) {
	none := func(string) bool { return false }
	ok := bundleInfo{Path: "/Applications/agenterm.app", Version: "0.2.0", PublicEDKey: "abc=", HasSparkle: true}
	for _, c := range []struct {
		name  string
		built bool
		b     bundleInfo
		brew  bool
		want  string
	}{
		{"enabled", true, ok, false, updNone},
		{"no sparkle build", false, ok, false, updUnsupported},
		{"no bundle", true, bundleInfo{PublicEDKey: "abc=", HasSparkle: true}, false, updNoBundle},
		{"framework missing", true, bundleInfo{Path: ok.Path, PublicEDKey: "abc="}, false, updNoBundle},
		{"no key", true, bundleInfo{Path: ok.Path, HasSparkle: true}, false, updNoKey},
		{"placeholder key", true, bundleInfo{Path: ok.Path, HasSparkle: true, PublicEDKey: "@SPARKLE_PUBLIC_ED_KEY@"}, false, updNoKey},
		{"homebrew", true, ok, true, updHomebrew},
	} {
		exists := none
		if c.brew {
			exists = func(p string) bool { return p == "/opt/homebrew/Caskroom/agenterm/0.2.0" }
		}
		if got := decideUpdater(c.built, c.b, []string{"/opt/homebrew"}, exists); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUpdatesMenuText(t *testing.T) {
	if title, hint := updatesMenuText(i18n.ES, updNone); title != "Buscar actualizaciones…" || hint != "" {
		t.Errorf("enabled: %q %q", title, hint)
	}
	if _, hint := updatesMenuText(i18n.EN, updHomebrew); hint != i18n.T(i18n.EN, i18n.UpdatesHomebrew) {
		t.Errorf("homebrew hint %q", hint)
	}
	if _, hint := updatesMenuText(i18n.EN, updNoKey); hint != i18n.T(i18n.EN, i18n.UpdatesUnavailable) {
		t.Errorf("nokey hint %q", hint)
	}
	// Without Sparkle the bindings report why and refuse to check.
	a := NewApp()
	a.upd.reason = updUnsupported
	if st := a.UpdaterState(); st.Enabled || st.Reason != updUnsupported || st.Version == "" {
		t.Errorf("UpdaterState = %+v", st)
	}
	if err := a.CheckForUpdates(); err == nil {
		t.Error("CheckForUpdates without Sparkle should fail")
	}
	if err := a.InstallUpdateNow(); err == nil {
		t.Error("InstallUpdateNow without Sparkle should fail")
	}
}
