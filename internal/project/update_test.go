package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/apierr"
)

func ptr(s string) *string { return &s }

func TestUpdate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := r.Add(t.TempDir(), "api")
	b, _ := r.Add(t.TempDir(), "web")

	got, err := r.Update(a.ID, Patch{Name: ptr("  backend "), Color: ptr("teal"), Icon: ptr(" 🚀 ")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "backend" || got.Color != "teal" || got.Icon != "🚀" || got.Path != a.Path || got.AddedAt != a.AddedAt {
		t.Fatalf("update: %+v", got)
	}
	// nil fields stay; empty ones go back to automatic.
	if got, err = r.Update(a.ID, Patch{Icon: ptr("")}); err != nil || got.Icon != "" || got.Color != "teal" || got.Name != "backend" {
		t.Fatalf("clear icon: %+v %v", got, err)
	}
	// ZWJ sequences and flags fit.
	for _, icon := range []string{"👩‍💻", "🇨🇱", "❤️", "👨‍👩‍👧‍👦", "🏴󠁧󠁢󠁳󠁣󠁴󠁿", "👩🏽‍❤️‍💋‍👨🏻", "AB"} {
		if _, err := r.Update(a.ID, Patch{Icon: ptr(icon)}); err != nil {
			t.Errorf("icon %q: %v", icon, err)
		}
	}

	// Persisted.
	r2, _ := Open(file)
	if p, _ := r2.Get(a.ID); p.Name != "backend" || p.Color != "teal" || p.Icon != "AB" {
		t.Fatalf("reload: %+v", p)
	}

	cases := []struct {
		patch Patch
		code  string
	}{
		{Patch{Name: ptr("   ")}, apierr.ProjectNameEmpty},
		{Patch{Name: ptr("web")}, apierr.ProjectNameTaken},
		{Patch{Name: ptr(b.ID)}, apierr.ProjectNameTaken},
		{Patch{Name: ptr("a\nb")}, apierr.ProjectNameInvalid},
		{Patch{Name: ptr(strings.Repeat("n", 81))}, apierr.ProjectNameInvalid},
		{Patch{Color: ptr("chartreuse")}, apierr.ProjectColorInvalid},
		{Patch{Icon: ptr(strings.Repeat("x", 17))}, apierr.ProjectIconInvalid},
		{Patch{Icon: ptr("a\x07")}, apierr.ProjectIconInvalid},
	}
	for _, c := range cases {
		before, _ := r.Get(a.ID)
		_, err := r.Update(a.ID, c.patch)
		if err == nil {
			t.Errorf("%+v: want an error", c.patch)
			continue
		}
		if code, _, _ := apierr.Code(err); code != c.code {
			t.Errorf("%+v: code %q, want %q", c.patch, code, c.code)
		}
		if after, _ := r.Get(a.ID); after != before {
			t.Errorf("%+v: a refused patch changed the project: %+v", c.patch, after)
		}
	}
	// Keeping its own name is not "taken".
	if _, err := r.Update(b.ID, Patch{Name: ptr("web")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Update("nope", Patch{}); err == nil {
		t.Fatal("unknown id: want an error")
	}
}

// A save that fails leaves the registry as it was.
func TestUpdateRollback(t *testing.T) {
	dir := t.TempDir()
	r, _ := Open(filepath.Join(dir, "cfg", "projects.json"))
	a, _ := r.Add(t.TempDir(), "api")
	os.RemoveAll(filepath.Join(dir, "cfg"))
	os.WriteFile(filepath.Join(dir, "cfg"), nil, 0o600) // a file where the folder was
	if _, err := r.Update(a.ID, Patch{Name: ptr("other")}); err == nil {
		t.Fatal("want a save error")
	}
	if p, _ := r.Get(a.ID); p.Name != "api" {
		t.Fatalf("not rolled back: %+v", p)
	}
}

// Files written before badges existed load with an automatic badge, and an
// automatic badge is not written out.
func TestBadgeFieldsOptional(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	os.WriteFile(file, []byte(`{"version":1,"projects":[{"id":"ab12","name":"api","path":"/x","addedAt":"2026-01-02T03:04:05Z"}]}`), 0o600)
	r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := r.Get("ab12"); p.Color != "" || p.Icon != "" {
		t.Fatalf("old file: %+v", p)
	}
	if _, err := r.Update("ab12", Patch{Name: ptr("api2")}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), "color") || strings.Contains(string(data), "icon") {
		t.Fatalf("empty badge fields written: %s", data)
	}
}

func TestUpdateMessages(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := r.Add(t.TempDir(), "api")
	for in, want := range map[string]string{"manual": MessagesManual, " auto ": MessagesAuto, "default": "", "": ""} {
		got, err := r.Update(a.ID, Patch{Messages: ptr(in)})
		if err != nil || got.Messages != want {
			t.Fatalf("messages %q = %+v, %v", in, got, err)
		}
	}
	if _, err := r.Update(a.ID, Patch{Messages: ptr("loud")}); err == nil {
		t.Fatal("unknown mode accepted")
	}
	r.Update(a.ID, Patch{Messages: ptr("manual")})
	re, _ := Open(file)
	if p, _ := re.Get(a.ID); p.Messages != MessagesManual {
		t.Fatalf("after reload: %+v", p)
	}
	if p, _ := r.Update(a.ID, Patch{Name: ptr("x")}); p.Messages != MessagesManual {
		t.Fatal("a name change reset the mode")
	}
}
