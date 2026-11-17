package profile

import (
	"os"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func TestParseFormats(t *testing.T) {
	legacy := "- id: claude\n  args: [\"-v\"]\n- id: mine\n  cmd: x\n"
	current := "# comment\nversion: 1\nprofiles:\n  - id: claude\n    args: [\"-v\"]\n  - id: mine\n    cmd: x\n"
	for name, in := range map[string]string{"legacy list": legacy, "versioned": current} {
		es, err := Parse([]byte(in))
		if err != nil || len(es) != 2 || es[0].ID != "claude" || es[1].Cmd != "x" {
			t.Errorf("%s: %v %v", name, es, err)
		}
	}
	for name, in := range map[string]string{"empty": "", "comments only": "# nothing\n", "empty list": "[]\n", "no profiles": "version: 1\n"} {
		if es, err := Parse([]byte(in)); err != nil || len(es) != 0 {
			t.Errorf("%s: %v %v", name, es, err)
		}
	}
	if _, err := Parse([]byte("profiles: []\n")); err == nil {
		t.Error("a mapping without version is an error")
	}
	for _, in := range []string{"version: 2\nprofiles:\n  - id: a\n    future: true\n", "version: 2\nprofiles: {a: {cmd: x}}\n"} {
		if _, err := Parse([]byte(in)); !fileformat.IsTooNew(err) {
			t.Errorf("%q: %v", in, err)
		}
	}
	if _, err := Parse([]byte("version: two\nprofiles: []\n")); !fileformat.Protected(err) {
		t.Errorf("bad version: %v", err)
	}
}

func TestMarshalWritesVersion(t *testing.T) {
	data, err := Marshal([]Entry{{ID: "claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\nversion: 1\nprofiles:\n  - id: claude\n") {
		t.Fatalf("got:\n%s", data)
	}
	if es, err := Parse(data); err != nil || len(es) != 1 {
		t.Fatalf("round trip: %v %v", es, err)
	}
	empty, _ := Marshal(nil)
	if es, err := Parse(empty); err != nil || len(es) != 0 || !strings.Contains(string(empty), "version: 1\nprofiles: []\n") {
		t.Fatalf("empty:\n%s %v", empty, err)
	}
}

func TestStoreUpgradesLegacyAndKeepsNewer(t *testing.T) {
	s, path := newStore(t, "- id: claude\n  args: [\"-v\"]\n")
	if _, err := s.Reorder([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "version: 1\nprofiles:\n") || !strings.Contains(string(data), "- id: claude\n    args:") {
		t.Fatalf("legacy not upgraded:\n%s", data)
	}

	newer := "version: 2\nprofiles:\n  - id: claude\n"
	s, path = newStore(t, newer)
	if _, err := s.Save(Entry{ID: "mine", Cmd: "x"}); !fileformat.IsTooNew(err) {
		t.Fatalf("save over a newer file: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != newer {
		t.Fatal("newer file was modified")
	}
}
