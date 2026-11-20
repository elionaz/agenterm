package daemon

import (
	"errors"
	"path/filepath"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/hooks"
	"github.com/elionaz/panal/internal/instructions"
	"github.com/elionaz/panal/internal/project"
	"github.com/elionaz/panal/internal/protocol"
)

// The user's standing instructions (package instructions, one file per
// topic): edited from the app's Settings, handed to Claude Code, Gemini
// CLI and Cursor sessions in their session-start hook (msg.take with
// Intro), to codex sessions as launch arguments (agentArgs) and to
// opencode sessions as an instructions file (agentEnv); "on demand" topics are read by the agent
// with instructions.show.

func (s *Server) instr() instructions.Store { return instructions.New(s.cfg.Paths.ConfigDir) }

func (s *Server) instructionsList(projectRef string) (protocol.Instructions, error) {
	st := s.instr()
	ts, errs := st.List()
	out := protocol.Instructions{
		Topics:   make([]protocol.InstructionTopic, 0, len(ts)),
		Dir:      st.Dir(),
		CodexOwn: hooks.CodexOwnInstructions(),
		MaxBytes: instructions.MaxBytes,
		MaxTotal: instructions.MaxTotal,
	}
	for _, t := range ts {
		out.Topics = append(out.Topics, topicOut(t))
	}
	for _, e := range errs {
		out.Errors = append(out.Errors, e.Error())
	}
	if projectRef != "" {
		proj, err := s.projects.Get(projectRef)
		if err != nil {
			return out, err
		}
		out.ProjectID = proj.ID
		out.Composed = instructions.Compose(ts, proj.ID, proj.Name)
	} else {
		out.Composed = instructions.Compose(ts, "", "")
	}
	return out, nil
}

func topicOut(t instructions.Topic) protocol.InstructionTopic {
	return protocol.InstructionTopic{Slug: t.Slug, Title: t.Title, Description: t.Description, Load: t.Load,
		Projects: t.Projects, Enabled: t.Enabled, Order: t.Order, Body: t.Body}
}

func (s *Server) instructionsSave(p protocol.InstructionsSaveParams) (protocol.Instructions, error) {
	in := p.Topic
	t := instructions.Topic{Title: in.Title, Description: in.Description, Load: in.Load,
		Projects: in.Projects, Enabled: in.Enabled, Order: in.Order, Body: in.Body}
	saved, err := s.instr().Save(t, p.OldSlug)
	if err != nil {
		return protocol.Instructions{}, topicErr(err, p.OldSlug)
	}
	s.log.Info("instructions saved", "topic", saved.Slug, "load", saved.Load, "bytes", len(saved.Body))
	return s.instructionsList(p.ProjectID)
}

func (s *Server) instructionsDelete(p protocol.InstructionsSlugParams) (protocol.Instructions, error) {
	if err := s.instr().Delete(p.Slug); err != nil {
		return protocol.Instructions{}, topicErr(err, p.Slug)
	}
	s.log.Info("instructions deleted", "topic", p.Slug)
	return s.instructionsList(p.ProjectID)
}

func (s *Server) instructionsReorder(p protocol.InstructionsReorderParams) (protocol.Instructions, error) {
	if err := s.instr().Reorder(p.Slugs); err != nil {
		return protocol.Instructions{}, err
	}
	return s.instructionsList(p.ProjectID)
}

// instructionsShow is a topic's text, whether or not it is enabled or
// loads on demand (the agent asked for it by name).
func (s *Server) instructionsShow(p protocol.InstructionsSlugParams) (protocol.InstructionTopicText, error) {
	t, err := s.instr().Get(p.Slug)
	if err != nil {
		return protocol.InstructionTopicText{}, topicErr(err, p.Slug)
	}
	return protocol.InstructionTopicText{Slug: t.Slug, Title: t.Title, Body: t.Body}, nil
}

func topicErr(err error, slug string) error {
	if errors.Is(err, instructions.ErrNotFound) {
		return apierr.New(apierr.InstructionsNotFound, map[string]any{"slug": slug}, "%v", err)
	}
	return err
}

// sessionInstructions is what a session of proj receives ("" for none):
// the user's topics, then where its browser is.
func (s *Server) sessionInstructions(proj project.Project) string {
	text := s.instr().For(proj.ID, proj.Name)
	if cfg := s.browserSettings(); cfg.Enabled && s.browserExe(cfg) != "" {
		if text == "" {
			text = "From panal:"
		}
		text += "\n\n" + browserNote
	}
	return text
}

// agentEnv is the environment panal adds for a session of proj after the
// hook injection's: for opencode, the inline config again with the
// instructions file (instructionsFile) added; nil otherwise.
func (s *Server) agentEnv(lp launchProfile, proj project.Project) []string {
	if lp.prof.SetupHooks != hooks.AgentOpencode {
		return nil
	}
	file := s.instructionsFile(proj)
	if file == "" {
		return nil
	}
	if kv := hooks.OpencodeInstructionsEnv(lp.inj, s.cfg.Paths.HooksDir(), file); kv != "" {
		return []string{kv}
	}
	return nil
}

// instructionsFile writes what a session of proj receives to a file in
// the hooks dir (one per project, for agents that read instructions from
// files) and returns its path; "" when there is nothing to read or it
// could not be written.
func (s *Server) instructionsFile(proj project.Project) string {
	text := s.sessionInstructions(proj)
	if text == "" {
		return ""
	}
	file := filepath.Join(s.cfg.Paths.HooksDir(), "instructions", proj.ID+".md")
	if err := hooks.WriteIfChanged(file, []byte(text+"\n")); err != nil {
		s.log.Warn("instructions file", "err", err)
		return ""
	}
	return file
}

// agentArgs are the arguments panal adds before a profile's own: the
// hook integration, plus the instructions for codex (unless the user's
// config.toml sets its own developer_instructions).
func (s *Server) agentArgs(lp launchProfile, proj project.Project) []string {
	// The browser MCP server first: claude's --mcp-config takes several
	// values, and the --settings that follows ends them.
	args := append(s.browserMCPArgs(lp.prof.SetupHooks), lp.inj.Args...)
	if lp.prof.SetupHooks == hooks.AgentCodex && !hooks.CodexOwnInstructions() {
		args = append(args, hooks.CodexInstructionArgs(s.sessionInstructions(proj))...)
	}
	return args
}
