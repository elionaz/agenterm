// Package apierr gives user-facing refusals a stable, machine-readable code
// and structured data, so UIs can render them in their own language. The
// error text (Error()) is the canonical English message, which clients
// show for codes they do not know. The daemon sends Code and Data in
// protocol.Response.ErrorCode / ErrorData.
package apierr

import (
	"errors"
	"fmt"
)

// Codes. Keep in sync with the frontend dictionaries (err.<code>).
const (
	// LiveSessions: daemon.shutdown without force while sessions run.
	// Data: {count, sessions: [{id, profileId}]}.
	LiveSessions = "live_sessions"
	// WorktreeDirty / WorktreeUnmerged: removing a worktree would lose
	// uncommitted files / unmerged commits. Data: gitx.LossError.
	WorktreeDirty    = "worktree_dirty"
	WorktreeUnmerged = "worktree_unmerged"
	// WorktreeLive: a running session uses the worktree. Data: {sessionId}.
	WorktreeLive = "worktree_live"
	// WorktreeNotFound: unknown worktree id. Data: {id}.
	WorktreeNotFound = "worktree_not_found"
	// MergeMainDirty: the main work tree has uncommitted changes.
	// Data: {dir, count}.
	MergeMainDirty = "merge_main_dirty"
	// MergeWrongBranch: the main work tree is not on the base branch.
	// Data: {dir, current (empty: detached HEAD), base}.
	MergeWrongBranch = "merge_wrong_branch"
	// MergeBaseBehind: the local base branch lacks commits of its
	// upstream (pull requests merged on the host, never pulled).
	// Data: {dir, base, upstream, count}.
	MergeBaseBehind = "merge_base_behind"
	// SetupUntrusted: the repository's .panal.yaml has a worktree setup
	// command the user has not trusted yet (spawn again with trustSetup or
	// skipSetup). Data: {repo, file, command}.
	SetupUntrusted = "setup_untrusted"
	// InstructionsNotFound: no instructions topic with that slug.
	// Data: {slug}.
	InstructionsNotFound = "instructions_not_found"
	// MsgUnsentInput: the session's prompt has text typed but not sent:
	// the "new messages" line would be appended to it and sent with it.
	MsgUnsentInput = "msg_unsent_input"
	// MergeNothing: the branch has no commits the base lacks.
	// Data: {branch, base}.
	MergeNothing = "merge_nothing"
	// MergeConflict: the merge conflicted and was aborted.
	// Data: {branch, base, files}.
	MergeConflict = "merge_conflict"
	// PRNothing: the branch has no commits the base lacks (nothing to
	// open a pull request for). Data: {branch, base}.
	PRNothing = "pr_nothing"
	// PRNoRemote: no remote to push the branch to (none, or several
	// without upstream/origin). Data: {branch, remotes}.
	PRNoRemote = "pr_no_remote"
	// PushFailed: git push failed. Data: {remote, branch, stderr}.
	PushFailed = "push_failed"
	// BranchMissing: the worktree's branch no longer exists. Data: {branch}.
	BranchMissing = "branch_missing"
	// BranchExists: a new worktree's branch is taken. Data: {branch}.
	BranchExists = "branch_exists"
	// BranchInvalid: not a valid branch name. Data: {branch}.
	BranchInvalid = "branch_invalid"
	// ProfileNotInstalled: the agent's command is not on PATH.
	// Data: {profileId, cmd, install}.
	ProfileNotInstalled = "profile_not_installed"
	// NotARepo: the folder is not a git repository. Data: {dir, project}.
	NotARepo = "not_a_repo"
	// GitMissing: git is not installed (protocol.ProjectGit.ErrorCode).
	GitMissing = "git_missing"
	// NoCommits: the repository has no commits yet. Data: {dir}.
	NoCommits = "no_commits"
	// BaseMissing: the base branch/commit does not exist (or none could
	// be guessed: base empty). Data: {base}.
	BaseMissing = "base_missing"
	// DirMissing: a session's or worktree's folder is gone. Data: {dir}.
	DirMissing = "dir_missing"
	// ResumeFailed: a suspended session cannot be resumed (reason:
	// project_missing | still_running | not_suspended | busy). Missing
	// folders and agents use DirMissing / ProfileNotInstalled.
	// Data: {sessionId, reason, pid?}.
	ResumeFailed = "resume_failed"
	// ForgeUnsupported: the remote is not on GitHub or GitLab. Data: {host}.
	ForgeUnsupported = "forge_unsupported"
	// ForgeCLIMissing: gh / glab is not installed. Data: {tool, host, install}.
	ForgeCLIMissing = "forge_cli_missing"
	// ForgeNotAuthed: gh / glab is not logged in to the host. Data: {tool, host}.
	ForgeNotAuthed = "forge_not_authed"
	// ForgeOnBase: the folder is on its base branch (or a detached HEAD):
	// there is no pull request to show. Data: {branch}.
	ForgeOnBase = "forge_on_base"
	// ForgeHeadMismatch: the pull request to merge is not the branch the
	// user saw (the folder moved to another branch, or it is a fork's
	// branch of the same name). Data: {number, branch, head}.
	ForgeHeadMismatch = "forge_head_mismatch"
	// ForgeFailed: gh / glab failed. Data: {tool, stderr}.
	ForgeFailed = "forge_failed"
	// ProjectNameEmpty / ProjectNameTaken: renaming a project to nothing,
	// or to another project's name. Data (taken): {name}.
	ProjectNameEmpty = "project_name_empty"
	ProjectNameTaken = "project_name_taken"
	// ProjectNameInvalid: too long or with control characters. Data: {max}.
	ProjectNameInvalid = "project_name_invalid"
	// ProjectColorInvalid (data: {color}) / ProjectIconInvalid: not a badge
	// color; an icon that is not one emoji or a couple of characters.
	ProjectColorInvalid = "project_color_invalid"
	ProjectIconInvalid  = "project_icon_invalid"
	// UnknownMethod: the daemon does not know the method, usually because
	// it is older than the client. Data: {method}.
	UnknownMethod = "unknown_method"

	// Messages between sessions (msg.*).
	// MsgDisabled: messages are off (settings messages.enabled).
	MsgDisabled = "msg_disabled"
	// MsgNoSession: no session of the project matches. Data: {to}.
	MsgNoSession = "msg_no_session"
	// MsgAmbiguous: several sessions of the project have that name.
	// Data: {to, ids}.
	MsgAmbiguous = "msg_ambiguous"
	// MsgOtherProject: the recipient is in another project. Data: {to}.
	MsgOtherProject = "msg_other_project"
	// MsgNotInSession: the call must come from inside a session (msg.take).
	MsgNotInSession = "msg_not_in_session"
	// MsgFromSession: only the user may do it, not a session (msg.deliver).
	MsgFromSession = "msg_from_session"
)

// Coded is implemented by errors that carry a code.
type Coded interface {
	error
	ErrorCode() string
	ErrorData() any
}

// Error is a coded error. Err, if set, is wrapped (errors.Is/As).
type Error struct {
	Code string
	Msg  string
	Data any
	Err  error
}

func (e *Error) Error() string     { return e.Msg }
func (e *Error) Unwrap() error     { return e.Err }
func (e *Error) ErrorCode() string { return e.Code }
func (e *Error) ErrorData() any    { return e.Data }

// New returns a coded error; format follows fmt.Errorf (%w wraps).
func New(code string, data any, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)
	return &Error{Code: code, Msg: err.Error(), Data: data, Err: errors.Unwrap(err)}
}

// Code returns err's code and data, if it (or an error it wraps) has one.
func Code(err error) (code string, data any, ok bool) {
	var c Coded
	if errors.As(err, &c) && c.ErrorCode() != "" {
		return c.ErrorCode(), c.ErrorData(), true
	}
	return "", nil, false
}
