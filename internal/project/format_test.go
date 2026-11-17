package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func TestLegacyFileIsReadAndUpgraded(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	dir := t.TempDir()
	legacy := `[{"id":"ab12cd34","name":"old","path":"/nowhere","addedAt":"2026-01-01T00:00:00Z"}]`
	if err := os.WriteFile(file, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(file)
	if err != nil || len(r.List()) != 1 || r.List()[0].Name != "old" {
		t.Fatalf("legacy: %v %v", r, err)
	}
	if _, err := r.Add(dir, "new"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"projects\": [") {
		t.Fatalf("not rewritten with a version:\n%s", data)
	}
	r2, err := Open(file)
	if err != nil || len(r2.List()) != 2 {
		t.Fatalf("reopen: %v %v", r2, err)
	}
}

func TestNewerFormatIsRefused(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	newer := `{"version":99,"projects":[]}`
	if err := os.WriteFile(file, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(file); !fileformat.IsTooNew(err) {
		t.Fatalf("got %v", err)
	}
	if data, _ := os.ReadFile(file); string(data) != newer {
		t.Fatal("file was modified")
	}
}
