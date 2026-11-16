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

A YAML list. Each entry has a required, unique `id` and is **layered over** the catalog:

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
| `notify.enabled` | `true` | notifications (from the app and the daemon) |
| `notify.sound` | `true` | sound, only for approvals and questions |
| `notify.notify_on_done` | `true` | also notify when an agent finishes its turn (errors always notify) |
| `notify.notify_on_stall` | `true` | low-key notice when a turn looks stalled |
| `activity.stall_seconds` | `180` | seconds without output or events before marking "stalled?"; `0` disables it |
| `git.editor` | `""` | for "Open in editor": a command on the `PATH` (`code`, `cursor -n`, `zed`), an absolute path or the name of a macOS app (`Visual Studio Code`, `Cursor.app`). Empty: `code` if it's on the `PATH`, otherwise Finder |
| `git.worktree_dir` | `""` | where to create worktrees (absolute path or `~/…`). Empty: `~/.agenterm/worktrees` (or `$AGENTERM_HOME/worktrees`) |
| `git.default_mode` | `same` | `same` or `worktree`: what the **+** menu (and ⌘N) suggests in a project where you haven't chosen yet |

```yaml
ui:
  language: auto
notify:
  enabled: true
  sound: true
  notify_on_done: true
  notify_on_stall: true
activity:
  stall_seconds: 180
git:
  editor: ""
  worktree_dir: ""
  default_mode: same
```

When it's read:

- `notify.*` is re-read on every notification (in the app and in the daemon).
- `activity.stall_seconds` is read when each session is launched: it affects new sessions.
- `git.worktree_dir` is read when each worktree is created (and when listing a project's worktrees).
- `git.editor` is read on every "Open in editor".

**Settings → Notifications** edits the `notify.*` keys and `activity.stall_seconds` (RPC
`settings.save`). Saving edits the file in place: unknown keys and comments **are preserved**, and
fields a client doesn't send keep their value. If the file can't be parsed, the defaults are used
(and this is logged); saving replaces it.

**Settings → Git** edits `git.editor`, `git.worktree_dir` and `git.default_mode`.

### Language

`ui.language` sets the app language:

- `auto` (default): the first of English/Spanish found in the macOS preferred languages, then in
  `LC_ALL` / `LC_MESSAGES` / `LANG` (`C`/`POSIX` are skipped); otherwise English;
- `en`: English;
- `es`: Spanish.

You can pick it in **Settings → General**; it localizes the app UI, its menu and notifications. CLI
output and daemon/RPC error messages are always in English; user-facing refusals carry stable error
codes so the app can show them in its language. Any other value is treated as `auto`.
