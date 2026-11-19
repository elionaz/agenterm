package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Security":            "security",
		"Data management":     "data-management",
		"  Seguridad / Auth ": "seguridad-auth",
		"Diseño de API":       "diseno-de-api",
		"!!!":                 "topic",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseFormat(t *testing.T) {
	in := Topic{Title: "Git", Description: "Branches and commits", Load: LoadOnDemand,
		Projects: []string{"p1", "p2"}, Enabled: false, Order: 30, Body: "Never force-push.\n\n- squash"}
	got, err := Parse(Format(in))
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != in.Title || got.Description != in.Description || got.Load != in.Load || got.Enabled ||
		got.Order != 30 || strings.Join(got.Projects, ",") != "p1,p2" || got.Body != in.Body {
		t.Fatalf("round trip = %+v", got)
	}
	if !strings.Contains(string(Format(in)), "projects: [p1, p2]") ||
		!strings.Contains(string(Format(Topic{Title: "x", Load: LoadAlways, Enabled: true})), "projects: all") {
		t.Fatalf("format:\n%s", Format(in))
	}

	// A plain file (written by hand): the whole text, always, everywhere.
	plain, err := Parse([]byte("Use pnpm.\n"))
	if err != nil || plain.Load != LoadAlways || !plain.Enabled || !plain.AllProjects() || plain.Body != "Use pnpm." {
		t.Fatalf("plain = %+v, %v", plain, err)
	}
	for _, bad := range []string{"---\ntitle: x\n", "---\nprojects: {a: 1}\n---\nx", "---\ntitle: [\n---\n"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
	if e, err := Parse([]byte("---\n---\nbody")); err != nil || e.Body != "body" {
		t.Errorf("empty front matter = %+v, %v", e, err)
	}
}

func TestStore(t *testing.T) {
	s := New(t.TempDir())
	if ts, errs := s.List(); len(ts) != 0 || len(errs) != 0 {
		t.Fatalf("empty = %v %v", ts, errs)
	}
	sec, err := s.Save(Topic{Title: "Security", Load: LoadAlways, Enabled: true, Body: "No secrets in logs."}, "")
	if err != nil || sec.Slug != "security" || sec.Order != 10 {
		t.Fatalf("save = %+v, %v", sec, err)
	}
	dup, err := s.Save(Topic{Title: "security", Enabled: true}, "")
	if err != nil || dup.Slug != "security-2" || dup.Order != 20 {
		t.Fatalf("second = %+v, %v", dup, err)
	}
	fi, err := os.Stat(s.Path("security"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file: %v %v", fi, err)
	}
	// Renaming keeps the slug.
	sec.Title = "Security & secrets"
	if r, err := s.Save(sec, "security"); err != nil || r.Slug != "security" {
		t.Fatalf("rename = %+v, %v", r, err)
	}
	if _, err := s.Save(Topic{Title: "x"}, "missing"); err == nil {
		t.Fatal("saved over a missing topic")
	}
	if err := s.Reorder([]string{"security-2"}); err != nil {
		t.Fatal(err)
	}
	ts, _ := s.List()
	if len(ts) != 2 || ts[0].Slug != "security-2" || ts[1].Title != "Security & secrets" {
		t.Fatalf("after reorder = %+v", ts)
	}
	if err := s.Delete("security-2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("security-2"); err == nil {
		t.Fatal("deleted twice")
	}
	// A broken file is reported, the others still listed.
	if err := os.WriteFile(filepath.Join(s.Dir(), "bad.md"), []byte("---\ntitle: [\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ts, errs := s.List()
	if len(ts) != 1 || len(errs) != 1 {
		t.Fatalf("with a bad file: %d topics, errs %v", len(ts), errs)
	}
}

func TestStoreRejects(t *testing.T) {
	s := New(t.TempDir())
	for _, tp := range []Topic{
		{Title: " "},
		{Title: "a\nb"},
		{Title: "x", Load: "sometimes"},
		{Title: "x", Body: strings.Repeat("x", MaxBytes+1)},
		{Title: "x", Body: "\xff"},
	} {
		if _, err := s.Save(tp, ""); err == nil {
			t.Errorf("Save(%+v) accepted", tp.Title)
		}
	}
	for _, slug := range []string{"", "../x", "A", "a/b"} {
		if _, err := s.Get(slug); err == nil {
			t.Errorf("Get(%q) accepted", slug)
		}
	}
}

func TestCompose(t *testing.T) {
	ts := []Topic{
		{Slug: "general", Title: "General", Load: LoadAlways, Enabled: true, Body: "Answer in Spanish."},
		{Slug: "git", Title: "Git", Description: "Branches, commits, PRs", Load: LoadOnDemand, Enabled: true, Body: "No force-push."},
		{Slug: "web-only", Title: "Web", Load: LoadAlways, Enabled: true, Projects: []string{"web"}, Body: "Use pnpm."},
		{Slug: "off", Title: "Off", Load: LoadAlways, Enabled: false, Body: "hidden"},
		{Slug: "empty", Title: "Empty", Load: LoadAlways, Enabled: true},
	}
	if got := Compose(nil, "web", "web"); got != "" {
		t.Fatalf("none: %q", got)
	}
	web := Compose(ts, "web", "web")
	for _, want := range []string{`of project "web"`, "## General\n\nAnswer in Spanish.", "## Web\n\nUse pnpm.",
		"agenterm instructions show <topic>", "- git: Branches, commits, PRs"} {
		if !strings.Contains(web, want) {
			t.Errorf("web lacks %q:\n%s", want, web)
		}
	}
	for _, not := range []string{"No force-push.", "hidden", "## Empty"} {
		if strings.Contains(web, not) {
			t.Errorf("web has %q", not)
		}
	}
	api := Compose(ts, "api", "api")
	if strings.Contains(api, "pnpm") || !strings.Contains(api, "Answer in Spanish.") {
		t.Fatalf("api:\n%s", api)
	}
	// Past MaxTotal, always topics become "read them" entries.
	big := []Topic{
		{Slug: "a", Title: "A", Load: LoadAlways, Enabled: true, Body: strings.Repeat("a", MaxBytes)},
		{Slug: "b", Title: "B", Load: LoadAlways, Enabled: true, Body: strings.Repeat("b", MaxBytes)},
		{Slug: "c", Title: "C", Load: LoadAlways, Enabled: true, Body: "ccc"},
		{Slug: "d", Title: "D", Load: LoadAlways, Enabled: true, Body: strings.Repeat("d", 10)},
	}
	got := Compose(big, "x", "")
	if !strings.Contains(got, "- c (too long") || !strings.Contains(got, "- d (too long") || strings.Contains(got, "ccc") {
		t.Fatalf("over the total: %d bytes, tail %q", len(got), got[len(got)-120:])
	}
}

func TestFor(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Save(Topic{Title: "Data", Load: LoadOnDemand, Enabled: true, Description: "Migrations", Body: "Back up first."}, ""); err != nil {
		t.Fatal(err)
	}
	if got := s.For("p", "proj"); !strings.Contains(got, "- data: Migrations") {
		t.Fatalf("For = %q", got)
	}
}
