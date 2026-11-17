# Architecture

**English** · [Español](es/architecture.md)

```
          ┌──────────────┐   ┌──────────────┐   ┌───────────────────────────┐
          │ agenterm.app │   │ agenterm CLI │   │ agent hooks               │
          │ (Wails)      │   │              │   │ (`agenterm notify …`)     │
          └──────┬───────┘   └──────┬───────┘   └─────────────┬─────────────┘
                 │   unix socket (~/.agenterm/d.sock)          │
                 └──────────────────┬──────────────────────────┘
                             ┌──────┴──────┐
                             │   daemon    │  PTYs, state, git, events
                             └──────┬──────┘
                 ┌──────────────────┼──────────────────┐
              claude              codex               shell …
```

- The **daemon** is the sole owner of the PTYs: it launches the processes, keeps their scrollback,
  computes state, attention, activity and git, and publishes events. It listens on a unix socket
  (`0600`).
- The **app** and the **CLI** are clients (`internal/client`). Either one starts the daemon if it
  doesn't respond (`client.StartDaemon`: a detached process in its own session, output to
  `daemon.log`). Closing the app or detaching from `attach` doesn't affect sessions.
- The `.app` is self-contained: its executable also implements `daemon` and `notify`
  (`agenterm.app/Contents/MacOS/agenterm daemon`), so the CLI isn't needed.
- Before launching the daemon, the app takes the `PATH` from your login shell (`$SHELL -l -i -c`,
  with a 3 s timeout) and sets `TERM=xterm-256color`, `COLORTERM=truecolor`, `TERM_PROGRAM=agenterm`
  and `LANG` if missing: apps opened from Finder inherit launchd's minimal environment and wouldn't
  find `claude` or `codex`. The daemon, in turn, looks for agents in its own `PATH` plus the login
  shell's.

## Packages

```
cmd/agenterm            CLI + `daemon` and `notify` subcommands (a single binary)
app/                    Wails app: bindings (app.go), output batching, attention, daemon environment
app/frontend            Svelte 5 + TypeScript + xterm.js (WebGL, fit, unicode11) + diff2html
internal/daemoncmd      `daemon` subcommand shared by the CLI and the app (+ fallback notifier)
internal/notifycmd      `notify` subcommand shared by the CLI and the app
internal/daemon         server: sessions, RPC, attach, events, git, shutdown, pid/socket
internal/client         reusable client + StartDaemon + socket resolution
internal/protocol       protocol types: line-delimited JSON + binary frames
internal/transport      Listen/Dial/Cleanup over a unix socket
internal/session        PTY (creack/pty), ring buffer, fan-out, kill/wait, activity, tail
internal/status         state machine + attention + heuristic + "stalled?"
internal/termsig        streaming parser for BEL / OSC 9 / 777 / 99 and OSC 9;4 progress
internal/hooks          per-session hook setup (claude, codex) and payload parsing
internal/profile        agent catalog, detection, merging with profiles.yaml and writing
internal/settings       settings.yaml (reading with defaults and in-place editing)
internal/gitx           git via the CLI: repo, branches, status, worktrees, diff, merge, open in editor
internal/worktree       agenterm worktree registry (worktrees.json)
internal/project        project registry (projects.json)
internal/desktopnotify  notification texts + osascript fallback
internal/macnotify      UNUserNotificationCenter (cgo, darwin): sending, permission, clicks, run loop
internal/peercred       PID of the process on the other end of the socket (LOCAL_PEERPID / SO_PEERCRED) + `ps`
internal/shellenv       login shell PATH
internal/atomicfile     atomic writes with .bak
internal/paths          directory resolution (AGENTERM_HOME)
```

## Protocol

### Control

`\n`-delimited JSON. Request `{"id":1,"method":"session.list","params":{…}}`; response
`{"id":1,"result":{…}}` or `{"id":1,"error":"…"}`. A connection accepts several requests in a row.

| group | methods |
|---|---|
| projects | `project.add`, `project.list`, `project.remove`, `project.reorder`, `project.git` |
| profiles | `profile.list`, `profile.catalog`, `profile.save`, `profile.delete`, `profile.reorder`, `profile.rescan` |
| settings | `settings.get`, `settings.save` |
| sessions | `session.spawn`, `session.list`, `session.kill`, `session.resize`, `session.notify`, `session.ack`, `session.rename`, `session.reorder`, `session.attach`, `session.tail`, `session.diff`, `session.resume`, `session.discard` |
| worktrees | `worktree.list`, `worktree.diff`, `worktree.remove`, `worktree.merge`, `worktree.pr`, `worktree.open` |
| daemon | `events.subscribe`, `daemon.ping`, `daemon.shutdown` |

`daemon.ping` returns the daemon's `version`, `commit` and `protocol` version; a method the daemon
doesn't know fails with the code `unknown_method`. See [compatibility.md](compatibility.md#daemon-protocol).

Some parameters:

- `session.spawn`: `projectId` (id or name), `profileId`, `cols`, `rows`, `mode` (`same` |
  `worktree`), `branch`, `base` and `worktreeId` (reopen a worktree; implies `worktree`).
- Session info includes `mode`, `worktreeId`, `worktreePath`, `branch`, `base`, `git`
  (`{files, uncommitted, ahead, at}`), the attention fields and the activity fields.
- `session.tail`: `{"sessionId":…,"lines":8}` (200 max) returns `{"lines":[…]}`: the end of the
  scrollback (last 64 KiB) with ANSI stripped, CR overwriting from column 0 and no runs of blank
  lines.
- `session.diff` / `worktree.diff`: `{sessionId | worktreeId, stat}`.
- `worktree.remove`: `{id, force, deleteBranch, dryRun}` → `{removed, loss}`.
- `worktree.open`: `{id | sessionId, target: "editor" | "finder"}`.
- `session.resume` / `session.discard`: `{sessionId}` or `{all: true}` → `{sessions, failed}`
  (`failed`: `[{sessionId, error, errorCode, errorData}]`, only with `all`; a single session fails
  with the error itself). Only for `suspended` sessions.
- `session.notify` also takes `agent`, `agentSessionId` and `transcriptPath` (sent by
  `notify --from-hook`): the agent's own session id, recorded for resuming. Without `state` and
  `activity` only that is recorded.
- Suspended sessions add `suspendedAt`, `resumeMode` (`agent` | `args` | `fresh`) and, after a
  failed attempt, `resumeError` / `resumeErrorCode`.
- `daemon.shutdown`: `{"force": false}` (no params = no force); fails with an error listing the
  live sessions if there are any and `force: true` isn't passed.

RPC error messages are in English. User-facing refusals also carry a stable `errorCode` plus
structured `errorData` next to the `error` message (older clients keep working), so the app can
show them in its language (see [configuration.md](configuration.md#language)):

| code | `errorData` |
|---|---|
| `live_sessions` | `{count, sessions: [{id, profileId}]}` |
| `worktree_dirty`, `worktree_unmerged` | `{path, branch, base, uncommitted, unmerged}` |
| `worktree_live` | `{sessionId}` |
| `worktree_not_found` | `{id}` |
| `merge_main_dirty` | `{dir, count}` |
| `merge_wrong_branch` | `{dir, current, base}` (empty `current` = detached HEAD) |
| `merge_nothing` | `{branch, base}` |
| `merge_conflict` | `{branch, base, files}` |
| `branch_missing`, `branch_exists`, `branch_invalid` | `{branch}` |
| `profile_not_installed` | `{profileId, cmd, install}` |
| `not_a_repo` | `{dir, project?}` |
| `no_commits` | `{dir}` |
| `base_missing` | `{base}` (empty = no base could be guessed) |
| `dir_missing` | `{dir}` |
| `resume_failed` | `{sessionId, reason, pid?}`; `reason`: `still_running`, `project_missing`, `not_suspended`, `busy` |

Unknown codes fall back to the English `error` text. The list lives in `internal/apierr/apierr.go`.

### Events

`events.subscribe` (with an optional `{"client":"ui"}`): after the response, the daemon emits one
JSON event per line.

| event | |
|---|---|
| `session.spawned` | new session |
| `session.state` | state change (with attention and activity) |
| `session.attention` | attention changed without a state change (message filled in, *ack*) |
| `session.activity` | activity line, progress, "stalled?" or output heartbeat |
| `session.git` | new git summary (`git`) |
| `session.renamed` | the session's display name changed (`sessionName`; absent: back to the profile's) |
| `session.reordered` | the sessions of `projectId` were reordered (`session.list` has the new order) |
| `project.changed` | the project list changed: added, removed or reordered |
| `session.removed` | the session was removed from the list |
| `worktree.changed` | a project's worktree list changed (`projectId`) |

`raised: true` marks a new attention (one worth notifying about); `stallRaised: true` (in
`session.activity`), a session that has just become "stalled?". If a subscriber falls behind, it's
disconnected: it must resubscribe and request `session.list`. While there's at least one `ui`
subscriber, the daemon leaves notifications to the app.

### Attach

`session.attach` (`{sessionId, cols, rows}`): after the JSON response, the connection switches to
binary frames `type (1 byte) | length (4 bytes BE) | payload` (1 MiB max):

- daemon → client: `0x01` output, `0x02` end of replay, `0x03` exit (int32 BE), `0x04` error;
- client → daemon: `0x10` input, `0x11` resize (cols, rows uint16 BE).

The daemon first sends the ring buffer snapshot and then the live output. Snapshot and subscription
are taken under the same lock, so there are no gaps or duplicates.

Attaching a `suspended` session replays its saved scrollback plus a dim "suspended since …" line,
then keeps the stream open without output (input is ignored). When the session is resumed, the
stream ends with an error frame `session resumed; reattach` (the app reattaches by itself and gets
the new process); when it is discarded, with `session discarded`.

### Go → frontend events (app)

| event | payload |
|---|---|
| `pty:<id>` | base64 output, batched (every 8 ms or 32 KiB) |
| `pty-replay-done:<id>` | end of the scrollback replay |
| `pty-exit:<id>` | exit code |
| `pty-error:<id>` | stream lost (the frontend reattaches and repaints) |
| `daemon:event` | any daemon event (table above) |
| `daemon:resync` | (re)subscribed: reload lists |
| `daemon:down` | no connection to the daemon (it retries and relaunches it) |
| `app:focus-session` | a notification was clicked: select that session |
| `app:open-settings` | the settings item in the **agenterm** app menu |

## Sessions

- Each session has a scrollback ring buffer (8 MiB) and N subscribers with bounded queues; a slow
  subscriber is disconnected (frame `0x04`) instead of blocking the PTY read.
- `kill` sends SIGHUP and SIGTERM to the process group and SIGKILL after 2 s. A second `kill` on an
  exited session removes it from the list.
- Project and session ids are 4 random bytes in hex.

### Session environment

The daemon injects into every session:

- `AGENTERM_SESSION=<id>`;
- `AGENTERM_SOCK=<its socket>`;
- `AGENTERM_HOME=<its directory>` if the daemon uses `AGENTERM_HOME`; if it uses the default paths,
  it **removes** any inherited `AGENTERM_HOME`.

Clients give `AGENTERM_HOME` priority over `AGENTERM_SOCK` (see
[cli.md](cli.md#which-daemon-a-command-uses)), so both must point to the same daemon. Generated hooks
also pass `--sock <the daemon's socket>`, so they reach their daemon even if you change
`AGENTERM_HOME` inside the session.

## Persistence and resume

| survives… | closing the app | restarting the daemon / the computer |
|---|---|---|
| session processes | yes | **no** (they are respawned on resume) |
| session list and scrollback | yes | yes (as `suspended` sessions) |
| the agent's conversation | yes | claude and codex: yes (resumed by id); others: with `resume_args` |
| projects (`projects.json`), profiles, settings | yes | yes |
| worktrees and their registry (`worktrees.json`) | yes | yes |

Processes live in the daemon: if it stops (`agenterm stop --force`, SIGTERM, a crash, restarting the
computer) they die. What the next daemon needs to bring them back is kept on disk, in the runtime
dir:

- **`sessions.json`**: one record per session — `id`, `projectId`, `profileId`, `name` (the display name, if any), `mode`,
  `worktreeId`, `cwd`, `cmd`/`args` as launched (shown while suspended), `pid`, size, `createdAt`,
  the last `state`, `suspendedAt` and `resume` (`{agent, sessionId, transcriptPath, at}`). The
  resolved environment is **never** stored (profiles may put secrets in `env`): a resume
  re-resolves the profile from `profiles.yaml`. Written on spawn, exit, removal and when the agent
  reports its session id; atomic, `0600`. A file that doesn't parse is moved aside
  (`sessions.json.corrupt-<time>`, logged) and the daemon starts without sessions to restore.
- **`scrollback/<id>.log`**: the session's ring buffer (8 MiB max, the ring's capacity), rewritten
  atomically (`0600`) at most every 2 s while there is new output, and once more when the daemon
  stops (before killing the sessions).

A session that exits, or that you kill, loses both (it's not restored). On startup, the sessions
that were live when the previous daemon stopped come back as **`suspended`**: listed, with their
scrollback, but without a process. Then `sessions.restore` ([configuration.md](configuration.md#settingsyaml))
decides: `ask` (default) leaves them suspended until you choose **Resume** or **Discard** (sidebar
row, or the "N sessions from the previous run · Resume all / Discard all" banner; CLI
`agenterm resume|discard <id>|--all`), `always` resumes them right away and `never` discards them.

**Resuming** respawns the profile in the same folder — the same worktree for worktree sessions —
with the same id, size and creation time. The new terminal starts with the saved scrollback, the
modes a full-screen program left on reset (alternate screen, mouse, hidden cursor…) and a dim
`── session restored · <time> ──` line, written by the daemon into the scrollback in the
`ui.language` language (a plain ANSI line rather than a protocol marker: every client — app, CLI
`attach`, `tail` — shows it without changes). How the agent is restarted (`resumeMode`):

| profile | restarted as | `resumeMode` |
|---|---|---|
| `setup_hooks: claude` with a known session id | `claude --settings <hooks> --resume <session_id> <args>` | `agent` |
| `setup_hooks: codex` with a known thread id | `codex resume -c notify=… <args> <thread-id>` | `agent` |
| with `resume_args` (opencode: `--continue`) | `<cmd> <hooks> <args> <resume_args>` | `args` |
| anything else (shell, agents without a session id yet) | `<cmd> <hooks> <args>`: a fresh process | `fresh` |

Claude's `session_id` comes with every hook payload (agenterm also listens to `SessionStart`, so it
is known from the start and follows `/clear`); codex reports its `thread-id` in the `notify`
payload, i.e. after its first turn. Both flags were checked with `claude --help` (`--resume <id>`)
and `codex resume --help` (`codex resume [OPTIONS] [SESSION_ID]`).

What is **not** restored: the process itself. A shell comes back in the session's starting folder
with nothing running; whatever was running in it (servers, `vim`…) is gone. An agent without a
resumable id starts a new conversation. Resume fails — and the session stays suspended, with the
error in its tooltip / `ls` — when the folder or worktree no longer exists (`dir_missing`), the
agent is no longer installed (`profile_not_installed`), the project was removed or the previous
process is somehow still running (`resume_failed`; a pid is only considered alive if it leads its
own process group and still runs the same command, so a recycled pid doesn't block it).

## Security and shutdown

- The socket is `0600` and the directories `0700`: only your user can talk to the daemon.
- `daemon.shutdown` **refuses** without `force` while there are live sessions, so an
  `agenterm stop` aimed at the wrong daemon by mistake (for example, a test's cleanup) doesn't take
  down work in progress. SIGTERM/SIGINT to the daemon still always shut it down (the system is
  asking).
- Every shutdown request, accepted or refused, is logged in `daemon.log` with who made it:
  `peer_pid`, `peer_cmd`, `peer_ppid` and `peer_parent_cmd` (the PID on the other end of the socket
  via `LOCAL_PEERPID` on macOS / `SO_PEERCRED` on Linux, and its command line via `ps`), plus
  `force` and `live`. The shutdown reason (signal or RPC) is logged too.
- SIGTERM/SIGINT are caught with a `SA_SIGINFO` handler (`internal/sigsender`, cgo on macOS and
  Linux), so a `termination signal received` line records the **sender** as well: `sender_pid`,
  `sender_uid`, `sender_cmd`, `sender_ppid` and `sender_parent_cmd` (looked up right away; a sender
  that already exited shows only its PID; some kernels or sandboxes report no sender at all, logged
  as `sender_pid=0`). Without cgo the daemon falls back to `os/signal`.
- `AGENTERM_HOME` takes precedence over `AGENTERM_SOCK`: `AGENTERM_HOME=/tmp/x agenterm stop` run
  from inside a session goes to `/tmp/x`, not to the real daemon.
- On shutdown, the daemon kills every session and removes the socket and the pid file. A stale
  socket (nobody listening, after a crash) is detected and replaced on startup.
