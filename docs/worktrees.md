# Worktrees and changes

**English** · [Español](es/worktrees.md)

Each session can work in the project folder or in its own **git worktree** (a separate branch and
folder), so several agents can work on the same repo in parallel without stepping on each other.

Everything goes through the `git` CLI (with timeouts; never libgit2). Read-only queries use
`GIT_OPTIONAL_LOCKS=0`, so they never take `index.lock` while the agent is using git in the same
folder.

## Session modes

- **Same folder** (`same`, the default): the agent works in the project folder.
- **New worktree** (`worktree`): `git worktree add -b <branch> <folder> <base>`.
  - **Branch**: `agent/<profile>-<first 4 characters of the session id>` by default. The name is
    sanitized (spaces and invalid characters → `-`, no `..`, no `.lock`…) and, if it already exists,
    gets `-2`, `-3`… appended.
  - **Base**: the project's current branch by default (or `main`/`master` if `HEAD` is detached).
    It can be any branch or commit.
  - **Folder**: `<worktrees>/<project>-<projectId>/<sessionId>/`, where `<worktrees>` is
    `~/.agenterm/worktrees` (or `git.worktree_dir`) and `<project>` is the project name, lowercased
    and sanitized. Worktrees are **never** created inside the project.
  - It's allowed even if the main folder has changes: the worktree starts from the base's commit.
  - If the project is a subfolder of the repo, the session starts in that same subfolder of the
    worktree.
  - Clear errors if the project isn't a git repo (in the app the option is disabled, with the
    reason), if the repo has no commits yet or if the base doesn't exist.
- **Reopening a worktree** (`--in-worktree <id>` in the CLI, *open a shell here* in the app): a new
  session in an existing agenterm worktree.

A worktree's id is the id of the session that created it.

Hook integrations work the same way in a worktree: Claude Code gets `--settings` with an absolute
path and nothing is written to the worktree or the project. Claude Code may ask you to trust the new
folder the first time.

## Lifecycle

- **Killing a session never deletes its worktree.** The registry (`worktrees.json`) survives daemon
  restarts.
- Worktree states (`agenterm worktrees <project>`):
  - `live`: a live session is using it;
  - `exited`: only used by sessions that have exited but are still in the list;
  - `orphan`: no session in the daemon uses it (you removed them from the list or the daemon
    restarted).
- When listing a project's worktrees, the daemon forgets the ones that no longer exist on disk (and
  runs `git worktree prune`) and **adopts** worktrees git knows about inside that project's
  worktrees folder that the registry doesn't have (for example, if `worktrees.json` was lost).

## Changes (diff)

- **Worktree**: `git diff <merge-base with the base>` (committed and uncommitted changes, in a
  single list) plus untracked, non-ignored files, and the commits ahead (`git log <base>..HEAD`, at
  most 200). If the base branch disappears, the base commit saved when the worktree was created is
  used.
- **Same folder**: `git diff HEAD` plus untracked files (against the empty tree if the repo has no
  commits).
- Per file: path, previous path (renames), status `A`/`M`/`D`/`R`/`C`/`T`/`U`, `+`/`−` lines, whether
  it's binary (no content) and its patch.
- Limits: **2 MB** of patch in total (remaining files are listed without content and the diff is
  marked `truncated`); untracked files over 1 MB, or beyond the first 300, don't include content
  either.
- Each session's **summary** (`git`: `files`, `uncommitted`, `ahead`) is recomputed lazily (on going
  idle, waiting or exited, and when the diff is requested), at most once every 3 s per session and
  never on every chunk of output. Changes are published with the `session.git` event.

## Actions

- **Merge into the base** (`agenterm worktree merge <id>`): `git merge --no-ff` of the branch in the
  repo's **main folder**. Only if:
  - the main folder is on the base branch;
  - it has no changes to tracked files (untracked files don't block it; there's no automatic
    stash);
  - the branch has commits the base doesn't have.

  On conflicts it runs `git merge --abort` and reports which files clashed. Uncommitted changes in
  the worktree aren't part of the merge (you're told how many were left out).
- **Remove worktree** (`agenterm worktree rm <id>`):
  - refuses if a live session is using it;
  - without *force* it also refuses if uncommitted changes would be lost or, when also deleting the
    branch (`--delete-branch`), commits not merged into the base; the error lists what would be
    lost;
  - `--dry-run` only runs the check;
  - after removing: `git worktree prune` and, if requested, `git branch -D` (safe: unmerged commits
    were already checked against the base, unless *force* is used).
- **Open in editor** (`git.editor`; `code` by default if it's on the `PATH`, otherwise Finder) and
  **Open in Finder** (`agenterm worktree open [--finder] <id>`).
- **Copy branch** (app only, to the clipboard).

## In the app

- A project's **+** menu: after picking the agent, choose same folder or new worktree (an editable,
  pre-filled branch and a base selector defaulting to the current branch). The last choice is
  remembered per project, and ⌘N uses it; if there's none, `git.default_mode` is used.
- Session row: a chip with the branch (without the `agent/` prefix) and `±N` changed files; the
  same on the overview cards, with `↑N` commits ahead.
- **Terminal** and **Changes (N)** tabs on sessions with git: a file list on the left (status,
  `+`/`−`), commits ahead, the diff on the right with diff2html (side by side or unified), ⟳ to
  reload, a truncated-diff notice and the action bar (open in editor, open in Finder, copy branch,
  merge into &lt;base&gt;, and remove worktree with a confirmation that shows what would be lost, a
  checkbox to also delete the branch and another to force).
- Under each project, **orphaned worktrees** (if any): view changes, open a shell here (a new
  `shell` session in that worktree) and remove.
