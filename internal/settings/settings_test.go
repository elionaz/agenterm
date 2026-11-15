package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultsWhenMissing(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() || !s.Notify.Enabled || !s.Notify.Sound || !s.Notify.OnDone {
		t.Fatalf("got %+v", s)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.yaml")
	os.WriteFile(p, []byte("notify:\n  sound: false\n"), 0o600)
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Notify.Enabled || s.Notify.Sound || !s.Notify.OnDone {
		t.Fatalf("got %+v", s)
	}
	if s.Notify.WantsSound("permission") {
		t.Fatal("sound disabled")
	}
}

func TestBrokenFileFallsBack(t *testing.T) {
	s, err := Parse([]byte("notify: [nope"))
	if err == nil || s != Defaults() {
		t.Fatalf("got %+v, %v", s, err)
	}
}

func TestShouldNotify(t *testing.T) {
	n := Notify{Enabled: true, Sound: true}
	if n.ShouldNotify("done") || !n.ShouldNotify("permission") {
		t.Fatal("notify_on_done=false")
	}
	if !n.WantsSound("question") || n.WantsSound("done") {
		t.Fatal("sound kinds")
	}
	if (Notify{}).ShouldNotify("permission") {
		t.Fatal("disabled")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.yaml")
	os.WriteFile(p, []byte("notify:\n  sound: false\n"), 0o600)
	want := Settings{Notify: Notify{Enabled: true, Sound: true, OnDone: false}, Activity: Activity{StallSeconds: 60}}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != want {
		t.Fatalf("got %+v %v", got, err)
	}
	if bak, _ := os.ReadFile(p + ".bak"); string(bak) != "notify:\n  sound: false\n" {
		t.Fatalf(".bak = %q", bak)
	}
}

func TestStallDefaultsAndParse(t *testing.T) {
	d := Defaults()
	if d.Activity.StallSeconds != 180 || !d.Notify.OnStall || d.Activity.StallAfter() != 180*time.Second {
		t.Fatalf("defaults %+v", d)
	}
	s, err := Parse([]byte("activity:\n  stall_seconds: 0\nnotify:\n  notify_on_stall: false\n"))
	if err != nil || s.Activity.StallSeconds != 0 || s.Notify.OnStall || s.Activity.StallAfter() != 0 {
		t.Fatalf("got %+v %v", s, err)
	}
	if s.Notify.ShouldNotify("stall") || !d.Notify.ShouldNotify("stall") || d.Notify.WantsSound("stall") {
		t.Fatal("stall toggles")
	}
}

func TestSavePreservesUnknownKeysAndComments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.yaml")
	orig := "# mis ajustes\nnotify:\n  sound: false # silencio\n  future_toggle: yes\ntheme: dark\nextra:\n  - a\n  - b\n"
	os.WriteFile(p, []byte(orig), 0o600)
	want := Defaults()
	want.Notify.Sound = true
	want.Notify.OnStall = false
	want.Activity.StallSeconds = 300
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != want {
		t.Fatalf("round trip: got %+v %v", got, err)
	}
	data, _ := os.ReadFile(p)
	text := string(data)
	for _, keep := range []string{"# mis ajustes", "future_toggle: yes", "theme: dark", "- a", "# silencio", "stall_seconds: 300", "notify_on_stall: false"} {
		if !strings.Contains(text, keep) {
			t.Errorf("lost %q in:\n%s", keep, text)
		}
	}
	// Saving again is stable.
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(p); string(again) != text {
		t.Fatalf("not idempotent:\n%s\n---\n%s", text, again)
	}
}

func TestSaveNewAndBrokenFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.yaml")
	if err := Save(p, Defaults()); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got != Defaults() {
		t.Fatalf("new: %+v %v", got, err)
	}
	if data, _ := os.ReadFile(p); !strings.HasPrefix(string(data), "# agenterm") {
		t.Fatalf("header: %s", data)
	}
	os.WriteFile(p, []byte("notify: [broken"), 0o600)
	if err := Save(p, Defaults()); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got != Defaults() {
		t.Fatalf("broken replaced: %+v %v", got, err)
	}
}
