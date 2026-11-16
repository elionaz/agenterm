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
	// MergeNothing: the branch has no commits the base lacks.
	// Data: {branch, base}.
	MergeNothing = "merge_nothing"
	// MergeConflict: the merge conflicted and was aborted.
	// Data: {branch, base, files}.
	MergeConflict = "merge_conflict"
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
