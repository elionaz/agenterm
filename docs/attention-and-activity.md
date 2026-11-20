# Attention and activity

**English** · [Español](es/attention-and-activity.md)

Each session has a **state** (what the process is doing), an **attention** (why it needs you) and
**activity** data (what it's doing right now, for how long, and whether it looks stalled).

## States

| state | meaning |
|---|---|
| `running` | working |
| `waiting_input` | waiting for your reply (approval, question or terminal signal) |
| `idle` | idle / turn finished |
| `exited` | the process ended (final state) |
| `suspended` | restored after a daemon restart: no process yet, only its saved scrollback; resume or discard it (see [architecture.md](architecture.md#persistence-and-resume)) |

There are two sources:

- **Heuristic** (`status: heuristic` profiles): output → `running`; 3 s without output (or the
  profile's `idle_seconds`, or `panal daemon --idle`) → `idle`. `waiting_input` isn't ended by
  output (spinners keep drawing) but by what you type. Automatic terminal replies (focus, DA,
  DSR…) don't count as typing.
- **Explicit events** (agent hooks or `panal notify`). In `status: hooks` profiles, the first
  explicit event turns the heuristic off for that session.

## Attention

| field | |
|---|---|
| `attentionKind` | `permission`, `question`, `done` or `error` (empty if it doesn't need you) |
| `attentionMessage` | text (the command asking for permission, the question, the last message…) |
| `attentionAt` | when it was raised |
| `unread` | you haven't seen it yet |

- `unread` is cleared when the session is **on screen** in the app, selected or in any split pane,
  while the window is active (RPC `session.ack`), when you
  **type into it** or with `panal ack <id>`. A session still waiting for an approval or an
  answer (`waiting_input`) is the exception: looking at it doesn't answer it, so it stays flagged
  until you type into it or the agent moves on.
- An explicit `idle` after a turn (Claude Code's `Stop` hook, codex's `agent-turn-complete`, Gemini
  CLI's `AfterAgent`, opencode's `session.execution.succeeded`) is a `done`. A heuristic `idle` does **not** notify: no output doesn't mean it's finished.

## Where the signals come from

Integrations only apply to the sessions panal launches. `~/.claude/settings.json`,
`~/.codex/config.toml`, `~/.gemini/`, opencode's configuration, `~/.cursor/` and the project are
never touched: everything goes through command-line flags, environment variables and files
generated in `~/.panal/hooks/` (or `$PANAL_HOME/hooks/`). Hooks call `<daemon executable> notify --sock <socket> --from-hook …` with
absolute paths: the CLI, or the `.app`'s own executable if the app started the daemon.

### Claude Code (`setup_hooks: claude`)

panal generates `hooks/claude-settings.json` and launches `claude --settings <that file> …`.
Hooks from `--settings` are **added** to yours. Each hook has a 5 s timeout (3 s for
`PreToolUse`).

| hook | effect |
|---|---|
| `SessionStart` | no state change: records Claude's `session_id`, so a daemon restart can resume the conversation |
| `UserPromptSubmit` | `running`, activity "Thinking…" and a new turn (resets the timer) |
| `PreToolUse` | `running` and an activity line based on the tool (see below) |
| `PostToolUse` | `running` |
| `PermissionRequest` | `waiting_input` + `permission`, with a summary ("Bash: rm -rf node_modules") |
| `Notification` `permission_prompt` | `waiting_input` + `permission` |
| `Notification` `elicitation_dialog`, `elicitation_url_dialog`, `agent_needs_input` | `waiting_input` + `question` |
| `Stop` | `idle` + `done`, with the assistant's last message |
| `StopFailure` | `idle` + `error`, with the error details |

Other notifications (such as the `idle_prompt` reminder) are ignored. `notify --from-hook` prints
nothing and exits with 0, so it **never** emits a permission decision: every tool goes through
Claude's normal approval flow.

### codex (`setup_hooks: codex`)

```sh
codex -c 'notify=["<exe>","notify","--sock","<socket>","--from-hook","codex"]' \
      -c 'tui.notifications=["approval-requested","plan-mode-prompt","async-question"]' \
      -c 'tui.notification_method="osc9"' \
      -c 'tui.notification_condition="always"' …
```

- `notify` with `agent-turn-complete` → `idle` + `done` (with the last message).
- Approvals and questions arrive as OSC 9 notifications from the TUI and are picked up by the
  terminal-signal parser.
- codex doesn't report when it starts working again, so the profile uses `status: heuristic`.

### Gemini CLI (`setup_hooks: gemini`)

Gemini CLI has no `--settings` flag, but it reads its *system* settings from the file in
`GEMINI_CLI_SYSTEM_SETTINGS_PATH`, and hook lists from every settings scope are **added** together.
panal generates `hooks/gemini-settings.json` and launches `gemini` with that variable pointing at it.
If a real system settings file exists (`/Library/Application Support/GeminiCli/settings.json`,
`/etc/gemini-cli/settings.json`), it is copied into the generated file so nothing of it is lost. If it
isn't plain JSON (comments), panal leaves it in charge and the session runs without hooks.
`GEMINI_CLI_SYSTEM_DEFAULTS_PATH` keeps pointing at the real `system-defaults.json`.

| hook | effect |
|---|---|
| `SessionStart` | no state change: records Gemini's `session_id` (for `gemini --resume <id>`) and the model from its transcript; hands over the standing instructions and the introduction to the other sessions |
| `BeforeAgent` | `running`, "Thinking…" and a new turn; hands over pending messages |
| `BeforeTool` | `running` and an activity line (`run_shell_command` → Bash, `write_file`/`replace` → editing…) |
| `AfterTool` | `running` |
| `Notification` `ToolPermission` | `waiting_input` + `permission` ("Shell: rm -rf build", "Edit: main.go") |
| `AfterAgent` | `idle` + `done` with the reply; with pending messages it **blocks** the end of the turn and hands them over as the next prompt (like Claude's `Stop`), never twice in a row |

- Gemini only runs settings hooks in **trusted folders**: in a folder you haven't trusted, the session
  runs without them (the heuristic still works).
- A turn that fails or that you cancel doesn't fire `AfterAgent`, so the profile keeps `status:
  heuristic`: such a turn goes back to `idle` once the output is quiet.
- Messages from other sessions are handled as with Claude Code (see
  [messages](messages.md)).

### opencode (`setup_hooks: opencode`)

opencode 2 has no hook commands; it has plugins that run inside its server. panal generates a
plugin in `hooks/opencode-plugin/` and launches `opencode --standalone` with
`OPENCODE_CONFIG_CONTENT={"plugins":[{"package":"<that folder>"}],"instructions":["<file>"]}`, an
extra configuration document merged over yours and the project's. `--standalone` gives the session
its own server, which inherits that environment (by default opencode 2 uses a shared background
service started with its own environment). The plugin forwards the events below to `notify
--from-hook opencode`, one at a time and in order.

| event | effect |
|---|---|
| `session.created` | no state change: records the session id (for `opencode --session <id>`) and the model |
| `session.execution.started` | `running`, "Thinking…" and a new turn |
| `session.tool.called` | `running` and an activity line |
| `permission.asked` | `waiting_input` + `permission` ("bash: rm -rf dist") |
| `form.created` | `waiting_input` + `question` (its title) |
| `permission.replied`, `form.replied`, `form.cancelled` | `running` |
| `session.execution.succeeded` | `idle` + `done` |
| `session.execution.failed` | `idle` + `error`, with the error |
| `session.execution.interrupted` (by you) | `idle` + `done` ("Interrupted") |
| `session.step.started`, `session.model.selected` | the model, when it changes |

- Every turn reports its end, so the profile uses `status: hooks`.
- Subagent sessions only report what waits for you (permissions, questions), for the main session.
- Standing instructions arrive as an instructions file (`hooks/instructions/<project>.md`).
- Needs opencode 2 or later (`opencode --version`); with opencode 1, or if `OPENCODE_CONFIG_CONTENT`
  is already set (in your environment or the profile's `env`), the session runs without the
  integration (the daemon log says why).
- Messages are delivered like codex's: a line asking it to run `panal msg read` once it has finished
  its turn.

### Cursor CLI (`setup_hooks: cursor`)

`cursor-agent --plugin-dir <dir>` loads a local plugin for that invocation. panal generates one in
`hooks/cursor-plugin/` (`.cursor-plugin/plugin.json` and `hooks/hooks.json`).

| hook | effect |
|---|---|
| `sessionStart` | no state change: records the chat id (`conversation_id`, for `cursor-agent --resume <id>`) and the model; hands over the standing instructions as `additional_context` (new chats only: Cursor doesn't run it when resuming) |
| `preToolUse` | `running` and an activity line (`Shell` → Bash, `MCP:<tool>`…); prints nothing, so it never answers a permission |
| `postToolUse`, `postToolUseFailure` | `running` |
| `beforeSubmitPrompt`, `stop` | `running` + new turn / `idle` + `done` or `error`; registered, but as of cursor-agent 2026.11 the CLI only runs them from `~/.cursor/hooks.json` or the project's, not from plugins |

- No turn end and no approval event reach panal, so the profile keeps `status: heuristic` and its
  attention patterns for Cursor's approval menus.

### Terminal signals (any agent)

The parser is agent-agnostic and doesn't alter the stream: bytes reach the terminal untouched.

| signal | |
|---|---|
| bare BEL | no message (the BEL that terminates an OSC doesn't count) |
| `OSC 9 ; message` | iTerm2 / Ghostty. `OSC 9;4` is **progress** (below) and never asks for attention; other numeric ConEmu subcommands are ignored |
| `OSC 777 ; notify ; title ; body` | urxvt / foot / Ghostty |
| `OSC 99` | kitty |
| `attention_patterns` | the profile's regexes over recent output with ANSI stripped, evaluated once output has been quiet for ~300 ms |

A signal puts the session in `waiting_input` with a `question` attention, which lasts until you
type. In sessions with hooks they only count while the agent is `running` (hooks already report
waits and turn ends); if there's already an unread attention, a signal only fills in its message.

## Notifications

### While the app is open

The app subscribes to the daemon as a `ui` client and handles notifications itself:

- A native notification if the window isn't active or the session isn't on screen (in any split
  pane). Title `<project> · <name>` (the session's name, or its profile); the body is the message, or a generic text for the kind (needs your
  approval, waiting for your reply, finished, finished with an error) in the app language
  (`ui.language`). Clicking it activates the app and selects the session.
- Sound only for approvals and questions (`notify.sound`).
- A Dock badge with the number of pending sessions, and the icon bounces on approvals (if the app
  isn't in front).
- In the sidebar: a pulsing amber dot, a label (approval / question / done / error), a tooltip with
  the message, a per-project counter and `panal (N)` in the window title.
- Marking the attention as seen withdraws the delivered notification.

### While the app is closed

When no `ui` client is subscribed, the daemon notifies:

- If its executable is inside a `.app` and the bundle has permission, it uses
  `UNUserNotificationCenter`: the notification shows up as **panal**, with its icon and the same
  identifier the app uses (`panal-<sessionId>`). It asks for permission the first time.
- The daemon runs as the app's executable whoever starts it: the app, and also the CLI bundled in
  the `.app` (`Contents/Helpers/panal`, which Homebrew links into your `PATH`) — its auto-start
  and `panal daemon [--detach]` run `panal.app/Contents/MacOS/panal daemon` instead of
  themselves. Only a CLI installed outside the bundle (`go install`, the CLI archives) runs the
  daemon as itself.
- On click, the daemon runs `open -a <its .app> [--env PANAL_HOME=…] panal://session/<id>`:
  the app opens (or activates) and selects that session.
- If native notifications can't be used (the daemon isn't in a bundle, there's no permission or
  sending fails), what happens depends on `notify.fallback`:
  - `none` (default): the notification is not shown; `daemon.log` says so, with the reason, once.
  - `osascript`: it's sent with `osascript` (`display notification`). macOS attributes it to
    *Script Editor*, and clicking it opens Script Editor, not panal. The app applies the same
    setting to its own notifications (*Settings → Notifications → Fallback via AppleScript*).
- `PANAL_NOTIFY=off` in the environment of the app or the daemon disables every desktop
  notification (native and fallback), whatever `settings.yaml` says: for test and demo instances
  (see [development.md](development.md#testing-without-touching-your-installation)).
- None of this blocks the daemon: native sending is capped at ~3 s and `osascript` at 5 s.
- `daemon.log` records every notification (`notifier=native` or `osascript`), dropped ones
  (`notifications not shown`, with the reason) and clicks (`notification clicked`).

### Notification permission

The first time you open the `.app`, macOS asks whether to allow notifications. If you declined:
*System Settings → Notifications → panal* → "Allow notifications" (and the "Alerts" style if you
want them to stay on screen). With `notify.fallback: osascript`, those notifications are
controlled under *Notifications → Script Editor*.

**Settings → Notifications** shows that permission at the top (allowed, not allowed, not asked
yet), with **Ask macOS** while it hasn't asked, **Open System Settings** on panal's entry, and
**Send a test notification**, which shows one right away and says why when it can't. The app reads
the permission again when you come back from System Settings, and before a notification it had to
drop (at most every 30 seconds), so a permission granted after launch applies without restarting the app.

### URL scheme

`panal://session/<id>` opens the app on that session. It's registered in `CFBundleURLTypes` (via
`protocols` in `app/wails.json`) and the app receives it through `mac.Options.OnUrlOpen`; if the app
wasn't open, it selects the session once it finishes loading.

## Activity

Each session carries, in `session.list` and in events:

| field | |
|---|---|
| `activity`, `activityAt`, `activitySource` | activity line and its source: `hook` or `output` |
| `turnStartedAt`, `turnEndedAt` | start of the current (or last) turn and its end (on going `idle`/`exited`) |
| `lastOutputAt`, `lastEventAt` | last PTY output and last explicit event |
| `stalled`, `stalledAt` | the turn looks stalled (a flag, not a state) |
| `progress` | `{state: normal\|error\|indeterminate\|paused, value: 0-100}` |

### Activity line

- **Claude Code** (`PreToolUse` hook): `Edit`/`Write`/`MultiEdit`/`NotebookEdit` → "Editing
  main.go", `Read` → "Reading go.mod", `Bash` → "Bash: <command, ~60 characters>", `Grep`/`Glob` →
  "Searching <pattern>", `WebFetch` → "Web: <host>", `WebSearch` → "Web: <query>", `Task`/`Agent` →
  "Subagent: <description>", `TodoWrite` → "Updating tasks", `mcp__srv__tool` → "MCP: srv.tool";
  any other tool, its name. With hooks, output never sets the activity, not even before the first
  hook, so Claude Code's status line (a HUD) doesn't show up as activity. The daemon sends
  `activityKind` (`thinking`, `edit`, `read`, `bash`, `search`, `web`, `subagent`, `todo`, `mcp`,
  `tool`) and `activityTarget` next to the English `activity` text, and the app renders it in its
  language.
- **Other agents**: the last non-empty output line (ANSI stripped), trimmed. A CR that redraws the
  line keeps its latest version. Lines that are only spinners or box drawing, have fewer than two
  letters or digits, or look like the **shell prompt** (short and ending in `$ % # > ❯ ➜` in the
  usual forms, or starting with `user@host`) are ignored, so in a shell you see the last output and
  not `user@mac ~ %`. In TUIs that redraw the screen (opencode, codex…) it's the last line they
  drew, which isn't always the most informative.
- `panal notify --activity "text"` also sets it (as `hook`).
- The daemon emits `session.activity` in batches: at most one every 500 ms per session (always with
  the latest value), plus a heartbeat every 5 s while there's output.

### Timers

`turnStartedAt` is set on going `running` from `idle` (or when you answer a wait detected by the
terminal) and on every `UserPromptSubmit`; an approval (explicit wait → `running`) continues the
same turn. The app shows how long the agent has been working (e.g. 4m 12s), the time since the last
output (from 10 s on) and, when idle, how long the turn took (e.g. 6m).

### "Stalled?"

If a turn opened **by a hook** (explicit `running`) goes `activity.stall_seconds` (180 by default)
without output or events, the session is marked `stalled` (a `session.activity` event with
`stallRaised: true`) and the app shows it in orange, with the time since the last activity (e.g.
5m). Any output or event clears it.

- With `notify.notify_on_stall`, a `stall` notification is sent: no sound, no Dock bounce, and it
  doesn't count towards the badge or as pending. With the app closed it takes the same path as the
  daemon's other notifications.
- Heuristic sessions have no "open turn" (3 s without output is already `idle`), so they're never
  flagged.
- Claude Code's spinner redraws every second while it works, and that counts as output: with Claude
  the warning mostly catches complete hangs (stopped process, frozen terminal), not a slow tool.

### Permission mode

What a session may do without asking you, read from the flags it was launched with (the profile's
`args`): **auto-approve** (runs commands and edits files unasked), **auto-edit** (edits unasked,
asks for commands) or **plan** (read-only). The title bar shows it as a chip, the sidebar row gets a
⚠ for auto-approve, and the tooltips show the full command line.

| agent | auto-approve | auto-edit | plan |
|---|---|---|---|
| Claude Code | `--dangerously-skip-permissions`, `--permission-mode bypassPermissions` | `--permission-mode acceptEdits` | `--permission-mode plan` |
| codex | `--dangerously-bypass-approvals-and-sandbox`, `--yolo`, `-a never`, `-s danger-full-access` | `--full-auto` | `-s read-only` |
| Gemini, Qwen | `--yolo`, `-y`, `--approval-mode yolo` | `--approval-mode auto_edit` | `--approval-mode plan` |
| Cursor | `--force`, `-f` | | |
| Copilot | `--allow-all-tools`, `--allow-all`, `--yolo` | | |
| Amp | `--dangerously-allow-all` | | |
| Crush | `--yolo`, `-y` | | |
| Kiro | `--trust-all-tools` | | |
| Aider | `--yes-always` | | |

It reflects the launch only: a mode switched inside the agent afterwards (Shift+Tab in Claude Code)
isn't seen.

### Model

The session's row and its title bar show the agent's model next to its name (*Opus 5.5*; the full
id in the tooltip). Claude Code reports it through its hooks: the `model` of the hook payload when
there is one, otherwise the model of the last assistant message in the session's transcript (read
from its end, at most 256 KiB, at the start of the session, on each prompt and at the end of each
turn), so `/model` shows up by the next turn. Other agents show one only when their command line
names it (`--model X`, `-m X`, `--model=X` or codex's `-c model=X` in the profile's `args`). It is
kept across daemon restarts.

### Progress

`OSC 9 ; 4 ; st ; pr` (ConEmu / Windows Terminal / Ghostty; `st`: 0 clears, 1 normal, 2 error, 3
indeterminate, 4 paused; `pr`: 0-100) is shown as a thin bar under the session's row and at the top
of the terminal pane. It never raises attention.

### Overview (⌘0)

The ▦ button in the sidebar header (or each project's) opens a grid with the current project's
sessions (with a toggle to show all projects): profile, state, attention, activity, timers,
progress, branch and the last 8 lines of output as plain text (RPC `session.tail`, refreshed when
events arrive and at most every 1.5 s per session). Clicking a card opens the session; Esc or ⌘0
closes the overview.
