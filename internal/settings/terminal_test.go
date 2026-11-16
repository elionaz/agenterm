package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalAndAppearance(t *testing.T) {
	d := Defaults()
	if d.UI.Appearance != AppearanceSystem || d.Terminal != (Terminal{FontFamily: DefaultFontFamily, FontSize: 13, LineHeight: 1.15, CursorStyle: "block", Theme: "agenterm-dark"}) {
		t.Fatalf("defaults %+v %+v", d.UI, d.Terminal)
	}
	// Out-of-range and unknown values are clamped / fall back.
	s, err := Parse([]byte("ui:\n  appearance: LIGHT\nterminal:\n  font_size: 99\n  line_height: 0.5\n  cursor_style: beam\n  theme: nope\n  font_family: \"  \"\n"))
	if err != nil || s.UI.Appearance != AppearanceLight || s.Terminal.FontSize != MaxFontSize || s.Terminal.LineHeight != 1 ||
		s.Terminal.CursorStyle != DefaultCursor || s.Terminal.Theme != DefaultTheme || s.Terminal.FontFamily != DefaultFontFamily {
		t.Fatalf("parse: %+v %+v %v", s.UI, s.Terminal, err)
	}
	if s, _ := Parse([]byte("ui:\n  appearance: sepia\nterminal:\n  font_size: 2\n")); s.UI.Appearance != AppearanceSystem || s.Terminal.FontSize != MinFontSize {
		t.Fatalf("fallbacks: %+v %+v", s.UI, s.Terminal)
	}
	// Round trip, in place, next to unknown terminal keys.
	p := filepath.Join(t.TempDir(), "settings.yaml")
	os.WriteFile(p, []byte("terminal:\n  ligatures: true # mine\n"), 0o600)
	want := Defaults()
	want.UI.Appearance = AppearanceDark
	want.Terminal = Terminal{FontFamily: `"JetBrains Mono", monospace`, FontSize: 15, LineHeight: 1.25, CursorStyle: "underline", Theme: "gruvbox-dark"}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got != want {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	data, _ := os.ReadFile(p)
	for _, keep := range []string{"ligatures: true # mine", "font_size: 15", "line_height: 1.25", "cursor_style: underline", "theme: gruvbox-dark", "appearance: dark"} {
		if !strings.Contains(string(data), keep) {
			t.Errorf("missing %q in:\n%s", keep, data)
		}
	}
	// Every theme name survives a round trip.
	for _, th := range Themes {
		want.Terminal.Theme = th
		if err := Save(p, want); err != nil {
			t.Fatal(err)
		}
		if got, _ := Load(p); got.Terminal.Theme != th {
			t.Errorf("theme %s saved as %s", th, got.Terminal.Theme)
		}
	}
	// A zero Terminal (an old client's full Settings) is saved as the defaults.
	if err := Save(p, Settings{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(p); got.Terminal != Defaults().Terminal {
		t.Fatalf("zero terminal saved as %+v", got.Terminal)
	}
}
