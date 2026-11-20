package main

import (
	"fmt"
	"strings"

	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/status"
)

// shellProfile is the plain shell: a terminal next to the agents, not one
// of them (mirrors SHELL_PROFILE in the app).
const shellProfile = "shell"

// folderSharers returns the other live agents working in a project's own
// folder (not in a worktree): they share its branch and its changes. Plain
// shells don't count.
func folderSharers(ss []protocol.SessionInfo, projectID, except string) []protocol.SessionInfo {
	var out []protocol.SessionInfo
	for _, s := range ss {
		if s.ProjectID != projectID || s.ID == except || s.ProfileID == shellProfile ||
			s.Mode == protocol.ModeWorktree || s.WorktreeID != "" {
			continue
		}
		if s.State == string(status.Exited) || s.State == string(status.Suspended) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// sharedNote is the warning spawn prints when the new session shares the
// project folder with other live sessions ("" when it doesn't).
func sharedNote(others []protocol.SessionInfo) string {
	if len(others) == 0 {
		return ""
	}
	names := make([]string, len(others))
	for i, s := range others {
		names[i] = fmt.Sprintf("%s (%s)", displayName(s), s.ID)
	}
	return fmt.Sprintf("note: this session shares the project folder with %s: same branch, same changes. Use --worktree to give an agent its own.",
		strings.Join(names, ", "))
}
