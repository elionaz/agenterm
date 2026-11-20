package daemon

import (
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/sessionstore"
)

func TestRenderNotes(t *testing.T) {
	if renderNotes(nil) != "" {
		t.Fatal("no sessions must give no section")
	}
	got := renderNotes([]noteSession{
		{Agent: "Claude <Code>", Model: "claude-opus-5-5", Prompts: []string{"fix @someone's bug", "has ``` inside"}, More: 2},
		{Agent: "Codex CLI"},
	})
	for _, want := range []string{
		noteSectionHead,
		"<summary><b>Claude &lt;Code&gt;</b> · <code>claude-opus-5-5</code> · 4 prompts</summary>",
		"**1.**\n\n```text\nfix @someone's bug\n```",
		"````text\nhas ``` inside\n````",
		"… 2 more",
		"\n<b>Codex CLI</b>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section lacks %q:\n%s", want, got)
		}
	}
}

func TestRenderNotesBounded(t *testing.T) {
	var notes []noteSession
	for range 40 {
		notes = append(notes, noteSession{Agent: "A", Prompts: []string{strings.Repeat("x", notePromptMax)}})
	}
	got := renderNotes(notes)
	if len(got) > noteSectionMax+len(notes)*64 { // overflowing sessions keep a short line
		t.Fatalf("section is %d bytes", len(got))
	}
	if !strings.Contains(got, "prompts left out") {
		t.Fatal("overflowing sessions must say their prompts were left out")
	}
}

func TestSameFolder(t *testing.T) {
	wt := sessGit{Mode: protocol.ModeWorktree, WorktreeID: "w1", Dir: "/x/w1"}
	main := sessGit{Mode: protocol.ModeSame, Dir: "/repo"}
	cases := []struct {
		r    sessionstore.Record
		g    sessGit
		want bool
	}{
		{sessionstore.Record{Mode: protocol.ModeWorktree, WorktreeID: "w1", Cwd: "/x/w1"}, wt, true},
		{sessionstore.Record{Mode: protocol.ModeWorktree, WorktreeID: "w2", Cwd: "/x/w2"}, wt, false},
		{sessionstore.Record{Cwd: "/repo/"}, main, true},
		{sessionstore.Record{Mode: protocol.ModeSame, Cwd: "/other"}, main, false},
		{sessionstore.Record{Mode: protocol.ModeWorktree, WorktreeID: "w1", Cwd: "/repo"}, main, false},
	}
	for i, c := range cases {
		if got := sameFolder(c.r, c.g); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}
