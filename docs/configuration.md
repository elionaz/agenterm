# Configuration

**English** · [Español](es/configuration.md)

## Paths

| | default | with `AGENTERM_HOME=/x` |
|---|---|---|
| daemon socket, pid and log | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| profiles, projects and settings | `~/.config/agenterm/{profiles.yaml,projects.json,settings.yaml}` | `/x/…` |
| generated hook files | `~/.agenterm/hooks/` | `/x/hooks/` |
| session worktrees | `~/.agenterm/worktrees/<project>-<projectId>/<sessionId>/` (or `git.worktree_dir`) | `/x/worktrees/…` |
| worktree registry | `~/.agenterm/worktrees.json` | `/x/worktrees.json` |
| sessions to restore and their scrollback | `~/.agenterm/sessions.json`, `~/.agenterm/scrollback/<id>.log` | `/x/…` |
| messages between sessions | `~/.agenterm/messages.json` | `/x/messages.json` |

- Directories are created with `0700` permissions and the socket with `0600`.
- On macOS a unix socket path is limited to about 104 bytes: if you use `AGENTERM_HOME`, keep it
  short (`/tmp/agt`, not a long temporary folder under `/var/folders/…`).
- `profiles.yaml` and `settings.yaml` are optional. When the app (or an RPC) saves them, the write
  is atomic and leaves the previous version in `<file>.bak`.

## Agents: catalog and detection

agenterm ships a catalog of known agent CLIs. An agent counts as **installed** if its binary is
found in the daemon's `PATH` or in your login shell's (`$SHELL -l -i -c`); that lookup is cached for
5 minutes. `agenterm profiles scan` or the rescan button in **Settings → Agents** refreshes it.

| id | agent | command | status | hooks | notes |
|---|---|---|---|---|---|
| `claude` | Claude Code | `claude` | `hooks` | `claude` | |
| `codex` | Codex CLI | `codex` | `heuristic` | `codex` | `notify` only reports the end of a turn, so "working" is decided by the heuristic |
| `opencode` | opencode | `opencode` | `heuristic` | | |
| `gemini` | Gemini CLI | `gemini` | `heuristic` | | |
| `cursor-agent` | Cursor CLI | `cursor-agent` | `heuristic` | | |
| `copilot` | GitHub Copilot CLI | `copilot` | `heuristic` | | |
| `aider` | Aider | `aider` | `heuristic` | | attention pattern `\(Y\)es/\(N\)o` |
| `amp` | Amp | `amp` | `heuristic` | | |
| `goose` | goose | `goose session` | `heuristic` | | |
| `qwen` | Qwen Code | `qwen` | `heuristic` | | |
| `crush` | Crush | `crush` | `heuristic` | | |
| `kiro` | Kiro CLI | `kiro-cli chat` | `heuristic` | | binary name not verified |
| `shell` | Shell | `$SHELL` (or `/bin/sh`) | `heuristic` | | always available |

- **status** (`status`): `hooks` = state is decided by explicit events (hooks / `notify`) and the
  heuristic is only used until the first event; `heuristic` = output → working, a few seconds
  without output → idle. See [attention-and-activity.md](attention-and-activity.md).
- **hooks** (`setup_hooks`): the integration agenterm sets up when launching the agent.
- Agents that aren't installed (or are disabled) don't appear in the **+** menu or on ⌘N; they do
  appear in **Settings**, with the install command and a link.

## `profiles.yaml`

A `version` key (the [file format](compatibility.md#file-formats)) and a `profiles:` list (files
from before 0.4.0 are just the list, and are still read). Each entry has a required, unique `id` and is **layered over** the catalog:

| field | |
|---|---|
| `id` | identifier (letters, digits, `.`, `_`, `-`) |
| `name` | display name |
| `extends` | id of the profile it inherits from (custom profiles only) |
| `enabled` | `false` hides it from the menus; `true` by default (not inherited) |
| `order` | explicit position (lowest first); without it, the position in the list counts |
| `cmd` | command; `$VARIABLES` are expanded |
| `args` | list of arguments |
| `env` | `KEY: value` map (values also expand `$VARIABLES`) |
| `status` | `hooks` or `heuristic` |
| `setup_hooks` | `claude`, `codex` or `none` |
| `idle_seconds` | seconds without output before going idle (overrides the daemon's value, 3 s) |
| `attention_patterns` | regular expressions (Go syntax) over recent output with ANSI stripped; a match marks the session as waiting for your reply |
| `resume_args` | arguments appended (after `args`) when a session of this profile is resumed after a daemon restart and there is no agent session id to resume; e.g. `["--continue"]`. The catalog sets it for opencode (`--continue`: the project's last session). claude and codex resume by id and only use it as a fallback. Not shown in the app's editor (saving from the app keeps it) |

### Merge rules

- **Entry with a catalog agent's id**: changes only the fields you set; the rest (including
  `setup_hooks`) is inherited. An empty or omitted field inherits; `setup_hooks: none` disables the
  integration; `args: []` clears the inherited arguments. A catalog entry can't use `extends`.
- **New id without `extends`**: a custom profile; `cmd` is required.
- **New id with `extends: <id>`**: a variant. It inherits everything from the already-resolved
  parent (your changes included) and its own fields are applied on top; `args`, `env` and
  `attention_patterns` **replace** the parent's (they aren't merged). A variant's default name is
  its id.
- **Order**: file entries first, by `order` or by position; then the catalog agents the file
  doesn't mention, in catalog order. The first enabled, installed one is what ⌘N launches. An entry
  with just `- id: x` pins a catalog agent's position.
- **Errors**: an error in one entry (invalid regex, unknown or cyclic `extends`, missing `cmd`,
  unknown `setup_hooks`…) only disables that entry; it shows up in `agenterm profiles` and in
  **Settings**. Structural errors (invalid YAML, empty or duplicate id) invalidate the whole file:
  commands that read it fail until you fix it.

```yaml
version: 1
profiles:
  - id: claude-yolo           # variant
    name: Claude (no permissions)
    extends: claude
    args: ["--dangerously-skip-permissions"]
  - id: claude
    env: {CLAUDE_CODE_USE_BEDROCK: "1"}
  - id: gemini
    enabled: false
  - id: mine                  # custom profile
    cmd: $HOME/bin/my-agent
    args: []
    env: {}
    status: heuristic         # hooks | heuristic
    setup_hooks: none         # claude | codex | none
    idle_seconds: 3
    attention_patterns:
      - '\(y/n\)'
```

### Editing from the app

**Settings → Agents** (⌘,, the ⚙ in the sidebar or the **agenterm** app menu) lets you enable and
disable agents, reorder them (drag or ▲▼), edit them (name, command, arguments, `KEY=value`
environment, status mode, hook integration, idle seconds and attention patterns), duplicate one as a
variant, add a custom agent, delete custom profiles, reset modified catalog agents and rescan.

When the app saves (RPC `profile.save`, `profile.delete`, `profile.reorder`), the file is rewritten
in **canonical form**: one entry per profile in menu order, only with the fields that differ from
what's inherited (unchanged catalog agents become `- id: x`) and without `order`. **Comments are not
preserved.** The file is re-read on every `spawn` and every listing, so you can also edit it by
hand.

## `settings.yaml`

Every key is optional; missing ones take the default value.

| key | default | |
|---|---|---|
| `ui.language` | `auto` | app language: `auto`, `en` or `es` (see [Language](#language)) |
| `ui.appearance` | `system` | the app's colors (sidebar, title bars, Settings): `system` follows macOS light/dark, or `dark` / `light`. Terminals keep their own `terminal.theme` |
| `terminal.font_family` | `"SF Mono", Menlo, Monaco, monospace` | a CSS font-family list; the first installed font is used. Empty: the default |
| `terminal.font_size` | `13` | points, clamped to 8–32. ⌘= / ⌘- / ⌘⌥0 change it (and save it) from anywhere in the app |
| `terminal.line_height` | `1.15` | a multiple of the font size, 1.0–2.0 |
| `terminal.cursor_style` | `block` | `block`, `bar` or `underline` |
| `terminal.theme` | `bright-lights` | built-in palette, each named after an album. Dark: `bright-lights` (*Turn On the Bright Lights*), `room-on-fire` (*Room on Fire*), `is-this-it` (*Is This It*), `funeral` (*Funeral*), `silent-alarm` (*Silent Alarm*), `z` (*Z*). Light: `it-still-moves` (*It Still Moves*), `alphabetical` (*Alphabetical*), `hot-fuss` (*Hot Fuss*). Retired names still work and pick the closest palette: `agenterm-dark` → `bright-lights`, `agenterm-light` → `alphabetical`, `solarized-dark` → `z`, `solarized-light` → `hot-fuss`, `dracula` → `funeral`, `gruvbox-dark` → `is-this-it`, `nord` → `silent-alarm`. Unknown names fall back to `bright-lights` |
| `notify.enabled` | `true` | notifications (from the app and the daemon) |
| `notify.sound` | `true` | sound, only for approvals and questions |
| `notify.notify_on_done` | `true` | also notify when an agent finishes its turn (errors always notify) |
| `notify.notify_on_stall` | `true` | low-key notice when a turn looks stalled |
| `notify.fallback` | `none` | when native notifications can't be used (no `.app` bundle, no permission): `none` doesn't show them (`daemon.log` says why, once) and `osascript` sends them through AppleScript, which macOS shows as *Script Editor* (clicking opens Script Editor, not agenterm). `AGENTERM_NOTIFY=off` in the environment disables all notifications regardless. See [attention-and-activity.md](attention-and-activity.md#while-the-app-is-closed) |
| `activity.stall_seconds` | `180` | seconds without output or events before marking "stalled?"; `0` disables it |
| `git.editor` | `""` | for "Open in editor": a command on the `PATH` (`code`, `cursor -n`, `zed`), an absolute path or the name of a macOS app (`Visual Studio Code`, `Cursor.app`). Empty: `code` if it's on the `PATH`, otherwise Finder |
| `git.worktree_dir` | `""` | where to create worktrees (absolute path or `~/…`). Empty: `~/.agenterm/worktrees` (or `$AGENTERM_HOME/worktrees`) |
| `git.default_mode` | `worktree` | `worktree` or `same`: where the **+** menu (and ⌘N) starts a new agent in a project where you haven't chosen yet. Shells always start in the project folder. Before 0.7.0 the default was `same`; a file that says `same` keeps it |
| `sessions.restore` | `ask` | what a starting daemon does with the sessions that were live when the previous one stopped: `ask` (keep them `suspended` until you resume or discard them), `always` (resume them right away) or `never` (discard them). See [architecture.md](architecture.md#persistence-and-resume) |
| `messages.enabled` | `true` | messages between the sessions of a project (`agenterm msg`): when off, sending is refused and agent hooks hand nothing over. See [messages.md](messages.md) |
| `messages.auto_deliver` | `false` | a Claude session idle at its prompt is told about new messages without the **Deliver** click; never over text typed without sending, at most 5 times per session until you act on it again (and 20 per project every 10 minutes). See [messages.md](messages.md#automatic-delivery) |

```yaml
version: 1
ui:
  language: auto
  appearance: system
notify:
  enabled: true
  sound: true
  notify_on_done: true
  notify_on_stall: true
  fallback: none
activity:
  stall_seconds: 180
git:
  editor: ""
  worktree_dir: ""
  default_mode: worktree
sessions:
  restore: ask
messages:
  enabled: true
  auto_deliver: false
terminal:
  font_family: '"SF Mono", Menlo, Monaco, monospace'
  font_size: 13
  line_height: 1.15
  cursor_style: block
  theme: bright-lights
```

When it's read:

- `notify.*` is re-read on every notification (in the app and in the daemon).
- `activity.stall_seconds` is read when each session is launched: it affects new sessions.
- `git.worktree_dir` is read when each worktree is created (and when listing a project's worktrees).
- `git.editor` is read on every "Open in editor".
- `sessions.restore` is read when the daemon starts.
- `messages.enabled` is read on every message sent and every hook that could deliver one;
  `messages.auto_deliver` whenever a message arrives or a session goes idle.
- `ui.appearance` and `terminal.*` are read by the app when it loads (and when Settings opens);
  changes made in the app apply at once to every open terminal. A hand edit applies on the next
  reload of the window.

**Settings → Notifications** edits the `notify.*` keys and `activity.stall_seconds` (RPC
`settings.save`). Saving edits the file in place: unknown keys and comments **are preserved**, and
fields a client doesn't send keep their value. If the file can't be parsed, the defaults are used
(and this is logged); saving replaces it.

**Settings → Git** edits `git.editor`, `git.worktree_dir` and `git.default_mode`. **Settings →
General** edits `ui.language`, `sessions.restore`, `messages.enabled` and `messages.auto_deliver` (saved right away). **Settings →
Appearance** edits `ui.appearance` and `terminal.*`, with a live preview; changes apply to the open
terminals at once and are saved right away.

### Language

`ui.language` sets the app language:

- `auto` (default): the first of English/Spanish found in the macOS preferred languages, then in
  `LC_ALL` / `LC_MESSAGES` / `LANG` (`C`/`POSIX` are skipped); otherwise English;
- `en`: English;
- `es`: Spanish.

You can pick it in **Settings → General**; it localizes the app UI, its menu and notifications. CLI
output and daemon/RPC error messages are always in English; user-facing refusals carry stable error
codes so the app can show them in its language. Any other value is treated as `auto`.
