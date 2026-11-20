package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/redact"
	"github.com/elionaz/panal/internal/session"
	"github.com/elionaz/panal/internal/status"
)

// Continue a session's work with another agent ("Continue with…"), or with
// the same one in a worktree of its own (a session sharing the project
// folder): handoffNote drafts a summary of what the session did, the user
// edits it, handoff starts the new session with it where the old one
// worked (or in a new worktree from its current commit) and closes the old
// one. Claude Code and codex get the summary as their first prompt; other
// agents get a line typed at their prompt pointing to it as a file.

const (
	handoffPrompts     = 10   // the user's last prompts quoted
	handoffPromptMax   = 1500 // characters per prompt
	handoffFiles       = 30   // uncommitted paths listed
	handoffCommits     = 20   // commits listed
	handoffTailLines   = 60   // terminal lines quoted
	handoffNoteMax     = 32 << 10
	handoffReadyWait   = 60 * time.Second
	handoffReadyPeriod = 250 * time.Millisecond
)

func (s *Server) handoffNote(id string) (protocol.HandoffNote, error) {
	sess, err := s.lookup(id)
	if err != nil {
		return protocol.HandoffNote{}, err
	}
	info := s.sessionInfo(sess)
	ctx, cancel := context.WithTimeout(s.bg, 10*time.Second)
	defer cancel()
	var b strings.Builder
	agent := s.profileName(info.ProfileID)
	fmt.Fprintf(&b, "# Handoff: continue the work of a previous session\n\n")
	fmt.Fprintf(&b, "Another agent (%s, panal session %s", agent, info.ID)
	if info.Name != "" {
		fmt.Fprintf(&b, " %q", info.Name)
	}
	fmt.Fprintf(&b, ") worked on this in project %q. Pick up where it left off: check the current state of the files "+
		"first, don't redo what is done, and ask the user when the next step is unclear.\n", info.ProjectName)

	if rec, ok := s.store.Get(id); ok && rec.Resume != nil && rec.Resume.Agent == hooks.AgentClaude {
		ps := hooks.Prompts(rec.Resume.TranscriptPath)
		if len(ps) > 0 {
			b.WriteString("\n## What the user asked\n")
			from := max(0, len(ps)-handoffPrompts)
			if from > 0 {
				fmt.Fprintf(&b, "\n(the last %d of %d prompts)\n", len(ps)-from, len(ps))
			}
			for i, p := range ps[from:] {
				fmt.Fprintf(&b, "\n%d. %s\n", from+i+1, indentRest(redact.String(truncRunes(strings.TrimSpace(p), handoffPromptMax))))
			}
		}
	}

	g := s.sessionGit(id)
	if g != nil && g.Dir != "" && gitx.IsRepo(ctx, g.Dir) {
		branch, _ := gitx.CurrentBranch(ctx, g.Dir)
		b.WriteString("\n## Where it worked\n\n")
		fmt.Fprintf(&b, "Folder %s", g.Dir)
		if branch != "" {
			fmt.Fprintf(&b, ", branch %s", branch)
		}
		if g.Mode == protocol.ModeWorktree {
			b.WriteString(" (a panal worktree)")
		}
		b.WriteString(".\n")
		if st, err := gitx.GetStatus(ctx, g.Dir); err == nil && st.Dirty() {
			fmt.Fprintf(&b, "\nUncommitted changes (%d):\n", st.Changed())
			for i, f := range st.Files {
				if i == handoffFiles {
					fmt.Fprintf(&b, "- … %d more\n", st.Changed()-i)
					break
				}
				fmt.Fprintf(&b, "- %s\n", f)
			}
		}
		if base := s.handoffBase(ctx, *g, branch); base != "" {
			if cs, err := gitx.CommitsAhead(ctx, g.Dir, base, "HEAD"); err == nil && len(cs) > 0 {
				fmt.Fprintf(&b, "\nCommits on the branch since %s (%d, newest first):\n", gitx.ShortRef(base), len(cs))
				for i, c := range cs {
					if i == handoffCommits {
						fmt.Fprintf(&b, "- … %d more\n", len(cs)-i)
						break
					}
					fmt.Fprintf(&b, "- %s %s\n", c.Short, c.Subject)
				}
			}
		}
	}

	if tail := cleanTail(sess.Tail(handoffTailLines)); tail != "" {
		fence := strings.Repeat("`", max(3, longestRun(tail, '`')+1))
		fmt.Fprintf(&b, "\n## The end of its terminal\n\n%stext\n%s\n%s\n", fence, redact.String(tail), fence)
	}
	text := b.String()
	if len(text) > handoffNoteMax {
		text = truncRunes(text, handoffNoteMax)
	}
	shared := false
	if g == nil || g.Mode != protocol.ModeWorktree {
		shared = len(s.folderSharers(sess)) > 0
	}
	return protocol.HandoffNote{Text: text, Shared: shared}, nil
}

// handoffBase is what the session's commits are counted from: its
// worktree's base, else the project's default branch ("" when the session
// is on it).
func (s *Server) handoffBase(ctx context.Context, g sessGit, branch string) string {
	if g.Mode == protocol.ModeWorktree {
		ref, _, _, err := compareBase(ctx, g)
		if err != nil {
			return ""
		}
		return ref
	}
	base, err := gitx.DefaultBase(ctx, g.Dir)
	if err != nil || base == "" || base == branch {
		return ""
	}
	return gitx.FreshestBase(ctx, g.Dir, base, "HEAD")
}

// folderSharers are the other live agents in sess's folder (same project,
// no worktree; shells don't count).
func (s *Server) folderSharers(sess *session.Session) []string {
	var out []string
	for _, o := range s.liveSessions() {
		if o.ID == sess.ID || o.ProjectID != sess.ProjectID || o.ProfileID == "shell" {
			continue
		}
		if g := s.sessionGit(o.ID); g != nil && g.Mode == protocol.ModeWorktree {
			continue
		}
		out = append(out, o.ID)
	}
	return out
}

func (s *Server) profileName(id string) string {
	ps, _ := s.profiles.List()
	if p, ok := profile.Find(ps, id); ok && p.Name != "" {
		return p.Name
	}
	return id
}

// handoff starts p.ProfileID with p.Note where p.SessionID worked (or in a
// new worktree from its current commit) and, unless p.Keep, closes it.
func (s *Server) handoff(p protocol.HandoffParams) (protocol.SessionInfo, error) {
	old, err := s.lookup(p.SessionID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	note := strings.TrimSpace(p.Note)
	if note == "" {
		return protocol.SessionInfo{}, fmt.Errorf("the handoff note is empty")
	}
	if len(note) > 2*handoffNoteMax {
		return protocol.SessionInfo{}, fmt.Errorf("the handoff note is %d bytes, the limit is %d", len(note), 2*handoffNoteMax)
	}
	if p.TargetID != "" {
		return s.handoffInto(old, p.TargetID, note, p.Keep)
	}
	lp, err := s.resolveLaunch(p.ProfileID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	sp := protocol.SpawnParams{ProjectID: old.ProjectID, ProfileID: p.ProfileID, Cols: p.Cols, Rows: p.Rows}
	if p.Cols == 0 || p.Rows == 0 {
		sp.Cols, sp.Rows = old.Size()
	}
	g := s.sessionGit(old.ID)
	switch p.Where {
	case "", protocol.HandoffSame:
		if g != nil && g.Mode == protocol.ModeWorktree && g.WorktreeID != "" {
			sp.WorktreeID = g.WorktreeID
		}
	case protocol.HandoffWorktree:
		sp.Mode = protocol.ModeWorktree
		if g != nil && g.Dir != "" {
			ctx, cancel := context.WithTimeout(s.bg, 5*time.Second)
			if head, err := gitx.ResolveCommit(ctx, g.Dir, "HEAD"); err == nil {
				sp.Base = head // its commits come along; uncommitted changes stay
			}
			cancel()
		}
	default:
		return protocol.SessionInfo{}, fmt.Errorf("where must be %s or %s", protocol.HandoffSame, protocol.HandoffWorktree)
	}

	// Claude Code and codex take a first prompt as their last argument;
	// the others are told where the note is once they wait for input.
	var extra spawnExtra
	prompted := hooks.TakesPrompt(lp.prof.SetupHooks)
	if prompted {
		extra.args = []string{note}
	}
	info, err := s.spawnWith(sp, extra)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	if name := s.sessionName(old.ID); name != "" {
		if r, err := s.rename(protocol.RenameParams{SessionID: info.ID, Name: name}); err == nil {
			info = r
		}
	}
	s.log.Info("session handed off", "from", old.ID, "to", info.ID, "profile", p.ProfileID, "where", cmpOr(p.Where, protocol.HandoffSame))
	if !prompted {
		file := s.handoffFile(info.ID)
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err == nil && os.WriteFile(file, []byte(note+"\n"), 0o600) == nil {
			if sess, err := s.lookup(info.ID); err == nil {
				go s.typeHandoff(sess, file)
			}
		}
	}
	s.setContinued(info.ID, old.ID)
	info.ContinuedFrom = append(info.ContinuedFrom, old.ID)
	s.closeHandedOff(old, p.Keep)
	return info, nil
}

func (s *Server) closeHandedOff(old *session.Session, keep bool) {
	if keep {
		return
	}
	if _, exited := old.ExitCode(); !exited {
		s.log.Info("session closed after a handoff", "id", old.ID)
		s.markStopped(old.ID)
		old.Kill(s.cfg.KillGrace)
	}
}

// handoffInto hands old's work to the live session targetID of the same
// project: the note goes to a file, and a message from the user tells the
// session to read it (messages off: the line is typed at its prompt).
func (s *Server) handoffInto(old *session.Session, targetID, note string, keep bool) (protocol.SessionInfo, error) {
	target, err := s.lookup(targetID)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	switch {
	case target.ID == old.ID:
		return protocol.SessionInfo{}, fmt.Errorf("a session can't continue its own work")
	case target.ProjectID != old.ProjectID:
		return protocol.SessionInfo{}, fmt.Errorf("session %s is in another project", target.ID)
	case target.Suspended():
		return protocol.SessionInfo{}, fmt.Errorf("session %s is suspended: resume it first", target.ID)
	}
	if _, exited := target.ExitCode(); exited {
		return protocol.SessionInfo{}, fmt.Errorf("session %s has exited", target.ID)
	}
	file := s.handoffFile(target.ID + "-from-" + old.ID)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return protocol.SessionInfo{}, err
	}
	if err := os.WriteFile(file, []byte(note+"\n"), 0o600); err != nil {
		return protocol.SessionInfo{}, err
	}
	who := old.ID
	if n := s.sessionName(old.ID); n != "" {
		who = fmt.Sprintf("%s (%q)", old.ID, n)
	}
	body := fmt.Sprintf("Take over the work of session %s (%s), which is now closed: read the handoff note in %s "+
		"(what was asked, where it worked, its changes and commits, the end of its terminal), check the current state, "+
		"and continue it alongside what you are doing.", who, s.profileName(old.ProfileID), file)
	if _, err := s.msgSend(nil, protocol.MsgSendParams{To: target.ID, Body: body, ProjectID: target.ProjectID}); err != nil {
		if code, _, _ := apierr.Code(err); code != apierr.MsgDisabled {
			return protocol.SessionInfo{}, err
		}
		go s.typeHandoff(target, file)
	}
	s.setContinued(target.ID, old.ID)
	s.log.Info("session handed off", "from", old.ID, "into", target.ID)
	s.closeHandedOff(old, keep)
	return s.sessionInfo(target), nil
}

// handoffFile is where the note for session id is (agents without a first
// prompt read it); it goes when the session is removed.
func (s *Server) handoffFile(id string) string {
	return filepath.Join(s.cfg.Paths.RuntimeDir, "handoffs", id+".md")
}

func (s *Server) forgetHandoff(id string) {
	if !validSessionID.MatchString(id) {
		return
	}
	_ = os.Remove(s.handoffFile(id))
	more, _ := filepath.Glob(filepath.Join(filepath.Dir(s.handoffFile(id)), id+"-from-*.md"))
	for _, f := range more {
		_ = os.Remove(f)
	}
}

// typeHandoff types, once sess first waits for input, a line telling its
// agent to read the note at file.
func (s *Server) typeHandoff(sess *session.Session, file string) {
	deadline := time.Now().Add(handoffReadyWait)
	for time.Now().Before(deadline) {
		if _, exited := sess.ExitCode(); exited {
			return
		}
		if st := sess.State(); st == status.Idle || st == status.WaitingInput {
			break
		}
		time.Sleep(handoffReadyPeriod)
	}
	line := "Read the handoff note in " + file + " and continue that work."
	if _, err := sess.Write([]byte(line)); err != nil {
		return
	}
	time.Sleep(nudgeGap)
	_, _ = sess.Write([]byte("\r"))
}

// cleanTail joins terminal lines, dropping trailing blank ones.
func cleanTail(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

// indentRest indents every line after the first (a numbered list item).
func indentRest(s string) string {
	return strings.ReplaceAll(s, "\n", "\n   ")
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
