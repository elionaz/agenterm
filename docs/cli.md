# CLI reference

**English** · [Español](es/cli.md)

```sh
go build -o agenterm ./cmd/agenterm
agenterm help            # also -h / --help
agenterm version         # also --version: "agenterm 0.1.0 (abc1234)"; "dev" for local builds
```

A single binary acts as the CLI, the daemon (`agenterm daemon`) and the callback for agent hooks
(`agenterm notify`). The app's executable (`agenterm.app/Contents/MacOS/agenterm`) also implements
`daemon`, `notify` and `version`, so the app doesn't depend on the CLI. Release builds also ship the
CLI inside the app at `agenterm.app/Contents/Helpers/agenterm` (Homebrew links it onto the `PATH`).

General rules:

- Any command that talks to the daemon **starts it if it isn't running** (detached, with its output
  in `daemon.log`), except `stop`, `notify` and commands run inside a session that only have
  `$AGENTERM_SOCK` (see [Which daemon a command uses](#which-daemon-a-command-uses)).
- Flags can go before or after the arguments (`agenterm diff abc123 --stat`) and accept one or two
  dashes (`-force` / `--force`).
- Wherever a project is expected you can use its id or its name.
- Project and session ids are 8 hex characters. A worktree's id is the id of the session that
  created it.
- On failure, the command prints `agenterm: <error>` and exits with 1.
- CLI output and daemon error messages are in English, regardless of `ui.language` (see
  [configuration.md](configuration.md#language)).

## Daemon

### `agenterm daemon [--detach] [--idle D]`

Runs the daemon in the foreground (logging to stderr) until it gets SIGTERM/SIGINT or a shutdown
request.

| flag | |
|---|---|
| `--detach` | starts it in the background (logging to `daemon.log`) and returns once the socket responds; if it's already running, says so and exits with 0 |
| `--idle D` | idle timeout for the heuristic (a Go duration: `3s`, `1500ms`…); 3 s by default. A profile can override it with `idle_seconds` |

### `agenterm stop [--force]`

Asks the daemon to shut down and waits (up to 10 s) for the socket to disappear; prints
`daemon stopped`. It never starts a daemon.

- Without `--force` it **refuses** (exits with 1) if there are sessions that haven't finished: the
  error lists them (`<id> (<profile>)`) and suggests `--force`. `exited` sessions don't count.
- `--force` kills the live sessions and shuts the daemon down. They are not lost: the next daemon
  restores them as `suspended` (see [`resume`](#agenterm-resume-sessionid--all)). `suspended`
  sessions don't count as live either.

The check is done by the daemon itself, so it also protects against scripts and older clients. Every
request (accepted or refused) is logged in `daemon.log` along with the process that made it (see
[architecture.md](architecture.md#security-and-shutdown)).

## Projects

| command | |
|---|---|
| `agenterm project add <dir> [--name N]` | registers a folder (relative paths are resolved from your current directory). Prints `<id>  <name>  <path>`. The name defaults to the folder name. If the folder is already registered, returns the existing project |
| `agenterm project list` (or `ls`) | `ID NAME PATH ADDED` table |
| `agenterm project rm <id>` (or `remove`) | removes the project from the list (doesn't delete any files) |
| `agenterm project set <id\|name> [--name N] [--color C\|auto] [--icon E\|none]` | renames the project or changes its sidebar badge. Colors: `blue`, `teal`, `green`, `amber`, `orange`, `red`, `pink`, `purple`; `auto` picks one from the project id. `--icon` puts an emoji in place of the initials; `none` brings the initials back. Open windows update at once. Prints `<id>  <name>  <color>  <icon>` |

## Profiles (agents)

| command | |
|---|---|
| `agenterm profiles` (or `profiles list` / `ls`) | effective profiles (catalog + `profiles.yaml`), in menu order |
| `agenterm profiles scan` | re-reads the login shell's `PATH`, detects installed agents again and lists them |

Columns: `ID NAME ENABLED INSTALLED SOURCE STATUS CMD`. `INSTALLED` is the executable's path, `-` if
it wasn't found, or `error: …` if the `profiles.yaml` entry is invalid. `SOURCE` is `catalog`,
`override` (a catalog agent with your changes) or `custom`. See
[configuration.md](configuration.md).

## Sessions

### `agenterm spawn [flags] <project> <profile>`

Launches a session and prints its id. In worktree mode it prints `<id>  <branch>  <folder>`.

| flag | |
|---|---|
| `--cols C`, `--rows R` | initial size (defaults to your terminal's, or 80×24) |
| `--worktree` | creates a new git worktree with its own branch |
| `--branch B` | branch for `--worktree` (defaults to `agent/<profile>-<first 4 of the id>`) |
| `--base X` | base branch or commit for `--worktree` (defaults to the project's current branch) |
| `--in-worktree ID` | launches the session in an existing agenterm worktree (e.g. an orphan) |

`--branch` and `--base` require `--worktree`. Fails if the profile doesn't exist, has an error or its
command isn't installed (the message includes the catalog's install hint). See
[worktrees.md](worktrees.md).

### `agenterm ls`

Table with `ID NAME PROJECT PROFILE STATE ATTENTION PID SIZE AGE TURN GIT ACTIVITY CWD`:

- `NAME`: the name given with `rename` (or in the app), `-` if none.
- `STATE`: `running`, `waiting_input`, `idle`, `exited(<code>)` or `suspended[<mode>]` (restored
  after a daemon restart; `<mode>` is how `resume` would restart it: `agent`, `args` or `fresh`,
  followed by `!<errorCode>` if the last resume failed).
- `ATTENTION`: `permission`, `question`, `done`, `error` or `-`; with `*` if not seen yet.
- `TURN`: timer for the current turn (`4m12s`) or duration of the last one (`=6m0s`).
- `GIT`: branch, `±N` changed files and `↑N` commits ahead of the base.
- `ACTIVITY`: `[hook|output] <activity line>`, with `(state N%)` if there's progress and a
  `STALLED` prefix if the turn looks stalled.

### `agenterm tail [-n N] <sessionId>`

The last `N` lines (8 by default, 200 max) of the output as plain text, without ANSI sequences.

### `agenterm attach <sessionId>`

Connects your terminal to the session: it first repaints the scrollback and then follows live.
Window size changes are forwarded. **Ctrl-\\** detaches; the session keeps running. On a
`suspended` session it shows the saved scrollback and waits; if the session is resumed meanwhile it
ends with `[daemon: session resumed; reattach]` (run `attach` again).

### `agenterm kill <sessionId>`

Terminates the session (SIGHUP + SIGTERM to the process group, SIGKILL after 2 s) and prints
`<id> <state>`. On a session that has already exited, it **removes it from the list**; on a
`suspended` one it discards it. It never deletes the session's worktree. A killed session is not
restored after a daemon restart.

### `agenterm rename <sessionId> <name>`

Gives the session a display name (shown in the app's sidebar, pane headers, overview and
notifications, which read `<project> · <name>`) and prints `<id> <name>`. The remaining words are
joined with spaces (`agenterm rename ab12 api server`); use `--` before a name that starts with
`-`. `--reset` (or renaming to an empty name in the app) goes back to the profile's name. Names are
trimmed and capped at 80 characters, and survive daemon restarts and resumes (RPC `session.rename`).

### `agenterm resume <sessionId>|--all`

Respawns `suspended` sessions (restored after a daemon restart) in the same folder or worktree,
with their saved scrollback followed by a `── session restored · <time> ──` line. Claude Code and
codex continue the same conversation (`--resume <session_id>` / `codex resume <thread-id>`), other
profiles use their `resume_args` if they have any, and a shell comes back with nothing running.
Prints `<id> <state> pid <pid>`; with `--all`, failures go to stderr (`<id>: <error>`) and the exit
status is 1. See [architecture.md](architecture.md#persistence-and-resume).

### `agenterm discard <sessionId>|--all`

Forgets `suspended` sessions: they leave the list and their record and saved scrollback are deleted
(their worktree, if any, stays as an orphan).

### `agenterm ack <sessionId>`

Marks the session's attention as seen (same as selecting it in the app).

## Git

| command | |
|---|---|
| `agenterm diff [--stat] <sessionId\|worktreeId>` | unified diff of the session (worktree: against its base; same folder: against `HEAD`). With `--stat`, a per-file summary, commits ahead and totals |
| `agenterm worktrees <project>` | the project's agenterm worktrees: `ID BRANCH BASE STATE GIT SESSIONS PATH`, with `STATE` = `live`, `exited` or `orphan` |
| `agenterm worktree rm [--force] [--delete-branch] [--dry-run] <id>` (or `remove`) | removes a worktree. `--dry-run` only reports what would be lost |
| `agenterm worktree merge <id>` | `git merge --no-ff` of its branch into the main folder |
| `agenterm worktree pr [--no-open] <id>` | pushes its branch (`git push -u`, never forced) and prints the page to create the pull request into the base (GitHub / GitLab), opening it unless `--no-open`. Other hosts: push only |
| `agenterm worktree open [--finder] <id>` | opens the worktree in the editor (`git.editor`) or, with `--finder`, in Finder |

The safety rules for `rm`, `merge` and `pr` are in [worktrees.md](worktrees.md#actions).

## `agenterm notify`

Agent hooks use it, but you (or a script) can also call it **inside a session**. It never starts
the daemon and has a 3 s timeout.

```sh
agenterm notify --kind permission --message "Delete node_modules?"
agenterm notify --activity "Running migrations"
agenterm notify --state idle
```

| flag | |
|---|---|
| `--session ID` | session (defaults to `$AGENTERM_SESSION`, which the daemon injects into every session) |
| `--state S` | `running`, `waiting_input` or `idle` (aliases: `waiting`, `done`) |
| `--kind K` | `permission`, `question`, `done` or `error`; without `--state`, it implies one (`permission`/`question` → `waiting_input`, `done`/`error` → `idle`) |
| `--message M` | notification text |
| `--activity A` | activity line; without `--state` it only updates that |
| `--sock PATH` | daemon socket (defaults to the one given by the rule below) |
| `--from-hook claude\|codex` | reads a hook payload: Claude Code's from stdin, codex's as the last argument |

With `--from-hook` it prints nothing and **always** exits with 0 (2 s timeout, payload up to 1 MiB):
it never blocks or breaks the agent. Outside an agenterm session (no `--session` and no
`$AGENTERM_SESSION`) it does nothing.

## Environment variables

| variable | |
|---|---|
| `AGENTERM_HOME` | a single directory that replaces both `~/.agenterm` and `~/.config/agenterm` (useful for testing) |
| `AGENTERM_NOTIFY` | `off` disables every desktop notification of the app and the daemon (native and fallback), whatever `settings.yaml` says. Set it, with `AGENTERM_HOME`, for any test or demo instance |
| `AGENTERM_SESSION` | session id; the daemon injects it into every session and `notify` uses it by default |
| `AGENTERM_SOCK` | daemon socket; injected into every session. Ignored if `AGENTERM_HOME` is set |

### Which daemon a command uses

1. If `AGENTERM_HOME` is set, it **always wins**: `$AGENTERM_HOME/d.sock` is used.
2. Otherwise, `$AGENTERM_SOCK` (inside a session). In that case the CLI never starts another daemon.
3. If neither is set, `~/.agenterm/d.sock`.

So `AGENTERM_HOME=/tmp/x agenterm stop` run from inside a session goes to `/tmp/x` and not to the
daemon hosting the session. See [architecture.md](architecture.md#session-environment).
