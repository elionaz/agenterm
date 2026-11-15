package settings

import (
	"os"
	"path/filepath"
	"testing"
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
	want := Settings{Notify: Notify{Enabled: true, Sound: true, OnDone: false}}
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
