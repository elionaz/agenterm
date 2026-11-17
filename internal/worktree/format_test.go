package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func TestLegacyFileIsReadAndUpgraded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")
	if err := os.WriteFile(path, []byte(`[{"id":"a1","projectId":"p","branch":"agent/x"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil || len(s.List("")) != 1 {
		t.Fatalf("legacy: %v", err)
	}
	if err := s.Put(Record{ID: "b2", ProjectID: "p"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"worktrees\": [") {
		t.Fatalf("not rewritten with a version:\n%s", data)
	}
	if s2, err := Open(path); err != nil || len(s2.List("")) != 2 {
		t.Fatalf("reopen: %v", err)
	}
}

func TestNewerFormatIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"worktrees":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !fileformat.IsTooNew(err) {
		t.Fatalf("got %v", err)
	}
}
