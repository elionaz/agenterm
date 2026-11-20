package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameTarget(t *testing.T) {
	none := func(string) bool { return false }
	for _, c := range []struct {
		bundle string
		exists func(string) bool
		want   string
	}{
		{"/Applications/agenterm.app", none, "/Applications/panal.app"},
		{"/Users/me/Applications/agenterm.app", none, "/Users/me/Applications/panal.app"},
		{"/Applications/panal.app", none, ""},
		{"/Applications/My agenterm.app", none, ""},
		{"/Applications/agenterm.app", func(p string) bool { return p == "/Applications/panal.app" }, ""},
		{"/Volumes/panal 0.13/agenterm.app", none, ""},
		{"/private/var/folders/x/AppTranslocation/ABC/d/agenterm.app", none, ""},
		{"", none, ""},
	} {
		if got := renameTarget(c.bundle, c.exists); got != c.want {
			t.Errorf("renameTarget(%q) = %q, want %q", c.bundle, got, c.want)
		}
	}
}

func TestRelink(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "Applications", "agenterm.app")
	neu := filepath.Join(root, "Applications", "panal.app")
	mustLink := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	mustLink(old+"/Contents/Helpers/agenterm", "agenterm")
	mustLink(old+"/Contents/Helpers/panal", "panal")
	mustLink("/usr/bin/git", "git")
	mustLink(old+"-other/x", "near-miss")
	got := relink([]string{bin, filepath.Join(root, "missing")}, old, neu)
	if len(got) != 2 {
		t.Fatalf("relinked %v", got)
	}
	for name, want := range map[string]string{
		"agenterm":  neu + "/Contents/Helpers/agenterm",
		"panal":     neu + "/Contents/Helpers/panal",
		"git":       "/usr/bin/git",
		"near-miss": old + "-other/x",
	} {
		if l, _ := os.Readlink(filepath.Join(bin, name)); l != want {
			t.Errorf("%s -> %s, want %s", name, l, want)
		}
	}
}

func TestAddPanalLinks(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Applications", "panal.app")
	helpers := filepath.Join(bundle, "Contents", "Helpers")
	if err := os.MkdirAll(helpers, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"panal", "agenterm"} {
		if err := os.WriteFile(filepath.Join(helpers, f), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dir := func(name string) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	brew, user, taken, other := dir("brew"), dir("user"), dir("taken"), dir("other")
	link := func(target, d string) {
		if err := os.Symlink(target, filepath.Join(d, "agenterm")); err != nil {
			t.Fatal(err)
		}
	}
	link(helpers+"/agenterm", brew)
	link(helpers+"/agenterm", user)
	link(helpers+"/agenterm", taken)
	os.WriteFile(filepath.Join(taken, "panal"), []byte("someone else's"), 0o755)
	link("/elsewhere/agenterm", other)

	got := addPanalLinks([]string{brew, user, taken, other, filepath.Join(root, "missing")}, bundle)
	if len(got) != 2 {
		t.Fatalf("linked %v", got)
	}
	for _, d := range []string{brew, user} {
		if l, _ := os.Readlink(filepath.Join(d, "panal")); l != helpers+"/panal" {
			t.Errorf("%s/panal -> %q", d, l)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(taken, "panal")); string(b) != "someone else's" {
		t.Error("an existing panal was replaced")
	}
	if pathExists(filepath.Join(other, "panal")) {
		t.Error("linked next to an agenterm of another install")
	}
	// Run again: nothing new.
	if again := addPanalLinks([]string{brew, user}, bundle); len(again) != 0 {
		t.Fatalf("second run linked %v", again)
	}
	// No helper in the bundle (a dev build): nothing.
	if n := addPanalLinks([]string{brew}, filepath.Join(root, "dev.app")); n != nil {
		t.Fatalf("dev build linked %v", n)
	}
}
