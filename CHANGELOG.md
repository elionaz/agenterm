# Changelog

All notable changes to agenterm are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is 0.x, minor
releases may include breaking changes.

## [Unreleased]

### Added

- **Agent sessions in new pull requests.** *Create pull request* in Changes can append an *Agent
  sessions* section to the description: the agents that worked on the branch, their model and,
  for Claude Code, the prompts typed in each session, with secrets (API keys, tokens, private
  keys, `password=…`) replaced by `[redacted]`. On by default, with a preview; untick it to leave
  it out. Prompts go in code blocks, so their links and @mentions don't render or notify.
- **Hermes Agent** in the catalog (`hermes --cli`). It shows up in the **+** menu only when the
  `hermes` binary is installed and the profile stays enabled. The approval line
  `[o]nce | [s]ession` marks the session as waiting. After a daemon restart, resume appends
  `-c` (the latest session in that directory). `args`, `resume_args` and `enabled: false`
  change or hide it; agenterm does not write `~/.hermes`.

### Fixed

- **Screenshots dragged from the floating thumbnail onto a terminal** reach Claude Code again. macOS
  hands over a temporary file and deletes it as soon as the drag ends, so the typed path no longer
  existed and Claude Code dropped the image without a word (Cursor Agent, which reads the path
  later, seemed to work). agenterm now keeps the file the moment it is dropped, in
  `~/.agenterm/drops/` (kept for a day), and types that path. Files dropped from Finder are typed
  as before.

## [0.7.0] - 2026-11-18

### Added

- **The model of each session** next to its name in the sidebar, the title bar and split panes
  (*Opus 5.5*). Claude Code reports it through agenterm's hooks (the payload, or the end of the
  session's transcript), so a `/model` change shows up by the next turn; other agents show it when
  their profile's command line names it (`--model`, `-m`, `-c model=`). See
  [docs/attention-and-activity.md](docs/attention-and-activity.md#model).
- **The branch of every session in the sidebar**: worktree sessions keep their blue branch chip, and
  sessions in the project folder now show that folder's current branch in grey (it is shared with
  the other sessions there), also in the title bar.
- **Automatic delivery of messages** (Settings → General, `messages.auto_deliver`, off by default):
  a Claude session idle at its prompt is told about new messages without the **Deliver** click, so
  a reply that arrives after its turn ended doesn't wait for you. Never over text typed without
  sending, never while it waits for an approval, at most 5 times per session until you act on it
  again, and 20 per project every 10 minutes. See [docs/messages.md](docs/messages.md#automatic-delivery).

### Changed

- **New agents start in their own worktree by default** (`git.default_mode: worktree`), so agents
  working in parallel never share a branch or step on each other's changes; **Same folder** is one
  click away in the **+** menu. Shells always start in the project folder. A `settings.yaml` that
  already says `same` keeps it (change it in **Settings → Git**), and the per-project choice made
  before this version is no longer remembered. `agenterm spawn` is unchanged (`--worktree`).

## [0.6.0] - 2026-11-18

### Added

- **Project badges**: each project in the sidebar shows a small badge with its initials on a color
  picked from its id, also next to the title and in the overview of all sessions. Click the badge
  (or right-click the project) to rename the project, pick one of eight colors or put an emoji in
  place of the initials; `agenterm project set <project> --name/--color/--icon` does the same from
  the CLI. Notifications of running sessions use the new name.
- **Messages between the sessions of a project**: agents write to each other with
  `agenterm msg send <id|name|all>`, see who else is working with `agenterm msg peers` and wait for
  one with `agenterm wait <id>`. Claude Code gets its messages by itself through agenterm's hooks
  (an introduction at start, then new messages at each step, and a turn that would end goes on
  when one arrives); other agents read them with `agenterm msg read`. The new **Messages** tab
  lists a session's messages, lets you write to it and, for an agent idle at its prompt, **Deliver**
  types one fixed line asking it to read them (never the message). The sidebar shows an envelope while
  messages wait. The daemon decides who sent each message from the calling process, messages never
  leave their project, and **Settings → General** turns it off (`messages.enabled`). See
  [docs/messages.md](docs/messages.md).

## [0.5.0] - 2026-11-17

### Added

- **Pull requests on GitHub and GitLab in Changes**, for worktrees and for the branch the main
  folder is on: state, checks and review decision, the comments (line comments select their file),
  merging on the host (with the repository's methods and *delete the branch on the host*), and
  creating one with a title and description suggested from the commits. It runs through your own
  `gh` / `glab` login (agenterm stores no tokens); without it Changes says how to set it up and
  keeps the push-and-open-the-page flow.
- **Several agents in one project**: when an agent is already working in the project folder, the
  **+** menu, ⌘N and split panes pick a new worktree for the next one (each agent gets its own
  branch and changes) and say which session is there; **Same folder** is still one click away. Sessions that
  do share the folder say so in the Changes tab and the **±N** tooltip, and `agenterm spawn` prints
  a note on stderr. README and [docs/worktrees.md](docs/worktrees.md#several-agents-in-one-project)
  explain what the folder's sessions share.

### Changed

- **Choosing something in the sidebar closes Settings** (a session, a project, a new session, the
  overview), as × does, instead of leaving Settings covering it.
- **The overview has its own row** at the top of the sidebar (**Overview ⌘0**, highlighted while
  open) instead of a ▦ glyph in the title bar that sat off its line and said nothing. The project
  rows' overview button gets the same drawn icon.
- **After an update the app restarts the old daemon on its own** when no agent is in the middle of
  a turn: the sessions are suspended and resumed, conversations included. While an agent is
  working it still asks, with **Restart now**.

## [0.4.3] - 2026-11-17

### Added

- **Drop files on a terminal** to type their paths, escaped like Terminal.app does. Claude Code
  turns dropped images into attachments.

### Fixed

- Sessions resumed after a daemon restart (e.g. **Restart now**) no longer stay frozen in the
  app: their terminal kept the old screen and dropped every key, because the daemon told it the
  session had exited. The daemon now asks attached clients to reattach, and the app reattaches
  any terminal whose session came back with a new process.
- Shells no longer get junk like `11;rgb:0f0f/1414/2020` or `1R` typed at the prompt after the
  app reattaches: xterm answered terminal queries left in the replayed scrollback (by tools
  like `gh`) after input had already reopened. Input now waits until the replay is parsed.

### Changed

- The Homebrew cask declares `auto_updates true`: the app updates itself, so a plain
  `brew upgrade` leaves it alone. `brew upgrade --cask agenterm` (or `--greedy`) still upgrades it.

## [0.4.2] - 2026-11-17

### Changed

- **Homebrew installs update from the app**, like downloaded ones: the banner's **Update**
  installs and relaunches instead of copying `brew upgrade --cask agenterm` (which still works).
  Coming from 0.4.1 or earlier, run `brew upgrade --cask agenterm` one last time.

## [0.4.1] - 2026-11-16

### Added

- Five new terminal themes named after albums: *Is This It*, *Funeral*, *Silent Alarm* and *Z*
  (dark) and *Hot Fuss* (light).

### Changed

- Every terminal theme is now named after an album, and Settings shows only the album title.
  Solarized, Dracula, Gruvbox and Nord are gone; in `settings.yaml` their names keep working and
  pick the closest new palette: `solarized-dark` → `z`, `solarized-light` → `hot-fuss`,
  `dracula` → `funeral`, `gruvbox-dark` → `is-this-it`, `nord` → `silent-alarm`.

### Fixed

- In the light themes *It Still Moves* and *Alphabetical*, text in white or bright white was
  nearly invisible on the cream background. They're now readable grays, and bright black (dim
  text) is a darker gray so it still looks different from bright white.

## [0.4.0] - 2026-11-16

**Upgrade note:** 0.4.0 writes your settings, profiles and session files in a versioned format.
Going back to 0.3.x afterwards needs converting them back: see
[docs/compatibility.md](docs/compatibility.md#going-back-to-an-older-version).

### Added

- **Reorder by drag & drop**: drag projects in the sidebar, and sessions within their project
  (or move the selected one with ⌘⇧↑ / ⌘⇧↓). The order is kept by the daemon, so the CLI
  (`agenterm ls`, `project list`), ⌘1…⌘9 and restored sessions follow it.
- **Terminal themes named after indie albums**: *Turn On the Bright Lights* (Interpol, the new
  default), *Room on Fire* (The Strokes), *It Still Moves* (My Morning Jacket) and *Alphabetical*
  (Phoenix), with palettes taken from their covers; Settings shows the artist and year. They
  replace agenterm Dark and agenterm Light: `agenterm-dark` and `agenterm-light` in
  `settings.yaml` keep working as `bright-lights` and `alphabetical`.
- **Welcome screen** on first run (while there are no projects): the agent CLIs found (with
  Rescan), notification permission (**Allow notifications**, or **Open System Settings** if it was
  denied), whether the `agenterm` command is on your `PATH` (with the command to link the CLI that
  ships in the app), and **+ Add your first project**.
- README: how to **uninstall** (Homebrew and by hand), and what to check in `~/.agenterm/worktrees`
  before deleting your data.
- [docs/compatibility.md](docs/compatibility.md): what stays stable between versions (CLI,
  `AGENTERM_*` variables, configuration files, hooks) and how formats and the protocol are versioned.

### Changed

- **Versioned file formats**: `settings.yaml` and `profiles.yaml` get a `version` key
  (`profiles.yaml` becomes `version` + `profiles`), and `projects.json`, `worktrees.json` and
  `sessions.json` a `{"version": 1, …}` envelope. Older files are still read and get the version on
  their next save. A file written by a newer agenterm is refused with a clear error and never
  overwritten. **Going back to 0.3.x** after this needs converting the files back: see
  [docs/compatibility.md](docs/compatibility.md#going-back-to-an-older-version).
- The daemon reports its **protocol version** in `daemon.ping`. A command an older daemon doesn't
  know fails with a message that says to restart it, and the app says when the daemon is newer than
  itself instead of offering to restart (which would downgrade it).
- When the daemon can't start, the CLI and the app show why (for example, a file written by a newer
  agenterm) instead of only pointing at `daemon.log`.

### Fixed

- **Homebrew installs** can check for a new version from **Settings → General → Updates → Check
  now**, and check by themselves every 6 hours instead of once a day: a release published after
  you opened agenterm could go unnoticed for a day.

## [0.3.1] - 2026-11-16

### Fixed

- **⇧Enter** in the terminal inserts a new line in the agent's prompt (Claude Code, Codex…)
  instead of sending it. It sends a line feed (Ctrl-J); in a shell it still runs the command.

### Changed

- Development: the frontend has Vitest unit tests (`npm test`, run by CI), and the GitHub Actions
  workflows use actions that run on Node 24.

## [0.3.0] - 2026-11-16

### Added

- **Clickable URLs** in the terminal: ⌘-click opens a URL in the browser, both plain-text URLs
  and OSC 8 hyperlinks (`gh`, `ls --hyperlink`…). Only http and https; a plain click still selects.
- **Create pull request** for worktree sessions, in the Changes view and as
  `agenterm worktree pr [--no-open] <id>`: pushes the branch (`git push -u`, non-interactive, never
  forced) to the remote git would push to (`pushRemote`, `pushDefault`, upstream, `origin` or the
  only remote) and opens the GitHub compare page or the GitLab new merge request page. Other hosts
  get the push only. It doesn't need `gh`.
- After a successful **Merge into base**, the Changes view offers to remove the worktree and its
  branch.
- Credits in **About agenterm** (with the version) and a new *About* group in **Settings → General**
  with links to the author's GitHub profile and the repository.
- **Update banner**: when a new version is found, the window says so with **Update** (or
  **Restart to update** once it's downloaded; the daemon and its sessions keep running) and
  **Later**, which hides it for that version until it is downloaded. **Settings → General →
  Updates** shows it too. Scheduled checks use Sparkle's gentle reminders instead of popping its
  window while you work. Homebrew installs check the appcast daily and offer to copy
  `brew upgrade --cask agenterm`.

### Fixed

- Release notes in the update window: **bold**, links and `code` that wrap across lines of the
  changelog are now formatted too.

## [0.2.1] - 2026-11-16

### Fixed

- The release notes shown in the in-app update window now render Markdown links and *italics*
  (they appeared as raw `[text](url)` and `*text*`), and nothing inside `code` is formatted.

## [0.2.0] - 2026-11-16

### Added

- **In-app updates** with [Sparkle 2](https://sparkle-project.org): a daily background check,
  **Check for Updates…** in the app menu and an *Updates* section in **Settings → General**
  (version, automatic checks and downloads, "Check now"). Updates are EdDSA-signed and notarized.
  Homebrew installs keep updating with `brew upgrade --cask agenterm` (the app detects them and
  turns its updater off).
- After an update, a banner offers to **restart the background daemon** when it's still the
  previous version. The sessions running then are suspended and resumed whatever
  `sessions.restore` says (sessions already suspended stay so), the new daemon starts only once the
  old one has fully exited, and it asks first when agents are working. `daemon.ping` now reports
  the daemon's version.
- Release tooling: Sparkle is embedded and signed inside-out, and releases publish a signed
  `appcast.xml` (new optional `SPARKLE_PRIVATE_ED_KEY` secret and `SPARKLE_PUBLIC_ED_KEY`
  variable). Pre-release tags (`vX.Y.Z-suffix`) become GitHub pre-releases, not offered in-app nor
  to Homebrew, and, once in-app updates are on, a release without `appcast.xml` is not marked
  *Latest*.

### Fixed

- A second copy of the app with another bundle id, or started with its own `AGENTERM_HOME`, no
  longer hands off to the already open window and quits: the single-instance lock is now per
  bundle id and home.

## [0.1.1] - 2026-11-16

### Changed

- **No more Script Editor notifications by default.** When native notifications can't be used (the
  daemon isn't running from the `.app`, or notifications aren't allowed), they are no longer sent
  through AppleScript, which macOS attributes to Script Editor (and clicking one opened Script
  Editor). They are dropped and `daemon.log` says why, once. The new `notify.fallback: osascript`
  setting (**Settings → Notifications → Fallback via AppleScript**) brings the old behaviour back;
  the default is `none`.
- The CLI bundled in the app (`agenterm.app/Contents/Helpers/agenterm`, what the Homebrew cask puts
  on your `PATH`) now starts the daemon as the app's executable — on auto-start, with
  `agenterm daemon --detach` and in the foreground — so it gets native agenterm notifications and
  their clicks open the app. A CLI installed outside the bundle behaves as before.

### Fixed

- Test and demo instances can no longer pop up real notifications: `AGENTERM_NOTIFY=off` disables
  every desktop notification of the app and the daemon, whatever `settings.yaml` says, and
  `go test` never posts one (a daemon configured without a notifier now posts nothing instead of
  using osascript).

## [0.1.0] - 2026-11-16

First public release: a macOS desktop app (universal: Apple Silicon and Intel, macOS 13+) plus the
`agenterm` CLI and daemon.

### Added

- **Projects and sessions sidebar**: each project is a folder and each session a terminal
  (xterm.js) running an agent or a shell; sessions can be renamed (double-click or
  `agenterm rename`).
- **Agent catalog** with detection of the installed CLIs (Claude Code, codex, opencode, Gemini,
  Cursor, Copilot, aider, Amp, goose, Qwen, Crush, Kiro, shell), plus your own profiles and variants
  in `profiles.yaml`, editable from **Settings**.
- **Sessions outlive the app**: a daemon owns the PTYs and keeps the scrollback; the app and the CLI
  share it. Sessions are persisted: after a daemon restart they come back suspended with their
  scrollback and resume with one click (Claude Code and codex continue the same conversation).
- **Attention detection** (approvals, questions, finished turns, errors) through agent hooks and
  terminal signals, with native macOS notifications, a Dock badge and sidebar markers.
- **Live activity**: what each agent is doing, turn timers, a "stalled?" warning, progress bars
  (OSC 9;4) and a grid overview (⌘0).
- **Per-session git worktrees** with a **Changes** tab: diff, merge into the base branch and safe
  removal; also from the CLI (`agenterm diff`, `agenterm worktree …`).
- **Split panes** (⌘D / ⌘⇧D) with draggable dividers, kept per project.
- **Terminal search** (⌘F) and **appearance settings**: terminal themes, font, size, line height,
  cursor, and a light, dark or system app appearance.
- **English and Spanish** UI, following the macOS preferred languages or the one picked in
  **Settings → General**.
- `agenterm version` / `--version`.
- Release tooling: signed and notarized builds (when the credentials are configured), `.dmg` and
  `.zip` downloads, CLI archives for macOS and Linux, and a Homebrew cask
  (`brew install --cask elionaz/tap/agenterm`).

[Unreleased]: https://github.com/elionaz/agenterm/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/elionaz/agenterm/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/elionaz/agenterm/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/elionaz/agenterm/compare/v0.4.3...v0.5.0
[0.4.3]: https://github.com/elionaz/agenterm/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/elionaz/agenterm/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/elionaz/agenterm/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/elionaz/agenterm/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/elionaz/agenterm/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/elionaz/agenterm/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/elionaz/agenterm/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/elionaz/agenterm/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/elionaz/agenterm/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/elionaz/agenterm/releases/tag/v0.1.0
