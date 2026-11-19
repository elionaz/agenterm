package daemon

import (
	"errors"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/hooks"
	"github.com/elionaz/agenterm/internal/instructions"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
)

// The user's standing instructions (package instructions, one file per
// topic): edited from the app's Settings, handed to Claude Code sessions in
// their SessionStart hook (msg.take with Intro) and to codex sessions as
// launch arguments (agentArgs); "on demand" topics are read by the agent
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

// sessionInstructions is what a session of proj receives ("" for none).
func (s *Server) sessionInstructions(proj project.Project) string {
	return s.instr().For(proj.ID, proj.Name)
}

// agentArgs are the arguments agenterm adds before a profile's own: the
// hook integration, plus the instructions for codex (unless the user's
// config.toml sets its own developer_instructions).
func (s *Server) agentArgs(lp launchProfile, proj project.Project) []string {
	args := append([]string{}, lp.inj.Args...)
	if lp.prof.SetupHooks == hooks.AgentCodex && !hooks.CodexOwnInstructions() {
		args = append(args, hooks.CodexInstructionArgs(s.sessionInstructions(proj))...)
	}
	return args
}
