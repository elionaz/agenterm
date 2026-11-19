package daemon

import (
	"path/filepath"
	"strings"
)

// Permission modes a session was launched in, read from its command line
// (permissionMode): what the user should know before leaving an agent
// alone. "" is the agent's default (it asks).
const (
	PermBypass   = "bypass"    // runs commands and edits without asking
	PermAutoEdit = "auto-edit" // edits files without asking, asks for commands
	PermPlan     = "plan"      // read-only planning
)

// permRank orders the modes by how much the agent may do unasked.
var permRank = map[string]int{"": 0, PermPlan: 1, PermAutoEdit: 2, PermBypass: 3}

// permissionMode reads the permission flags of the agents that have them
// from cmd (a path or name) and args; for flags given twice the stronger
// mode wins. Unknown agents report "".
func permissionMode(cmd string, args []string) string {
	bin := filepath.Base(cmd)
	mode := ""
	raise := func(m string) {
		if permRank[m] > permRank[mode] {
			mode = m
		}
	}
	// value returns the flag's value (--flag=v or --flag v).
	value := func(i int, names ...string) (string, bool) {
		a := args[i]
		for _, n := range names {
			if a == n && i+1 < len(args) {
				return args[i+1], true
			}
			if v, ok := strings.CutPrefix(a, n+"="); ok && strings.HasPrefix(n, "--") {
				return v, true
			}
		}
		return "", false
	}
	for i, a := range args {
		switch bin {
		case "claude":
			switch {
			case a == "--dangerously-skip-permissions":
				raise(PermBypass)
			default:
				if v, ok := value(i, "--permission-mode"); ok {
					switch v {
					case "bypassPermissions":
						raise(PermBypass)
					case "acceptEdits":
						raise(PermAutoEdit)
					case "plan":
						raise(PermPlan)
					}
				}
			}
		case "codex":
			switch {
			case a == "--dangerously-bypass-approvals-and-sandbox", a == "--yolo":
				raise(PermBypass)
			case a == "--full-auto":
				raise(PermAutoEdit)
			default:
				if v, ok := value(i, "-a", "--ask-for-approval"); ok && v == "never" {
					raise(PermBypass)
				}
				if v, ok := value(i, "-s", "--sandbox"); ok {
					switch v {
					case "danger-full-access":
						raise(PermBypass)
					case "read-only":
						raise(PermPlan)
					}
				}
			}
		case "gemini", "qwen":
			switch {
			case a == "-y", a == "--yolo":
				raise(PermBypass)
			default:
				if v, ok := value(i, "--approval-mode"); ok {
					switch v {
					case "yolo":
						raise(PermBypass)
					case "auto_edit", "autoEdit":
						raise(PermAutoEdit)
					case "plan":
						raise(PermPlan)
					}
				}
			}
		case "cursor-agent":
			if a == "-f" || a == "--force" {
				raise(PermBypass)
			}
		case "copilot":
			if a == "--allow-all-tools" || a == "--allow-all" || a == "--yolo" {
				raise(PermBypass)
			}
		case "amp":
			if a == "--dangerously-allow-all" {
				raise(PermBypass)
			}
		case "crush":
			if a == "-y" || a == "--yolo" {
				raise(PermBypass)
			}
		case "kiro-cli":
			if a == "--trust-all-tools" {
				raise(PermBypass)
			}
		case "aider":
			if a == "--yes-always" {
				raise(PermBypass)
			}
		}
	}
	return mode
}
