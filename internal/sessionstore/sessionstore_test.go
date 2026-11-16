package sessionstore

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, backup, err := Open(path)
	if err != nil || backup != "" || len(s.List()) != 0 {
		t.Fatalf("open missing: %v %q %v", s.List(), backup, err)
	}
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := Record{ID: "a1b2", ProjectID: "p1", ProfileID: "claude", Mode: "worktree", WorktreeID: "a1b2", Cwd: "/w/a1b2",
		Cmd: "claude", Args: []string{"--settings", "/h/claude-settings.json"}, Pid: 42, Cols: 100, Rows: 30,
		CreatedAt: created, State: "running"}
	b := Record{ID: "c3d4", ProjectID: "p1", ProfileID: "shell", Cwd: "/p", Cmd: "/bin/zsh", CreatedAt: created, State: "idle"}
	if err := s.Put(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(b); err != nil {
		t.Fatal(err)
	}
	if err := s.Update("a1b2", func(r *Record) bool {
		r.Resume = &Resume{Agent: "claude", SessionID: "uuid-1", TranscriptPath: "/t.jsonl", At: created}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Record{ID: "../x"}); err == nil {
		t.Fatal("bad id accepted")
	}

	s2, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.List()
	a.Resume = &Resume{Agent: "claude", SessionID: "uuid-1", TranscriptPath: "/t.jsonl", At: created}
	if !reflect.DeepEqual(got, []Record{a, b}) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got, []Record{a, b})
	}
	// No resolved environment is ever stored.
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "env") {
		t.Fatalf("file mentions env: %s", data)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	// Returned records are copies.
	got[0].Args[0] = "changed"
	if r, _ := s2.Get("a1b2"); r.Args[0] != "--settings" {
		t.Fatal("List shares memory with the store")
	}
	if err := s2.Delete("a1b2", "nope"); err != nil {
		t.Fatal(err)
	}
	s3, _, _ := Open(path)
	if l := s3.List(); len(l) != 1 || l[0].ID != "c3d4" {
		t.Fatalf("after delete: %+v", l)
	}
}

func TestCorruptRegistryIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	os.WriteFile(path, []byte(`[{"id":"a1",`), 0o600)
	s, backup, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" || len(s.List()) != 0 {
		t.Fatalf("backup %q, records %+v", backup, s.List())
	}
	if b, _ := os.ReadFile(backup); string(b) != `[{"id":"a1",` {
		t.Fatalf("backup content %q", b)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("corrupt file left in place")
	}
	// The store works normally afterwards.
	if err := s.Put(Record{ID: "b2", State: "running"}); err != nil {
		t.Fatal(err)
	}
}

func TestScrollbackSaveLoadCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scrollback")
	data := bytes.Repeat([]byte("0123456789"), 100) // 1000 bytes
	if err := SaveScrollback(dir, "s1", data, 256); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(ScrollbackFile(dir, "s1"))
	if err != nil || fi.Size() != 256 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("saved: %v %v", fi, err)
	}
	got, err := LoadScrollback(dir, "s1", 0)
	if err != nil || !bytes.Equal(got, data[len(data)-256:]) {
		t.Fatalf("load: %q %v", got, err)
	}
	// Loading caps too (a file written with a bigger ring).
	if err := SaveScrollback(dir, "s2", data, 0); err != nil {
		t.Fatal(err)
	}
	got, _ = LoadScrollback(dir, "s2", 100)
	if !bytes.Equal(got, data[900:]) {
		t.Fatalf("capped load: %q", got)
	}
	if got, err := LoadScrollback(dir, "missing", 0); err != nil || got != nil {
		t.Fatalf("missing: %q %v", got, err)
	}
	if _, err := LoadScrollback(dir, "../etc", 0); err == nil {
		t.Fatal("path traversal accepted")
	}
	// Atomic replace: no temp files left behind.
	os.WriteFile(filepath.Join(dir, ".s1.log.tmp123"), nil, 0o600)
	PruneScrollback(dir, map[string]bool{"s1": true})
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 || ents[0].Name() != "s1.log" {
		t.Fatalf("after prune: %v", ents)
	}
	if err := RemoveScrollback(dir, "s1"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveScrollback(dir, "s1"); err != nil {
		t.Fatal("second remove:", err)
	}
}
