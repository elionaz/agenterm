package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func TestSaveAddsVersionFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	// A file from before 0.4.0: no version key, a comment and an unknown key.
	legacy := "# mine\nui:\n  language: es\nextra: kept\n"
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.UI.Language != "es" {
		t.Fatalf("legacy: %+v %v", s.UI, err)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	out := string(data)
	if !strings.HasPrefix(out, "# mine\nversion: 1\n") || !strings.Contains(out, "extra: kept") {
		t.Fatalf("got:\n%s", out)
	}
	// Saving again keeps a single version key.
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Count(string(data), "version:") != 1 {
		t.Fatalf("version duplicated:\n%s", data)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestNewerSettingsAreReadButNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	newer := "version: 2\nui:\n  language: es\nfuture:\n  thing: 1\n"
	if err := os.WriteFile(path, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if !fileformat.IsTooNew(err) || s.UI.Language != "es" {
		t.Fatalf("load: %+v %v", s.UI, err)
	}
	if err := Save(path, Defaults()); !fileformat.IsTooNew(err) {
		t.Fatalf("save: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != newer {
		t.Fatal("newer file was modified")
	}
}
