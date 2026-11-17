package sessionstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func TestLegacyFileIsReadAndUpgraded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	legacy := `[{"id":"a1b2","projectId":"p","profileId":"shell","cwd":"/","cmd":"sh","createdAt":"2026-01-01T00:00:00Z","state":"suspended"}]`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, backup, err := Open(path)
	if err != nil || backup != "" || len(s.List()) != 1 {
		t.Fatalf("legacy: %v %q %v", s.List(), backup, err)
	}
	if err := s.Delete("a1b2"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{\n  \"version\": 1,\n  \"sessions\": []\n}\n" {
		t.Fatalf("not rewritten with a version:\n%s", data)
	}
}

// A newer file must not be moved aside like a corrupt one: that would drop
// its sessions when an older build runs after a downgrade.
func TestNewerFormatIsRefusedAndKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	newer := `{"version":2,"sessions":[{"id":"a1b2"}]}`
	if err := os.WriteFile(path, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(path); !fileformat.IsTooNew(err) {
		t.Fatalf("got %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != newer {
		t.Fatal("file was modified")
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.corrupt-*")); len(m) != 0 {
		t.Fatalf("moved aside: %v", m)
	}
}
