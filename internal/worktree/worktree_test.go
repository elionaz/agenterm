package worktree

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worktrees.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "a1", ProjectID: "p", Branch: "agent/x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "b2", ProjectID: "q"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "a1", ProjectID: "p", Branch: "agent/y"}); err != nil { // replace
		t.Fatal(err)
	}
	re, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := re.List(""); len(got) != 2 {
		t.Fatalf("reloaded %+v", got)
	}
	if r, err := re.Get("a1"); err != nil || r.Branch != "agent/y" {
		t.Fatalf("Get = %+v, %v", r, err)
	}
	if got := re.List("q"); len(got) != 1 || got[0].ID != "b2" {
		t.Fatalf("List(q) = %+v", got)
	}
	if err := re.Delete("a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := re.Get("a1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted Get = %v", err)
	}
}

func TestProjectSlug(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"Mi Repo", "ab12"}:  "mi-repo-ab12",
		{"ñ/../x", "cd34"}:   "x-cd34",
		{"", "ef56"}:         "ef56",
		{"agenterm", "0000"}: "agenterm-0000",
	} {
		if got := ProjectSlug(in[0], in[1]); got != want {
			t.Errorf("ProjectSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddBranch(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "worktrees.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "w", ProjectID: "p", Branch: "agent/x"}); err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"agent/x", "fix/a", "fix/a", "feat/b", "fix/a", " "} {
		if _, err := s.AddBranch("w", b); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := s.Get("w")
	if got := strings.Join(r.Branches, ","); got != "feat/b,fix/a" {
		t.Fatalf("Branches = %q", got)
	}
	if got := strings.Join(r.AllBranches(), ","); got != "fix/a,feat/b,agent/x" {
		t.Fatalf("AllBranches = %q", got)
	}
	if !r.HasBranch("agent/x") || !r.HasBranch("feat/b") || r.HasBranch("main") || r.HasBranch("") {
		t.Fatal("HasBranch")
	}
	// Persisted, and bounded.
	for i := 0; i < MaxBranches+5; i++ {
		s.AddBranch("w", "b"+strconv.Itoa(i))
	}
	s2, _ := Open(s.path)
	r, _ = s2.Get("w")
	if len(r.Branches) != MaxBranches || r.Branches[len(r.Branches)-1] != "b"+strconv.Itoa(MaxBranches+4) {
		t.Fatalf("reloaded %d branches, last %q", len(r.Branches), r.Branches[len(r.Branches)-1])
	}
	if _, err := s.AddBranch("nope", "x"); err == nil {
		t.Fatal("unknown id")
	}
}
