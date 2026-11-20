// Package protocol defines the wire format between the panal daemon and its
// clients.
//
// Control traffic is newline-delimited JSON: the client writes one Request
// per line and the daemon answers with one Response per line. A connection may
// carry any number of sequential control requests.
//
// Two methods take over the connection after their (successful) response:
//
//   - events.subscribe: the daemon streams Event values, one JSON object per line.
//   - session.attach: both sides switch to binary frames (see frame.go).
package protocol

import (
	"encoding/json"
	"time"

	"github.com/elionaz/panal/internal/forge"
	"github.com/elionaz/panal/internal/gitx"
	"github.com/elionaz/panal/internal/history"
	"github.com/elionaz/panal/internal/mailbox"
	"github.com/elionaz/panal/internal/profile"
	"github.com/elionaz/panal/internal/project"
)

// Version is the protocol version, reported by daemon.ping. Additive
// changes (new methods, new optional fields, new event types) keep it;
// anything an older peer would misread (a removed or renamed method or
// field, a changed meaning or frame layout) bumps it. Daemons before 0.4.0
// do not report it: they speak version 1.
const Version = 1

// Environment variables injected into every session.
const (
	EnvSession = "PANAL_SESSION"
	EnvSock    = "PANAL_SOCK"
)

// Method names.
const (
	MethodProjectAdd    = "project.add"
	MethodProjectList   = "project.list"
	MethodProjectRemove = "project.remove"
	// MethodProjectReorder sets the project order (the sidebar's).
	MethodProjectReorder = "project.reorder"
	// MethodProjectUpdate renames a project or changes its badge.
	MethodProjectUpdate = "project.update"
	MethodSessionSpawn  = "session.spawn"
	MethodSessionList   = "session.list"
	MethodSessionKill   = "session.kill"
	MethodSessionResize = "session.resize"
	MethodSessionNotify = "session.notify"
	MethodSessionAck    = "session.ack"
	MethodSessionAttach = "session.attach"
	MethodSessionTail   = "session.tail"
	// MethodSessionRename sets (or, with an empty name, clears) a
	// session's display name.
	MethodSessionRename = "session.rename"
	// MethodSessionReorder sets the order of one project's sessions.
	MethodSessionReorder = "session.reorder"
	// Sessions restored from a previous daemon run (state suspended).
	MethodSessionResume   = "session.resume"
	MethodSessionDiscard  = "session.discard"
	MethodProfileList     = "profile.list"
	MethodProfileCatalog  = "profile.catalog"
	MethodProfileSave     = "profile.save"
	MethodProfileDelete   = "profile.delete"
	MethodProfileReorder  = "profile.reorder"
	MethodProfileRescan   = "profile.rescan"
	MethodProfileInstall  = "profile.install"
	MethodSettingsGet     = "settings.get"
	MethodSettingsSave    = "settings.save"
	MethodEventsSubscribe = "events.subscribe"
	MethodDaemonPing      = "daemon.ping"
	MethodDaemonShutdown  = "daemon.shutdown"

	// Git worktrees and diffs.
	MethodProjectGit     = "project.git"
	MethodSessionDiff    = "session.diff"
	MethodWorktreeDiff   = "worktree.diff"
	MethodWorktreeList   = "worktree.list"
	MethodWorktreeRemove = "worktree.remove"
	// MethodWorktreeRemoveMerged removes a project's orphaned worktrees
	// whose branch is already merged (and that have nothing uncommitted).
	MethodWorktreeRemoveMerged = "worktree.removeMerged"
	MethodWorktreeMerge        = "worktree.merge"
	MethodWorktreePR           = "worktree.pr"
	MethodWorktreeOpen         = "worktree.open"
	// Past sessions (internal/history).
	MethodHistorySearch = "history.search"
	MethodHistoryGet    = "history.get"
	MethodHistoryDelete = "history.delete"
	MethodHistoryResume = "history.resume"
	// MethodWorktreeCopy copies files from the main folder into a worktree
	// (and can write them into a new .panal.yaml).
	MethodWorktreeCopy = "worktree.copy"

	// Pull requests on the code host (GitHub through gh, GitLab through
	// glab), for a session's or worktree's branch.
	MethodForgeStatus   = "forge.status"
	MethodForgeList     = "forge.list"
	MethodForgeComments = "forge.comments"
	MethodForgeMerge    = "forge.merge"
	MethodForgeCreate   = "forge.create"

	// Messages between the sessions of a project (panal msg). The
	// daemon tells the sender from the connection: a call from inside a
	// session is from that session whatever its params say.
	MethodMsgPeers   = "msg.peers"
	MethodMsgSend    = "msg.send"
	MethodMsgList    = "msg.list"
	MethodMsgTake    = "msg.take"
	MethodMsgDeliver = "msg.deliver"
	MethodMsgAck     = "msg.ack"
	// MethodMsgRelease delivers every held message of a project (manual
	// mode): only the user can.
	MethodMsgRelease = "msg.release"

	// The user's standing instructions for the agents (topic files),
	// handed to sessions when they start.
	MethodInstructionsList    = "instructions.list"
	MethodInstructionsSave    = "instructions.save"
	MethodInstructionsDelete  = "instructions.delete"
	MethodInstructionsReorder = "instructions.reorder"
	MethodInstructionsShow    = "instructions.show"

	// Continue a session's work with another agent (or the same one in a
	// worktree): handoffNote drafts the summary, handoff starts the new
	// session with it and closes the old one.
	MethodSessionHandoffNote = "session.handoffNote"
	MethodSessionHandoff     = "session.handoff"

	// The isolated Chrome of each session (package browser).
	MethodBrowserStatus  = "browser.status"
	MethodBrowserPrepare = "browser.prepare"
	MethodBrowserOpen    = "browser.open"
	MethodBrowserClose   = "browser.close"
)

// Session modes (SpawnParams.Mode, SessionInfo.Mode).
const (
	ModeSame     = "same"
	ModeWorktree = "worktree"
)

// Request is a single control call.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers the Request with the same ID. Exactly one of Result or
// Error is meaningful.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	// Error is the (English) error message. User-facing refusals also
	// carry a stable ErrorCode (see package apierr) and structured
	// ErrorData, so UIs can render them in their language; clients fall
	// back to Error for codes they do not know.
	Error     string          `json:"error,omitempty"`
	ErrorCode string          `json:"errorCode,omitempty"`
	ErrorData json.RawMessage `json:"errorData,omitempty"`
}

// Params and results.

type ProjectAddParams struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type ProjectRef struct {
	ID string `json:"id"`
}

type ProjectList struct {
	Projects []project.Project `json:"projects"`
}

// ProfileList is the result of profile.list/save/delete/reorder/rescan:
// every effective profile (hidden ones included), in menu order.
type ProfileList struct {
	Profiles []profile.Profile `json:"profiles"`
}

// CatalogInfo is a catalog entry plus detection.
type CatalogInfo struct {
	profile.CatalogEntry
	// Command is the default command (the first binary found).
	Command   string `json:"command"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type ProfileCatalog struct {
	Entries []CatalogInfo `json:"entries"`
}

// ProfileInstallParams installs catalog agent ProfileID with its catalog
// install command, run in a new shell session of project ProjectID (its
// output and any password prompt in view); a successful install rescans.
type ProfileInstallParams struct {
	ProfileID string `json:"profileId"`
	ProjectID string `json:"projectId"`
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
}

// ProfileSaveParams upserts a profiles.yaml entry. Entry holds the desired
// values; fields equal to the inherited ones are not written.
type ProfileSaveParams struct {
	Entry profile.Entry `json:"entry"`
}

type ProfileRef struct {
	ID string `json:"id"`
}

// ProfileReorderParams moves IDs to the front, in order.
type ProfileReorderParams struct {
	IDs []string `json:"ids"`
}

// ProjectUpdateParams changes a project's name, badge color or badge icon;
// a nil field is left as is, an empty Color or Icon goes back to automatic.
type ProjectUpdateParams struct {
	ID    string  `json:"id"`
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
	Icon  *string `json:"icon,omitempty"`
	// Messages: auto | manual | default (see project.Project.Messages).
	Messages *string `json:"messages,omitempty"`
}

// ProjectReorderParams moves project IDs to the front, in order.
type ProjectReorderParams struct {
	IDs []string `json:"ids"`
}

// SessionReorderParams moves session IDs (all of ProjectID) to the front
// of that project's sessions, in order. Sessions of other projects keep
// their positions.
type SessionReorderParams struct {
	ProjectID string   `json:"projectId"`
	IDs       []string `json:"ids"`
}

type SpawnParams struct {
	// ProjectID accepts a project id or name.
	ProjectID string `json:"projectId"`
	ProfileID string `json:"profileId"`
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
	// Mode is same (the project folder; default) or worktree (a new git
	// worktree on a new branch, under the panal runtime dir).
	Mode string `json:"mode,omitempty"`
	// Branch for a new worktree (default agent/<profile>-<id>; sanitized
	// and made unique). Base is the branch/commit it starts from (default:
	// the project's current branch).
	Branch string `json:"branch,omitempty"`
	Base   string `json:"base,omitempty"`
	// WorktreeID reopens an existing panal worktree (implies worktree
	// mode; Branch and Base are ignored).
	WorktreeID string `json:"worktreeId,omitempty"`
	// TrustSetup trusts the repository's worktree setup command (as shown
	// by the setup_untrusted error) and runs it; SkipSetup creates the
	// worktree without running it. Both only matter for a new worktree.
	TrustSetup bool `json:"trustSetup,omitempty"`
	SkipSetup  bool `json:"skipSetup,omitempty"`
}

// GitInfo is a session's (or worktree's) change summary, refreshed lazily
// (on idle/done/exit and on demand, throttled).
type GitInfo struct {
	// Files changed against the base (worktree: merge base with the base
	// branch, committed + uncommitted; same folder: HEAD), untracked
	// included.
	Files int `json:"files"`
	// Uncommitted paths (git status).
	Uncommitted int `json:"uncommitted"`
	// Ahead: commits on the branch that the base does not have.
	Ahead int `json:"ahead"`
	// Head: the branch checked out (empty: detached HEAD).
	Head string `json:"head,omitempty"`
	// Merged (worktrees): the branch has commits of its own and the base
	// already holds them, merged or squash-merged. With no uncommitted
	// changes the worktree can go without losing anything.
	Merged bool      `json:"merged,omitempty"`
	At     time.Time `json:"at"`
}

// ProjectGitParams / ProjectGit: what the spawn menu needs to offer the
// worktree mode.
type ProjectGitParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectGit struct {
	IsRepo        bool     `json:"isRepo"`
	Root          string   `json:"root,omitempty"`
	CurrentBranch string   `json:"currentBranch,omitempty"`
	DefaultBase   string   `json:"defaultBase,omitempty"`
	Branches      []string `json:"branches"`
	// Error explains why worktrees are unavailable (not a repo, no
	// commits); ErrorCode is its apierr code (git_missing | not_a_repo |
	// no_commits).
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// DiffParams selects a session's or a worktree's diff. Stat skips the
// patch text.
type DiffParams struct {
	SessionID  string `json:"sessionId,omitempty"`
	WorktreeID string `json:"worktreeId,omitempty"`
	Stat       bool   `json:"stat,omitempty"`
}

// DiffResult is session.diff / worktree.diff's result.
type DiffResult struct {
	// From is what the work tree is compared with (merge base / HEAD).
	From      string          `json:"from"`
	Files     []gitx.DiffFile `json:"files"`
	Additions int             `json:"additions"`
	Deletions int             `json:"deletions"`
	// Truncated: the patch text hit the 2 MB cap (see DiffFile.Truncated).
	Truncated  bool   `json:"truncated,omitempty"`
	PatchSize  int    `json:"patchSize"`
	Mode       string `json:"mode"`
	Dir        string `json:"dir"`
	WorktreeID string `json:"worktreeId,omitempty"`
	// Branch is the branch the folder is on now; StartBranch the one the
	// worktree was created on, and Branches every branch it worked on
	// (newest first), when they differ.
	Branch      string        `json:"branch,omitempty"`
	StartBranch string        `json:"startBranch,omitempty"`
	Branches    []string      `json:"branches,omitempty"`
	Base        string        `json:"base,omitempty"`
	Commits     []gitx.Commit `json:"commits"`
	Git         GitInfo       `json:"git"`
}

// Patch concatenates every file's patch.
func (d DiffResult) Patch() string {
	return gitx.DiffResult{Files: d.Files}.Patch()
}

type WorktreeListParams struct {
	ProjectID string `json:"projectId"`
}

// WorktreeInfo describes a panal worktree.
type WorktreeInfo struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Base      string    `json:"base"`
	ProfileID string    `json:"profileId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// SessionIDs are the daemon's sessions in it (live or exited).
	SessionIDs []string `json:"sessionIds"`
	// Live: a running session uses it. Orphan: no session at all (it
	// ended and was removed, or the daemon restarted).
	Live   bool     `json:"live"`
	Orphan bool     `json:"orphan"`
	Git    *GitInfo `json:"git,omitempty"`
}

type WorktreeList struct {
	Worktrees []WorktreeInfo `json:"worktrees"`
}

// WorktreeRemoveParams: without Force the daemon refuses when uncommitted
// changes (or, with DeleteBranch, unmerged commits) would be lost. DryRun
// only reports that (Loss) without removing anything.
type WorktreeRemoveParams struct {
	ID           string `json:"id"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"deleteBranch,omitempty"`
	DryRun       bool   `json:"dryRun,omitempty"`
}

type WorktreeRemoveResult struct {
	Removed bool            `json:"removed"`
	Loss    *gitx.LossError `json:"loss,omitempty"`
}

// HistorySearchParams: every word of Query must appear in the session
// (empty: the most recent ones); ProjectID (id or name) filters.
type HistorySearchParams struct {
	Query     string `json:"query"`
	ProjectID string `json:"projectId,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type HistorySearchResult struct {
	Hits []history.Hit `json:"hits"`
}

type HistoryRef struct {
	ID string `json:"id"`
}

// HistoryDetail is an archived session with its terminal text; CanResume
// tells whether history.resume can reopen its agent conversation.
type HistoryDetail struct {
	Entry     history.Entry `json:"entry"`
	Text      string        `json:"text"`
	CanResume bool          `json:"canResume"`
}

// HistoryResumeParams starts a new session that reopens archived session
// ID's agent conversation in its folder.
type HistoryResumeParams struct {
	ID   string `json:"id"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// WorktreeRemoveMergedParams: DryRun only lists what would be removed.
type WorktreeRemoveMergedParams struct {
	ProjectID string `json:"projectId"`
	DryRun    bool   `json:"dryRun,omitempty"`
}

// WorktreeRemoveMergedResult lists the worktrees removed (with DryRun:
// those that would be) and the merged ones left, with why.
type WorktreeRemoveMergedResult struct {
	Removed []WorktreeInfo    `json:"removed"`
	Skipped []WorktreeSkipped `json:"skipped"`
}

type WorktreeSkipped struct {
	Worktree WorktreeInfo `json:"worktree"`
	Reason   string       `json:"reason"`
}

type WorktreeRef struct {
	ID string `json:"id"`
}

// WorktreePRParams pushes a worktree's branch and returns the page that
// opens a pull request into its base (result: gitx.PRResult). DryRun only
// reports the remote and URL that would be used.
type WorktreePRParams struct {
	ID     string `json:"id"`
	DryRun bool   `json:"dryRun,omitempty"`
}

// ForgeRef names the branch a forge.* method is about: a session's (the
// branch its folder is on) or a worktree's.
type ForgeRef struct {
	SessionID  string `json:"sessionId,omitempty"`
	WorktreeID string `json:"worktreeId,omitempty"`
	// Branch picks another branch the worktree worked on (one of its
	// Branches); empty means the branch it is on now.
	Branch string `json:"branch,omitempty"`
}

// ForgeListParams asks for the pull requests of a session's (or
// worktree's) branches; Refresh skips the cached answer.
type ForgeListParams struct {
	ForgeRef
	Refresh bool `json:"refresh,omitempty"`
}

// ForgeList is forge.list's result: the pull requests of the branches a
// session's worktree worked on, newest branch first (branches without
// one are left out).
type ForgeList struct {
	PRs []ForgeListItem `json:"prs"`
	// Unavailable: why there is no list (no gh/glab, no remote…).
	Unavailable string `json:"unavailable,omitempty"`
}

// ForgeListItem is one branch's pull request.
type ForgeListItem struct {
	Branch string    `json:"branch"`
	PR     *forge.PR `json:"pr"`
	// Current: the branch the folder is on now.
	Current bool `json:"current,omitempty"`
}

// ForgeStatus is forge.status's result (also returned by forge.merge and
// forge.create, refreshed).
type ForgeStatus struct {
	// Unavailable: why there is no integration for this branch, as an
	// apierr code (forge_unsupported, forge_cli_missing, forge_not_authed,
	// forge_on_base, pr_no_remote…); Message is its English text and
	// UnavailableData its data. The other fields may then be empty.
	Unavailable     string `json:"unavailable,omitempty"`
	Message         string `json:"message,omitempty"`
	UnavailableData any    `json:"unavailableData,omitempty"`
	Kind            string `json:"kind,omitempty"` // forge.GitHub | forge.GitLab
	Tool            string `json:"tool,omitempty"` // gh | glab
	Host            string `json:"host,omitempty"`
	Branch          string `json:"branch,omitempty"`
	Base            string `json:"base,omitempty"`
	// PR is the branch's pull request (nil: none yet).
	PR *forge.PR `json:"pr,omitempty"`
	// MergeMethods the repository allows (open PRs only), preferred first.
	MergeMethods []string `json:"mergeMethods,omitempty"`
	// For a new pull request: commits not in Base yet, and a suggested
	// title and body (from those commits).
	Ahead int    `json:"ahead"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	// Session is a suggested "Agent sessions" section for a new pull
	// request (the agents that worked on the branch, their models and
	// prompts, secrets removed); the app appends it to Body if the user
	// keeps it. Empty when there is an open pull request or no agent.
	Session string `json:"session,omitempty"`
	// After forge.merge: "merged", or "queued" when the host only
	// scheduled it (merge queue, auto-merge once checks pass).
	Result string `json:"result,omitempty"`
	// Warning: a step after the action failed (deleting the branch,
	// reading the new state); the action itself succeeded.
	Warning string `json:"warning,omitempty"`
}

// ForgeCommentsParams lists the comments of pull request Number.
type ForgeCommentsParams struct {
	ForgeRef
	Number int `json:"number"`
}

// ForgeMergeParams merges pull request Number on the host (Method: squash,
// merge or rebase; DeleteBranch removes the branch on the host only).
type ForgeMergeParams struct {
	ForgeRef
	Number       int    `json:"number"`
	Method       string `json:"method"`
	DeleteBranch bool   `json:"deleteBranch,omitempty"`
	// Head is the branch the user saw the pull request for: the merge is
	// refused (forge_head_mismatch) if the folder is on another one now.
	Head string `json:"head,omitempty"`
}

// ForgeCreateParams pushes the branch and opens its pull request.
type ForgeCreateParams struct {
	ForgeRef
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Draft bool   `json:"draft,omitempty"`
}

// WorktreeOpenParams opens a worktree (ID) or a session's folder
// (SessionID) in the editor or Finder.
type WorktreeOpenParams struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Target    string `json:"target"` // editor | finder
}

// PingResult is daemon.ping's. Version and Commit identify the daemon's
// build (internal/version); daemons before 0.2.0 send neither, so a client
// that finds an empty Version is talking to an older daemon. Protocol is
// the daemon's protocol Version (0 from daemons before 0.4.0: version 1).
type PingResult struct {
	OK       bool   `json:"ok"`
	Version  string `json:"version,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Protocol int    `json:"protocol,omitempty"`
	// Exe is the daemon's executable (what agent hooks run): when it no
	// longer exists (the app was renamed or moved), the daemon needs a
	// restart from the new place.
	Exe string `json:"exe,omitempty"`
}

// ProtocolOf is the protocol version r reports (1 for daemons before 0.4.0).
func (r PingResult) ProtocolOf() int {
	if r.Protocol == 0 {
		return 1
	}
	return r.Protocol
}

// ShutdownParams are daemon.shutdown's. Without Force the daemon refuses
// while any session is still alive (only exited ones may be discarded).
type ShutdownParams struct {
	Force bool `json:"force,omitempty"`
}

type SessionRef struct {
	SessionID string `json:"sessionId"`
}

// SuspendedRef selects suspended sessions for session.resume / discard:
// one (SessionID) or All of them.
type SuspendedRef struct {
	SessionID string `json:"sessionId,omitempty"`
	All       bool   `json:"all,omitempty"`
}

// SessionBatch is session.resume / discard's result: the sessions handled
// (resumed: their new info; discarded: their last one) and, with All, the
// ones that failed. A single-session request fails with the error instead.
type SessionBatch struct {
	Sessions []SessionInfo `json:"sessions"`
	Failed   []BatchError  `json:"failed,omitempty"`
}

// BatchError is one failure of a batch (ErrorCode as in Response).
type BatchError struct {
	SessionID string          `json:"sessionId"`
	Error     string          `json:"error"`
	ErrorCode string          `json:"errorCode,omitempty"`
	ErrorData json.RawMessage `json:"errorData,omitempty"`
}

// RenameParams is session.rename's: Name is trimmed and capped at
// MaxSessionName runes; empty resets the session to its profile's name.
type RenameParams struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

// MaxSessionName caps SessionInfo.Name (in runes).
const MaxSessionName = 80

type ResizeParams struct {
	SessionID string `json:"sessionId"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

type NotifyParams struct {
	SessionID string `json:"sessionId"`
	State     string `json:"state"`
	Kind      string `json:"kind,omitempty"`    // permission | question | done | error
	Message   string `json:"message,omitempty"` // shown in notifications
	// Activity describes what the agent is doing ("Reading main.go"). With
	// an empty State only the activity is updated. ActivityKind and
	// ActivityTarget are its structured form (see Live).
	Activity       string `json:"activity,omitempty"`
	ActivityKind   string `json:"activityKind,omitempty"`
	ActivityTarget string `json:"activityTarget,omitempty"`
	// Turn marks a new turn (prompt submitted): restarts the turn timer.
	Turn bool `json:"turn,omitempty"`
	// AgentSessionID is the agent's own session id from a hook payload
	// (claude session_id, codex thread-id), for resuming the conversation
	// after a daemon restart; Agent names the hook integration (claude |
	// codex). With an empty State and Activity only this is recorded.
	Agent          string `json:"agent,omitempty"`
	AgentSessionID string `json:"agentSessionId,omitempty"`
	TranscriptPath string `json:"transcriptPath,omitempty"`
	// Model is the agent's model (claude-opus-5-5…), from its hooks. Alone
	// (no State, Activity or AgentSessionID) only that is recorded.
	Model string `json:"model,omitempty"`
}

// TailParams asks for the last Lines lines of a session's output.
type TailParams struct {
	SessionID string `json:"sessionId"`
	Lines     int    `json:"lines,omitempty"` // default 8, max 200
}

// TailResult is the ANSI-stripped tail of the scrollback, oldest first.
type TailResult struct {
	SessionID string   `json:"sessionId"`
	Lines     []string `json:"lines"`
}

// Progress mirrors a terminal progress report (OSC 9;4). State is
// normal | error | indeterminate | paused; Value is 0-100.
type Progress struct {
	State string `json:"state"`
	Value int    `json:"value"`
}

// Live is what a session is doing right now: its activity line, timers,
// stall flag and progress. It is embedded (flattened) in SessionInfo and in
// session.activity / session.state events.
type Live struct {
	// Activity is a short human description ("Editing main.go", or the
	// last meaningful line of output); ActivitySource is hook | output.
	// Hook activity is English; ActivityKind (thinking | edit | read |
	// bash | search | web | subagent | todo | mcp | tool) and
	// ActivityTarget let UIs render it in their language. Both are empty
	// for output-derived (verbatim) activity.
	Activity       string     `json:"activity,omitempty"`
	ActivityKind   string     `json:"activityKind,omitempty"`
	ActivityTarget string     `json:"activityTarget,omitempty"`
	ActivityAt     *time.Time `json:"activityAt,omitempty"`
	ActivitySource string     `json:"activitySource,omitempty"`
	// TurnStartedAt is when the current (or last) turn started;
	// TurnEndedAt is set once it ended (idle/exited).
	TurnStartedAt *time.Time `json:"turnStartedAt,omitempty"`
	TurnEndedAt   *time.Time `json:"turnEndedAt,omitempty"`
	LastOutputAt  *time.Time `json:"lastOutputAt,omitempty"`
	LastEventAt   *time.Time `json:"lastEventAt,omitempty"`
	// Stalled: a hook-driven turn saw no output nor events for
	// activity.stall_seconds.
	Stalled   bool       `json:"stalled,omitempty"`
	StalledAt *time.Time `json:"stalledAt,omitempty"`
	Progress  *Progress  `json:"progress,omitempty"`
}

// SubscribeParams identifies the subscriber. Client "ui" marks a desktop
// app that shows its own notifications (the daemon then stays quiet).
type SubscribeParams struct {
	Client string `json:"client,omitempty"`
}

// ClientUI is SubscribeParams.Client for desktop UIs.
const ClientUI = "ui"

type AttachParams struct {
	SessionID string `json:"sessionId"`
	// Optional initial size; zero means "leave as is".
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// SessionInfo describes a session on the wire.
type SessionInfo struct {
	// SuggestCopy (only in spawn's answer, for a new worktree of a
	// repository without .panal.yaml): files at its root that git
	// ignores and the worktree therefore lacks (.env…), to offer copying.
	SuggestCopy []string `json:"suggestCopy,omitempty"`
	// Port is the session's own port (PORT…), when its repository's
	// .panal.yaml asks for ports (PANAL_PORT is always set).
	Port int `json:"port,omitempty"`
	// Permission is the permission mode its command line asks for: bypass
	// (runs everything unasked), auto-edit, plan, or "" (the agent asks).
	Permission string `json:"permission,omitempty"`
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	ProjectDir string `json:"projectDir"`
	ProfileID  string `json:"profileId"`
	// Name is the user-given display name (session.rename); empty means
	// "use the profile's name".
	Name string `json:"name,omitempty"`
	// ProjectName is the project's display name (for notifications).
	ProjectName string    `json:"projectName,omitempty"`
	Cmd         string    `json:"cmd"`
	Args        []string  `json:"args,omitempty"`
	Cwd         string    `json:"cwd"`
	Pid         int       `json:"pid"`
	CreatedAt   time.Time `json:"createdAt"`
	State       string    `json:"state"`
	ExitCode    *int      `json:"exitCode,omitempty"`
	Cols        uint16    `json:"cols"`
	Rows        uint16    `json:"rows"`
	// Attention: why the session needs the user. AttentionKind
	// (permission | question | done | error) is empty when it doesn't.
	// Unread stays true until session.ack (or the user types).
	AttentionKind    string     `json:"attentionKind,omitempty"`
	AttentionMessage string     `json:"attentionMessage,omitempty"`
	AttentionAt      *time.Time `json:"attentionAt,omitempty"`
	Unread           bool       `json:"unread,omitempty"`
	// MsgPending: messages from other sessions it has not got yet.
	MsgPending int `json:"msgPending,omitempty"`
	// MsgHold: why they wait (automatic delivery's reason: "unsent input",
	// "no turn end", "held" (manual project: the user delivers them),
	// "off"…; "" when they will arrive by themselves).
	MsgHold string `json:"msgHold,omitempty"`
	// Model: the agent's model id, when known (hooks or command line).
	Model string `json:"model,omitempty"`
	// BrowserURL: the DevTools URL of the session's own Chrome while it
	// runs (panal browser); "" when closed.
	BrowserURL string `json:"browserUrl,omitempty"`
	// Stopped: panal ended it (stop, close, continue with another
	// agent), so a non-zero exit code is not a failure.
	Stopped bool `json:"stopped,omitempty"`
	// ContinuedFrom are the sessions whose work this one continues
	// (session.handoff), oldest first.
	ContinuedFrom []string `json:"continuedFrom,omitempty"`
	// Mode is same | worktree; worktree sessions name their worktree.
	Mode         string   `json:"mode,omitempty"`
	WorktreeID   string   `json:"worktreeId,omitempty"`
	WorktreePath string   `json:"worktreePath,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Base         string   `json:"base,omitempty"`
	Git          *GitInfo `json:"git,omitempty"`
	// Suspended sessions (state suspended) come from a previous daemon run:
	// SuspendedAt is when that daemon stopped; ResumeMode is how resuming
	// would restart the agent: agent (its own session id: same
	// conversation), args (the profile's resume_args) or fresh (the plain
	// command). ResumeError(Code) is the last failed resume attempt.
	SuspendedAt     *time.Time `json:"suspendedAt,omitempty"`
	ResumeMode      string     `json:"resumeMode,omitempty"`
	ResumeError     string     `json:"resumeError,omitempty"`
	ResumeErrorCode string     `json:"resumeErrorCode,omitempty"`
	Live
}

// Resume modes (SessionInfo.ResumeMode).
const (
	ResumeAgent = "agent"
	ResumeArgs  = "args"
	ResumeFresh = "fresh"
)

type SessionList struct {
	Sessions []SessionInfo `json:"sessions"`
}

// Event types.
const (
	EventSessionSpawned = "session.spawned"
	EventSessionState   = "session.state"
	EventSessionRemoved = "session.removed"
	// EventSessionAttention: the attention changed without a state change
	// (message filled in, acknowledged).
	EventSessionAttention = "session.attention"
	// EventSessionActivity: activity line, progress, stall flag or output
	// heartbeat changed (coalesced: at most ~2 per second per session).
	EventSessionActivity = "session.activity"
	// EventSessionGit: the session's GitInfo changed (Git).
	EventSessionGit = "session.git"
	// EventSessionRenamed: the session's display name changed
	// (SessionName; empty: back to the profile's name).
	EventSessionRenamed = "session.renamed"
	// EventWorktrees: a project's worktree list changed (ProjectID):
	// created, removed, orphaned or reopened.
	EventWorktrees = "worktree.changed"
	// EventHistory: an ended session was archived, or an archived one
	// deleted (SessionID, ProjectID).
	EventHistory = "history.changed"
	// EventProjects: the project list changed (added, removed, reordered).
	EventProjects = "project.changed"
	// EventSessionsReordered: the sessions of ProjectID were reordered;
	// session.list has the new order.
	EventSessionsReordered = "session.reordered"
	// EventMessages: messages to or from SessionID (in ProjectID) changed:
	// sent or delivered. MsgPending is how many wait for SessionID.
	EventMessages = "message.changed"
	// EventMessageSent: a session (or the user) sent a message (MsgFrom →
	// MsgTo, in ProjectID): UIs show who is talking to whom. MsgHeld: it
	// waits for the user (manual mode).
	EventMessageSent = "message.sent"
	// EventProfiles: the agent list changed (a rescan found an agent that
	// was just installed…): clients reload profile.list.
	EventProfiles = "profile.changed"
	// EventSessionModel: the session's agent reported another model (Model).
	EventSessionModel = "session.model"
	// EventSessionBrowser: the session's own Chrome opened or closed
	// (BrowserURL).
	EventSessionBrowser = "session.browser"
)

// Event is pushed to events.subscribe streams.
type Event struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	State     string    `json:"state,omitempty"`
	PrevState string    `json:"prevState,omitempty"`
	Source    string    `json:"source,omitempty"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	At        time.Time `json:"at"`
	// Attention: why the session needs the user. AttentionKind
	// (permission | question | done | error) is empty when it doesn't.
	// Unread stays true until session.ack (or the user types).
	AttentionKind    string     `json:"attentionKind,omitempty"`
	AttentionMessage string     `json:"attentionMessage,omitempty"`
	AttentionAt      *time.Time `json:"attentionAt,omitempty"`
	Unread           bool       `json:"unread,omitempty"`
	// Raised: a new attention was raised by this event; UIs notify on it.
	Raised bool `json:"raised,omitempty"`
	// ProjectName, ProfileID and SessionName (the user-given name, if
	// any) label notifications.
	ProjectName string `json:"projectName,omitempty"`
	ProfileID   string `json:"profileId,omitempty"`
	SessionName string `json:"sessionName,omitempty"`
	// StallRaised (session.activity): the session just became stalled;
	// UIs may show a low-urgency notification (kind "stall"). It is not an
	// attention: it does not count as unread.
	StallRaised bool `json:"stallRaised,omitempty"`
	// Git (session.git) is the new change summary.
	Git *GitInfo `json:"git,omitempty"`
	// ProjectID (worktree.changed, message.changed).
	ProjectID string `json:"projectId,omitempty"`
	// MsgPending (message.changed): messages waiting for SessionID;
	// MsgHold why (see SessionInfo.MsgHold).
	MsgPending int    `json:"msgPending,omitempty"`
	MsgHold    string `json:"msgHold,omitempty"`
	// message.sent: sender and recipient (session ids, or "user"), their
	// names, the start of the text and whether it is held.
	MsgID       int    `json:"msgId,omitempty"`
	MsgFrom     string `json:"msgFrom,omitempty"`
	MsgFromName string `json:"msgFromName,omitempty"`
	MsgTo       string `json:"msgTo,omitempty"`
	MsgToName   string `json:"msgToName,omitempty"`
	MsgExcerpt  string `json:"msgExcerpt,omitempty"`
	MsgHeld     bool   `json:"msgHeld,omitempty"`
	// Model (session.model): the agent's model id.
	Model string `json:"model,omitempty"`
	// BrowserURL (session.browser): the session's Chrome opened ("" closed).
	BrowserURL string `json:"browserUrl,omitempty"`
	// Stopped (session.state to exited): panal ended it.
	Stopped bool `json:"stopped,omitempty"`
	// Live is filled in session.activity and session.state events.
	Live
}

// RestartMarker is the content of paths.RestartFile: the sessions that
// were live when the app replaced the daemon ("Restart now").
type RestartMarker struct {
	Resume []string `json:"resume"`
}

// MsgPeer is a session of the project, as panal msg peers shows it.
type MsgPeer struct {
	ID        string `json:"id"`
	Name      string `json:"name"` // display name (the user's, else the profile's)
	ProfileID string `json:"profileId"`
	State     string `json:"state"`
	Activity  string `json:"activity,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Branch    string `json:"branch,omitempty"`
	// SameFolder: it works in the same folder as the asking session.
	SameFolder bool `json:"sameFolder,omitempty"`
}

// MsgPeersParams: the project of SessionID (callers outside a session).
type MsgPeersParams struct {
	SessionID string `json:"sessionId,omitempty"`
}

// MsgPeers lists a project's sessions. Self is the asking session (nil
// when the call does not come from one).
type MsgPeers struct {
	ProjectID   string    `json:"projectId"`
	ProjectName string    `json:"projectName"`
	Self        *MsgPeer  `json:"self,omitempty"`
	Peers       []MsgPeer `json:"peers"`
	// Enabled: messages are on (settings messages.enabled).
	Enabled bool `json:"enabled"`
}

// MsgSendParams sends Body to To: a session id, a display name unique in
// the project, or "all" (every other session of the project). Callers
// outside a session (the app) name the project with ProjectID when To is
// not an id.
type MsgSendParams struct {
	To        string `json:"to"`
	Body      string `json:"body"`
	ProjectID string `json:"projectId,omitempty"`
}

// MsgListParams selects messages: those of SessionID (sent or received),
// else of ProjectID. A session asking sees only its own.
type MsgListParams struct {
	SessionID   string `json:"sessionId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	PendingOnly bool   `json:"pendingOnly,omitempty"`
}

// MsgList is the result of msg.send, msg.list and msg.take.
type MsgList struct {
	Messages []mailbox.Message `json:"messages"`
}

// MsgTakeParams: only a session may take its messages (they are marked
// delivered). Intro also returns its project's peers (agent hooks at
// session start).
type MsgTakeParams struct {
	Intro bool `json:"intro,omitempty"`
	// Peek: return them without marking them delivered; msg.ack does,
	// once they were handed over (agent hooks).
	Peek bool `json:"peek,omitempty"`
}

// MsgAckParams marks the calling session's messages IDs delivered.
type MsgAckParams struct {
	IDs []int `json:"ids"`
}

// MsgTakeResult is what msg.take returns.
type MsgTakeResult struct {
	Messages []mailbox.Message `json:"messages"`
	Peers    *MsgPeers         `json:"peers,omitempty"`
	Enabled  bool              `json:"enabled"`
	// Self is the asking session's id.
	Self string `json:"self"`
	// Instructions (with Intro): the user's standing instructions for the
	// session's project, composed ("" when none). Sent even when messages
	// are off.
	Instructions string `json:"instructions,omitempty"`
	// Auto: the project's messages travel by themselves (they wake an idle
	// recipient); otherwise the user delivers them. Held: messages for the
	// session that wait for the user.
	Auto bool `json:"auto"`
	Held int  `json:"held,omitempty"`
}

// InstructionsParams: ProjectID (id or name) also composes what a session
// of that project receives.
type InstructionsParams struct {
	ProjectID string `json:"projectId,omitempty"`
}

// InstructionTopic is one topic file (package instructions).
type InstructionTopic struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Load: "always" (whole text at start) or "on_demand" (the session
	// reads it with panal instructions show).
	Load string `json:"load"`
	// Projects it applies to; empty: all.
	Projects []string `json:"projects,omitempty"`
	Enabled  bool     `json:"enabled"`
	Order    int      `json:"order"`
	Body     string   `json:"body"`
}

// Instructions are the user's standing instruction topics.
type Instructions struct {
	Topics []InstructionTopic `json:"topics"`
	// Errors: topic files that could not be read.
	Errors []string `json:"errors,omitempty"`
	Dir    string   `json:"dir"`
	// ProjectID is the resolved project ("" without one).
	ProjectID string `json:"projectId,omitempty"`
	// Composed is what a session of ProjectID receives at start.
	Composed string `json:"composed"`
	// CodexOwn: codex's config.toml sets developer_instructions, which
	// panal leaves alone (its sessions don't get these then).
	CodexOwn bool `json:"codexOwn,omitempty"`
	// MaxBytes bounds each topic's text, MaxTotal what loads "always".
	MaxBytes int `json:"maxBytes"`
	MaxTotal int `json:"maxTotal"`
}

// InstructionsSaveParams saves Topic: a new one when OldSlug is "", else
// the topic OldSlug (its slug stays). ProjectID: as in InstructionsParams.
type InstructionsSaveParams struct {
	Topic     InstructionTopic `json:"topic"`
	OldSlug   string           `json:"oldSlug,omitempty"`
	ProjectID string           `json:"projectId,omitempty"`
}

// InstructionsSlugParams names a topic (delete, show).
type InstructionsSlugParams struct {
	Slug      string `json:"slug"`
	ProjectID string `json:"projectId,omitempty"`
}

// InstructionsReorderParams puts the topics in Slugs' order.
type InstructionsReorderParams struct {
	Slugs     []string `json:"slugs"`
	ProjectID string   `json:"projectId,omitempty"`
}

// InstructionTopicText is a topic's text, for an agent (instructions.show).
type InstructionTopicText struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// MsgDeliverParams: the app's "Deliver" button for SessionID.
type MsgDeliverParams struct {
	SessionID string `json:"sessionId"`
}

// MsgReleaseParams: ProjectID is an id or a name.
type MsgReleaseParams struct {
	ProjectID string `json:"projectId"`
}

// WorktreeCopyParams copies Files (relative to the repository root) from
// the main folder into worktree ID; Save also writes them as worktree.copy
// of a new .panal.yaml at the repository root.
type WorktreeCopyParams struct {
	ID    string   `json:"id"`
	Files []string `json:"files"`
	Save  bool     `json:"save,omitempty"`
}

// WorktreeCopyResult is what worktree.copy did.
type WorktreeCopyResult struct {
	Copied  []string `json:"copied"`
	Skipped []string `json:"skipped,omitempty"`
	Missing []string `json:"missing,omitempty"`
	// Created is the .panal.yaml written (Save).
	Created string `json:"created,omitempty"`
}

// BrowserParams names the session whose Chrome it is.
type BrowserParams struct {
	SessionID string `json:"sessionId"`
}

// BrowserInfo is a session's Chrome: browser.prepare reserves it (profile
// and port, nothing runs), browser.open starts it unless it runs.
type BrowserInfo struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
	Port      int    `json:"port"`
	Profile   string `json:"profile"`
	Exe       string `json:"exe"`
	Headless  bool   `json:"headless"`
	Running   bool   `json:"running"`
}

// BrowserStatus: whether sessions get a browser, and what it needs.
type BrowserStatus struct {
	// Enabled: settings browser.enabled.
	Enabled  bool `json:"enabled"`
	Headless bool `json:"headless"`
	// Exe is the browser found ("" none: install Chrome or set
	// browser.path); NPX the npx that runs the MCP server ("" none:
	// install Node.js; agents can still use panal browser).
	Exe string `json:"exe"`
	NPX string `json:"npx"`
	// MCP is the server package agents get.
	MCP string `json:"mcp"`
}

// HandoffNoteParams: the session whose work is handed over.
type HandoffNoteParams struct {
	SessionID string `json:"sessionId"`
}

// HandoffNote is the summary a new agent gets: what the user asked (Claude
// Code transcripts), where it worked, its git changes and commits, and the
// end of its terminal; secrets removed.
type HandoffNote struct {
	Text string `json:"text"`
	// Shared: other live agents work in the same folder (a new worktree
	// is the safer choice).
	Shared bool `json:"shared,omitempty"`
}

// Handoff "where" values.
const (
	HandoffSame     = "same"     // the session's folder or worktree
	HandoffWorktree = "worktree" // a new worktree from its current commit
)

// HandoffParams starts ProfileID with Note where SessionID worked (Where),
// or hands Note to the live session TargetID of the same project (ProfileID
// and Where are then ignored), and closes SessionID unless Keep.
type HandoffParams struct {
	SessionID string `json:"sessionId"`
	TargetID  string `json:"targetId,omitempty"`
	ProfileID string `json:"profileId"`
	Where     string `json:"where,omitempty"`
	Note      string `json:"note"`
	Keep      bool   `json:"keep,omitempty"`
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
}
