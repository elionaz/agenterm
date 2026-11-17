package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryLifecycle(t *testing.T) {
	cfg := t.TempDir()
	dir := t.TempDir()
	file := filepath.Join(cfg, "sub", "projects.json")

	r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.List()) != 0 {
		t.Fatal("expected empty registry")
	}
	p, err := r.Add(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != filepath.Base(dir) || p.Path != dir || p.ID == "" || p.AddedAt.IsZero() {
		t.Fatalf("bad project %+v", p)
	}
	again, err := r.Add(dir+"/.", "other")
	if err != nil || again.ID != p.ID {
		t.Fatalf("re-add should dedupe: %+v %v", again, err)
	}
	if _, err := r.Add(filepath.Join(dir, "missing"), ""); err == nil {
		t.Fatal("expected error for missing dir")
	}
	f := filepath.Join(dir, "file")
	os.WriteFile(f, nil, 0o600)
	if _, err := r.Add(f, ""); err == nil {
		t.Fatal("expected error for file")
	}

	// Persisted and reloadable.
	r2, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r2.Get(p.ID)
	if err != nil || got != p {
		t.Fatalf("reload: %+v %v", got, err)
	}
	if byName, err := r2.Get(p.Name); err != nil || byName.ID != p.ID {
		t.Fatalf("get by name: %+v %v", byName, err)
	}

	if err := r2.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if err := r2.Remove(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	r3, _ := Open(file)
	if len(r3.List()) != 0 {
		t.Fatal("remove not persisted")
	}
}

func TestOpenCorrupt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	os.WriteFile(file, []byte("{nope"), 0o600)
	if _, err := Open(file); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRegistryReorder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	r, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, n := range []string{"a", "b", "c"} {
		p, err := r.Add(t.TempDir(), n)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	got, err := r.Reorder([]string{ids[2]})
	if err != nil || got[0].Name != "c" || got[1].Name != "a" || got[2].Name != "b" {
		t.Fatalf("reorder: %v %v", got, err)
	}
	r2, _ := Open(file)
	if l := r2.List(); l[0].Name != "c" || l[2].Name != "b" {
		t.Fatalf("not persisted: %v", l)
	}
	if _, err := r.Reorder([]string{"nope"}); err == nil {
		t.Fatal("unknown id must fail")
	}
}
