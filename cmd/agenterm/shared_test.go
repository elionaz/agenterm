package main

import (
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/protocol"
)

func TestFolderSharers(t *testing.T) {
	ss := []protocol.SessionInfo{
		{ID: "new", ProjectID: "p1", ProfileID: "codex", State: "running"},
		{ID: "a", ProjectID: "p1", ProfileID: "claude", Name: "api", State: "idle"},
		{ID: "b", ProjectID: "p1", ProfileID: "claude", State: "running", Mode: protocol.ModeWorktree, WorktreeID: "b"},
		{ID: "c", ProjectID: "p1", ProfileID: "shell", State: "running", WorktreeID: "x"},
		{ID: "d", ProjectID: "p1", ProfileID: "claude", State: "exited"},
		{ID: "e", ProjectID: "p1", ProfileID: "claude", State: "suspended"},
		{ID: "f", ProjectID: "p2", ProfileID: "claude", State: "running"},
		{ID: "g", ProjectID: "p1", ProfileID: "shell", State: "running"},
	}
	got := folderSharers(ss, "p1", "new")
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("folderSharers = %+v, want only a", got)
	}
	if n := sharedNote(got); !strings.Contains(n, "api (a)") || !strings.Contains(n, "--worktree") {
		t.Errorf("sharedNote = %q", n)
	}
	if n := sharedNote(folderSharers(ss, "p2", "f")); n != "" {
		t.Errorf("alone in the folder: sharedNote = %q, want empty", n)
	}
}
