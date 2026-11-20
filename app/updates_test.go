package main

import (
	"testing"

	"github.com/elionaz/panal/internal/i18n"
)

func TestDecideUpdater(t *testing.T) {
	ok := bundleInfo{Path: "/Applications/panal.app", Version: "0.2.0", PublicEDKey: "abc=", HasSparkle: true}
	for _, c := range []struct {
		name  string
		built bool
		b     bundleInfo
		want  string
	}{
		// Homebrew installs included: they update through Sparkle too.
		{"enabled", true, ok, updNone},
		{"no sparkle build", false, ok, updUnsupported},
		{"no bundle", true, bundleInfo{PublicEDKey: "abc=", HasSparkle: true}, updNoBundle},
		{"framework missing", true, bundleInfo{Path: ok.Path, PublicEDKey: "abc="}, updNoBundle},
		{"no key", true, bundleInfo{Path: ok.Path, HasSparkle: true}, updNoKey},
		{"placeholder key", true, bundleInfo{Path: ok.Path, HasSparkle: true, PublicEDKey: "@SPARKLE_PUBLIC_ED_KEY@"}, updNoKey},
	} {
		if got := decideUpdater(c.built, c.b); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUpdatesMenuText(t *testing.T) {
	if title, hint := updatesMenuText(i18n.ES, updNone); title != "Buscar actualizaciones…" || hint != "" {
		t.Errorf("enabled: %q %q", title, hint)
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
	if ok, err := a.InstallUpdateNow(); err == nil || ok {
		t.Error("InstallUpdateNow without Sparkle should fail")
	}
}
