<p align="center">
  <img src="assets/icon/agenterm-icon-rounded.svg" alt="agenterm" width="96">
</p>

# agenterm

**English** · [Español](README.es.md)

A desktop workspace for coding agents, organized by project. Terminal tabs can't be grouped by
project, and once you run several agents (Claude Code, codex, opencode, Gemini, Cursor…) on the same
repo it's hard to tell which one is working, which one is done and which one is waiting for your
approval. agenterm groups sessions by project, keeps agents alive even after you close the app and
tells you when one of them needs you.

<!-- TODO: screenshot — add docs/img/screenshot.png -->

## Features

- **Projects and sessions** in a sidebar: each project is a folder and each session is a terminal
  (xterm.js) running an agent or a shell.
- **Any agent CLI**: a built-in catalog (Claude Code, codex, opencode, Gemini, Cursor, Copilot,
  aider, Amp, goose, Qwen, Crush, Kiro, shell) that detects which ones are installed, plus your own
  profiles and variants in `profiles.yaml`.
- **Sessions outlive the app**: a daemon owns the PTYs; close the window, reopen it and everything
  is still there, scrollback included. The CLI and the app share the same daemon. If the daemon
  itself restarts, sessions come back suspended with their scrollback and resume with one click
  (Claude Code and codex continue the same conversation).
- **Attention detection**: approvals, questions, finished turns and errors (agent hooks or terminal
  signals), with native macOS notifications, a Dock badge and sidebar markers.
- **Live activity**: what each agent is doing ("Editing main.go"), a turn timer, a "stalled?"
  warning, progress bars (OSC 9;4) and a grid overview (⌘0).
- **Per-session git worktrees**: each agent can work on its own branch and folder; the **Changes**
  tab shows the diff and lets you merge into the base or safely remove the worktree.
- **In-app settings**: enable, reorder and edit agents, and configure the language, notifications
  and git.
- **English and Spanish**: the app follows your macOS preferred languages (falling back to English)
  or the language you pick in **Settings → General**.

## Status and requirements

Early-stage project: there are no signed releases yet, so you have to build it from source.

- **macOS** (tested on Apple Silicon). The app and notifications are macOS-only.
- **Go 1.27.4+** (see `go.mod`).
- **Node 24+** and npm (for the frontend; it's the version CI uses).
- **Wails v2 CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1` (`wails doctor`
  must pass).

## Building from source

```sh
git clone https://github.com/elionaz/agenterm.git
cd agenterm

# Desktop app (includes the daemon)
cd app && wails build          # -> app/build/bin/agenterm.app
open build/bin/agenterm.app    # or copy it to /Applications

# CLI (optional)
cd .. && go build -o agenterm ./cmd/agenterm
```

The first time you open the app, macOS asks whether to allow notifications. If you declined:
*System Settings → Notifications → agenterm* → "Allow notifications".

## Quick start

**App.** Click the sidebar's **+ Project** button and pick a folder. With the **+** next to a
project you choose an agent and whether it runs in the project folder or in a new worktree; **⌘N**
launches the first agent in the list. When an agent needs you, you'll see an amber dot in the sidebar
and get a notification.

**CLI.** Any command starts the daemon if it isn't running:

```sh
agenterm project add ~/Dev/myapp           # -> <projectId>  myapp  /Users/me/Dev/myapp
agenterm profiles                          # catalog agents, installed or not
agenterm spawn myapp claude                # -> <sessionId>
agenterm spawn --worktree myapp codex      # -> <sessionId>  <branch>  <folder>
agenterm ls                                # state, attention, activity, branch…
agenterm attach <sessionId>                # Ctrl-\ to detach; the session keeps running
agenterm stop                              # refuses while sessions are live (use --force)
```

Full reference: [docs/cli.md](docs/cli.md).

## How attention is detected

Everything is configured **only for the sessions agenterm launches**, through command-line flags and
files generated in `~/.agenterm/hooks/`. Your own configuration (`~/.claude/settings.json`,
`~/.codex/config.toml`…) and the project folder are never modified.

| agent | mechanism | what it detects |
|---|---|---|
| Claude Code | hooks via `claude --settings <generated file>` (added on top of yours) | working + activity, approval, question, done, error |
| codex | `codex -c notify=[…]` + OSC 9 notifications from the TUI | done (`notify`); approvals and questions (OSC 9) |
| everything else | terminal signals: BEL, OSC 9, OSC 777, OSC 99 and `attention_patterns` | waiting for your reply |

Details (states, *ack*, "stalled?", progress, notifications while the app is closed):
[docs/attention-and-activity.md](docs/attention-and-activity.md).

## Configuration

| | default | with `AGENTERM_HOME=/x` |
|---|---|---|
| daemon socket, pid, log | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| `profiles.yaml`, `settings.yaml`, `projects.json` | `~/.config/agenterm/` | `/x/` |
| generated hooks, worktrees, `worktrees.json` | `~/.agenterm/{hooks,worktrees,worktrees.json}` | `/x/…` |

`profiles.yaml` and `settings.yaml` are optional, and the app edits them from **Settings** (⌘,).

**`profiles.yaml`** is layered over the catalog by `id`: you change only the fields you set, create
variants with `extends`, hide agents with `enabled: false`, and the list order is the menu order (the
first one is what ⌘N launches):

```yaml
- id: claude-yolo                 # variant: inherits everything from claude
  name: Claude (no permissions)
  extends: claude
  args: ["--dangerously-skip-permissions"]
- id: claude
  env: {CLAUDE_CODE_USE_BEDROCK: "1"}
- id: gemini
  enabled: false
- id: mine                        # your own profile
  cmd: $HOME/bin/my-agent
  attention_patterns: ['\(y/n\)']
```

**`settings.yaml`** (defaults):

```yaml
ui:
  language: auto         # auto (macOS preferred languages, else English) | en | es
notify:
  enabled: true          # notifications (app and daemon)
  sound: true            # sound on approval/question
  notify_on_done: true   # also notify when an agent finishes
  notify_on_stall: true  # low-key notice when a turn looks stalled
activity:
  stall_seconds: 180     # seconds without output or events before "stalled?"; 0 disables it
git:
  editor: ""             # for "Open in editor"; empty: code if available, else Finder
  worktree_dir: ""       # where to create worktrees; empty: ~/.agenterm/worktrees
  default_mode: same     # same | worktree
```

All the rules and the full catalog: [docs/configuration.md](docs/configuration.md). Worktrees and
diffs: [docs/worktrees.md](docs/worktrees.md).

## Keyboard shortcuts

| shortcut | action |
|---|---|
| ⌘N | new session in the current project with the first enabled, installed agent |
| ⌘1 … ⌘9 | select session n of the current project |
| ⌘0 | open/close the overview |
| ⌘, | open Settings |
| ⌘C / ⌘V | copy the selection / paste into the terminal |
| Esc | close the overview, Settings or an orphaned worktree's view |
| `Ctrl-\` | detach from `agenterm attach` (CLI) |

## Documentation

- [docs/cli.md](docs/cli.md): every CLI command and flag.
- [docs/configuration.md](docs/configuration.md): paths, `profiles.yaml`, the agent catalog,
  `settings.yaml` and the app language.
- [docs/attention-and-activity.md](docs/attention-and-activity.md): hooks, terminal signals,
  notifications, activity, "stalled?" and progress.
- [docs/worktrees.md](docs/worktrees.md): per-session worktrees, diff, merge and removal.
- [docs/architecture.md](docs/architecture.md): daemon, app and CLI, protocol, events and
  persistence.
- [docs/development.md](docs/development.md): development, tests, the icon and known limitations.

If you change commands, flags, paths or `settings.yaml`, update the README and the matching document
in `docs/`, in English (`README.md`, `docs/`) and Spanish (`README.es.md`, `docs/es/`).

## Contributing

Contributions are welcome via pull request: read [CONTRIBUTING.md](CONTRIBUTING.md). To report a
vulnerability, follow [SECURITY.md](SECURITY.md).

License: [MIT](LICENSE).
