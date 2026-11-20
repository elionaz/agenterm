package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/fileformat"
)

func TestSaveAddsVersionFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	// A file from before 0.4.0: no version key, a comment and an unknown key.
	legacy := "# mine\n\n# the UI\nui:\n  language: es\nextra: kept\n"
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
	if !strings.HasPrefix(out, "# mine\n\nversion: 1\n# the UI\nui:") || !strings.Contains(out, "extra: kept") {
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

// A newer file is recognized by its version alone, even when its keys changed
// shape (so it does not parse into this build's structure) or the version is
// not a number.
func TestNewerOrOddSettingsAreNeverOverwritten(t *testing.T) {
	for _, in := range []string{
		"version: 2\nnotify: true\n",
		"version: 2\nterminal:\n  font_size: big\n",
		"version: \"2\"\nui:\n  language: es\n",
		"version: 1.5\n",
		"version: [1]\n",
		"version: 0\n",
		"version: -1\n",
	} {
		path := filepath.Join(t.TempDir(), "settings.yaml")
		if err := os.WriteFile(path, []byte(in), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); !fileformat.Protected(err) {
			t.Errorf("%q: load: %v", in, err)
		}
		if err := Save(path, Defaults()); !fileformat.Protected(err) {
			t.Errorf("%q: save: %v", in, err)
		}
		if data, _ := os.ReadFile(path); string(data) != in {
			t.Errorf("%q: modified to %q", in, data)
		}
	}
	// A newer file that this build can still decode is applied.
	path := filepath.Join(t.TempDir(), "settings.yaml")
	_ = os.WriteFile(path, []byte("version: 2\nnotify: true\n"), 0o600)
	if _, err := Load(path); !fileformat.IsTooNew(err) {
		t.Errorf("type-changed newer file: %v", err)
	}
}
