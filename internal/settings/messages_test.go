package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessagesAutoDeliver(t *testing.T) {
	if d := Defaults(); !d.Messages.Enabled || !d.Messages.AutoDeliver {
		t.Fatalf("defaults: %+v", d.Messages)
	}
	path := filepath.Join(t.TempDir(), "settings.yaml")
	// An explicit false is kept (it is on by default).
	os.WriteFile(path, []byte("messages:\n  auto_deliver: false\n"), 0o600)
	if s, err := Load(path); err != nil || s.Messages.AutoDeliver {
		t.Fatalf("explicit false: %+v, %v", s.Messages, err)
	}
	os.WriteFile(path, []byte("messages:\n  auto_deliver: true\n"), 0o600)
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Messages.Enabled || !s.Messages.AutoDeliver {
		t.Fatalf("loaded: %+v", s.Messages)
	}
	s.Messages.AutoDeliver = false
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "auto_deliver: false") {
		t.Fatalf("saved:\n%s", data)
	}
	if s, _ := Load(path); s.Messages.AutoDeliver {
		t.Fatal("auto_deliver not saved")
	}
}
