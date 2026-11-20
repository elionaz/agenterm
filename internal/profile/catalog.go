package profile

import (
	"strings"

	"github.com/elionaz/panal/internal/status"
)

// CatalogEntry is a known agent CLI. Users override any field (or hide the
// entry) from profiles.yaml; see Resolve.
type CatalogEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Binaries are tried in order; the first one found is the command.
	// Ignored when Cmd is set.
	Binaries []string `json:"binaries,omitempty"`
	// Cmd is a fixed command ($VARS expanded), used instead of Binaries.
	Cmd               string      `json:"cmd,omitempty"`
	Args              []string    `json:"args,omitempty"`
	Status            status.Mode `json:"status"`
	SetupHooks        string      `json:"setupHooks,omitempty"`
	AttentionPatterns []string    `json:"attentionPatterns,omitempty"`
	// ResumeArgs are appended to Args when a session is respawned after
	// a daemon restart and the agent has no built-in resume (see
	// profile.Profile.ResumeArgs).
	ResumeArgs []string `json:"resumeArgs,omitempty"`
	Homepage   string   `json:"homepage,omitempty"`
	// Install is a one-line install hint shown when the binary is missing.
	Install string `json:"install,omitempty"`
}

// command is the entry's default command: Cmd, or the first binary that
// look finds (Binaries[0] if none is installed).
func (c CatalogEntry) command(look Lookup) string {
	if c.Cmd != "" || len(c.Binaries) == 0 {
		return c.Cmd
	}
	if look != nil {
		for _, b := range c.Binaries {
			if _, ok := look(b); ok {
				return b
			}
		}
	}
	return c.Binaries[0]
}

// catalog is ordered by how likely the agent is to be someone's default.
// Binary names marked "unverified" come from documentation, not from an
// installed copy; they are overridable from profiles.yaml (`cmd:`).
var catalog = []CatalogEntry{
	{ID: "claude", Name: "Claude Code", Binaries: []string{"claude"}, Status: status.ModeHooks, SetupHooks: "claude",
		Homepage: "https://docs.claude.com/en/docs/claude-code",
		Install:  "curl -fsSL https://claude.ai/install.sh | bash   (or npm i -g @anthropic-ai/claude-code)"},
	// codex reports turn ends via `notify` but has no per-invocation hook
	// for "working again", so the heuristic stays in charge of running.
	{ID: "codex", Name: "Codex CLI", Binaries: []string{"codex"}, Status: status.ModeHeuristic, SetupHooks: "codex",
		Homepage: "https://github.com/openai/codex",
		Install:  "npm i -g @openai/codex   (or brew install --cask codex)"},
	// opencode's plugin reports every turn's start and end (success,
	// failure, interruption), approvals and questions. `--continue`
	// reopens the project's last session when its id is unknown.
	{ID: "opencode", Name: "opencode", Binaries: []string{"opencode"}, Status: status.ModeHooks, SetupHooks: "opencode",
		ResumeArgs: []string{"--continue"},
		Homepage:   "https://opencode.ai",
		Install:    "curl -fsSL https://opencode.ai/install | bash   (or brew install sst/tap/opencode)"},
	// Gemini CLI has no hook for a turn that fails or is cancelled, so the
	// heuristic stays in charge of bringing such a turn back to idle.
	{ID: "gemini", Name: "Gemini CLI", Binaries: []string{"gemini"}, Status: status.ModeHeuristic, SetupHooks: "gemini",
		Homepage: "https://github.com/google-gemini/gemini-cli",
		Install:  "npm i -g @google/gemini-cli   (or brew install gemini-cli)"},
	// Cursor CLI runs a plugin's session and tool hooks but not its stop
	// hook (end of turn), so the heuristic decides working/idle.
	// `--continue` reopens the folder's last chat when its id is unknown.
	{ID: "cursor-agent", Name: "Cursor CLI", Binaries: []string{"cursor-agent"}, Status: status.ModeHeuristic, SetupHooks: "cursor",
		ResumeArgs: []string{"--continue"},
		// Its approval menus ("Run this command?", file edits…) end with
		// "Skip & tell the agent what to do instead (esc or n)".
		AttentionPatterns: []string{`Run this command\?`, `Skip & tell the agent what to do instead`},
		Homepage:          "https://cursor.com/cli",
		Install:           "curl https://cursor.com/install -fsS | bash"},
	{ID: "copilot", Name: "GitHub Copilot CLI", Binaries: []string{"copilot"}, Status: status.ModeHeuristic,
		Homepage: "https://github.com/github/copilot-cli",
		Install:  "npm i -g @github/copilot"},
	{ID: "aider", Name: "Aider", Binaries: []string{"aider"}, Status: status.ModeHeuristic,
		// "Add file to the chat? (Y)es/(N)o/(D)on't ask again [Yes]:"
		AttentionPatterns: []string{`\(Y\)es/\(N\)o`},
		Homepage:          "https://aider.chat",
		Install:           "python -m pip install aider-install && aider-install"},
	{ID: "amp", Name: "Amp", Binaries: []string{"amp"}, Status: status.ModeHeuristic,
		Homepage: "https://ampcode.com",
		Install:  "npm i -g @sourcegraph/amp"},
	// `goose session` starts the interactive session (plain `goose` only
	// prints help in older releases).
	{ID: "goose", Name: "goose", Binaries: []string{"goose"}, Args: []string{"session"}, Status: status.ModeHeuristic,
		Homepage: "https://block.github.io/goose",
		Install:  "brew install block-goose-cli"},
	{ID: "qwen", Name: "Qwen Code", Binaries: []string{"qwen"}, Status: status.ModeHeuristic,
		Homepage: "https://github.com/QwenLM/qwen-code",
		Install:  "npm i -g @qwen-code/qwen-code"},
	{ID: "crush", Name: "Crush", Binaries: []string{"crush"}, Status: status.ModeHeuristic,
		Homepage: "https://github.com/charmbracelet/crush",
		Install:  "brew install charmbracelet/tap/crush"},
	// Kiro CLI is the successor of the Amazon Q Developer CLI (`q chat`);
	// `q` itself is left out: the name clashes with unrelated tools.
	// Unverified binary name / args.
	{ID: "kiro", Name: "Kiro CLI", Binaries: []string{"kiro-cli"}, Args: []string{"chat"}, Status: status.ModeHeuristic,
		Homepage: "https://kiro.dev/cli",
		Install:  "curl -fsSL https://cli.kiro.dev/install | bash"},
	// Hermes Agent (Nous Research). `--cli` keeps the classic REPL even
	// when the user set display.interface: tui. `-c` on resume reattaches
	// the latest session in this directory. It only launches when `hermes`
	// is installed and the profile stays enabled; args and resume_args
	// are overridable from profiles.yaml.
	{ID: "hermes", Name: "Hermes Agent", Binaries: []string{"hermes"}, Args: []string{"--cli"}, Status: status.ModeHeuristic,
		// "      [o]nce  |  [s]ession  |  [a]lways  |  [d]eny"
		AttentionPatterns: []string{`\[o\]nce\s+\|\s+\[s\]ession`},
		ResumeArgs:        []string{"-c"},
		Homepage:          "https://hermes-agent.nousresearch.com/",
		Install:           "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash"},
	{ID: "shell", Name: "Shell", Cmd: "$SHELL", Status: status.ModeHeuristic},
}

// Catalog returns a copy of the built-in agent catalog.
func Catalog() []CatalogEntry {
	out := make([]CatalogEntry, len(catalog))
	for i, c := range catalog {
		c.Binaries = append([]string(nil), c.Binaries...)
		c.Args = append([]string(nil), c.Args...)
		c.AttentionPatterns = append([]string(nil), c.AttentionPatterns...)
		c.ResumeArgs = append([]string(nil), c.ResumeArgs...)
		out[i] = c
	}
	return out
}

func catalogIndex(id string) int {
	for i, c := range catalog {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// InstallCommand splits a catalog Install hint into the command panal
// can run ("npm i -g @openai/codex") and the alternatives shown as text
// ("brew install --cask codex"), from "cmd   (or alt)".
func InstallCommand(hint string) (cmd, alt string) {
	cmd = strings.TrimSpace(hint)
	if i := strings.Index(cmd, "(or "); i >= 0 {
		alt = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cmd[i+len("(or "):]), ")"))
		cmd = strings.TrimSpace(cmd[:i])
	}
	return cmd, alt
}

// CatalogInstall is the install command of catalog agent id ("" when it is
// not in the catalog or has none): only these are run by panal, never a
// profiles.yaml value.
func CatalogInstall(id string) (name, cmd string) {
	if i := catalogIndex(id); i >= 0 {
		c, _ := InstallCommand(catalog[i].Install)
		return catalog[i].Name, c
	}
	return "", ""
}
