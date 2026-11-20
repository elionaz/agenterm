package daemon

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"

	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/redact"
	"github.com/elionaz/panal/internal/sessionstore"
)

// The "Agent sessions" section suggested for a new pull request: which
// agents worked on the branch (and with what model) and, for Claude
// Code, the prompts typed in each session with secrets removed. The app
// shows it next to the body; the user decides whether it goes in.

// Bounds of the section: GitHub refuses bodies over 65536 characters and
// the commits' list comes first.
const (
	notePromptMax   = 2000  // characters per prompt
	notePromptsMax  = 30    // prompts per session
	noteSectionMax  = 30000 // characters in all
	noteSectionHead = "### Agent sessions"
)

// noteSession is one session that worked on the branch.
type noteSession struct {
	Agent   string   // the profile's name
	Model   string   // "" when unknown
	Prompts []string // oldest first, already redacted
	More    int      // prompts left out after notePromptsMax
}

// prSessionNotes is the section for the sessions of t's folder: those in
// its worktree, or in the main folder for a branch checked out there.
// Shells are left out; "" when no agent worked there.
func (s *Server) prSessionNotes(t forgeTarget) string {
	ps, _ := s.profiles.List()
	var notes []noteSession
	for _, r := range s.store.List() {
		if !sameFolder(r, t.g) {
			continue
		}
		if r.ProfileID == "shell" || shells[filepath.Base(r.Cmd)] {
			continue
		}
		p, ok := profile.Find(ps, r.ProfileID)
		n := noteSession{Agent: r.ProfileID, Model: r.Model}
		if ok && p.Name != "" {
			n.Agent = p.Name
		}
		if r.Resume != nil && r.Resume.Agent == hooks.AgentClaude {
			all := hooks.Prompts(r.Resume.TranscriptPath)
			if len(all) > notePromptsMax {
				n.More = len(all) - notePromptsMax
				all = all[:notePromptsMax]
			}
			for _, pr := range all {
				n.Prompts = append(n.Prompts, redact.String(truncRunes(pr, notePromptMax)))
			}
		}
		notes = append(notes, n)
	}
	return renderNotes(notes)
}

func sameFolder(r sessionstore.Record, g sessGit) bool {
	if g.Mode == protocol.ModeWorktree {
		return g.WorktreeID != "" && r.WorktreeID == g.WorktreeID
	}
	return r.Mode != protocol.ModeWorktree && r.Cwd != "" && filepath.Clean(r.Cwd) == filepath.Clean(g.Dir)
}

// renderNotes is the Markdown section ("" for no sessions). Prompts go in
// fenced blocks: nothing in them renders, links or @mentions included.
func renderNotes(notes []noteSession) string {
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(noteSectionHead + "\n\n")
	b.WriteString("Made with [panal](https://github.com/elionaz/panal). Prompts as typed, secrets removed.\n")
	for _, n := range notes {
		var sum strings.Builder
		sum.WriteString("<b>" + html.EscapeString(n.Agent) + "</b>")
		if n.Model != "" {
			sum.WriteString(" · <code>" + html.EscapeString(n.Model) + "</code>")
		}
		if len(n.Prompts) == 0 {
			b.WriteString("\n" + sum.String() + "\n")
			continue
		}
		total := len(n.Prompts) + n.More
		fmt.Fprintf(&sum, " · %d %s", total, plural(total, "prompt", "prompts"))
		var body strings.Builder
		for i, p := range n.Prompts {
			fence := strings.Repeat("`", max(3, longestRun(p, '`')+1))
			fmt.Fprintf(&body, "\n**%d.**\n\n%stext\n%s\n%s\n", i+1, fence, p, fence)
		}
		if n.More > 0 {
			fmt.Fprintf(&body, "\n… %d more\n", n.More)
		}
		entry := "\n<details><summary>" + sum.String() + "</summary>\n" + body.String() + "\n</details>\n"
		if b.Len()+len(entry) > noteSectionMax {
			// No room for its prompts: the session's line only.
			entry = "\n" + sum.String() + " (prompts left out: too long)\n"
		}
		b.WriteString(entry)
	}
	return strings.TrimRight(b.String(), "\n")
}

func longestRun(s string, c byte) int {
	best, cur := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
