package hooks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The user's standing instructions (package instructions) reach each agent
// through the integration it has: Claude Code in its SessionStart hook
// output (notify --from-hook), codex as developer_instructions for the
// invocation. Other agents don't get them.

// ReceivesInstructions reports whether an agent with setup_hooks agent
// gets the instructions.
func ReceivesInstructions(agent string) bool {
	return agent == AgentClaude || agent == AgentCodex
}

// CodexInstructionArgs are the codex arguments that hand it text as
// developer instructions (nil for an empty text). A JSON string is a valid
// TOML basic string, which is what `-c key=value` parses.
func CodexInstructionArgs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(text)
	return []string{"-c", "developer_instructions=" + strings.TrimSuffix(b.String(), "\n")}
}

var codexOwnKey = regexp.MustCompile(`^\s*developer_instructions\s*=`)

// CodexOwnInstructions reports whether the user's codex configuration
// ($CODEX_HOME/config.toml, else ~/.codex/config.toml) sets
// developer_instructions at its top level: `-c` would replace that text,
// so panal leaves it alone. Only the lines before the first [table]
// header are top level.
func CodexOwnInstructions() bool {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		dir = filepath.Join(home, ".codex")
	}
	return codexConfigSets(filepath.Join(dir, "config.toml"))
}

func codexConfigSets(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(l, "[") {
			return false
		}
		if codexOwnKey.MatchString(l) {
			return true
		}
	}
	return false
}
