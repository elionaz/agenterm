# Changelog

All notable changes to agenterm are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is 0.x, minor
releases may include breaking changes.

## [Unreleased]

### Changed

- **Versioned file formats**: `settings.yaml` and `profiles.yaml` get a `version` key
  (`profiles.yaml` becomes `version` + `profiles`), and `projects.json`, `worktrees.json` and
  `sessions.json` a `{"version": 1, …}` envelope. Older files are still read and get the version on
  their next save. A file written by a newer agenterm is refused with a clear error and never
  overwritten. **Going back to 0.3.x** after this needs the `.bak` copies: see
  [docs/compatibility.md](docs/compatibility.md#going-back-to-an-older-version).
- The daemon reports its **protocol version** in `daemon.ping`. A command an older daemon doesn't
  know fails with a message that says to restart it, and the app says when the daemon is newer than
  itself instead of offering to restart (which would downgrade it).

### Added

- [docs/compatibility.md](docs/compatibility.md): what stays stable between versions (CLI,
  `AGENTERM_*` variables, configuration files, hooks) and how formats and the protocol are versioned.

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

[Unreleased]: https://github.com/elionaz/agenterm/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/elionaz/agenterm/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/elionaz/agenterm/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/elionaz/agenterm/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/elionaz/agenterm/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/elionaz/agenterm/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/elionaz/agenterm/releases/tag/v0.1.0
