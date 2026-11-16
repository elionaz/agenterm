# Changelog

All notable changes to agenterm are documented in this file. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the version is 0.x, minor
releases may include breaking changes.

## [Unreleased]

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

[Unreleased]: https://github.com/elionaz/agenterm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/elionaz/agenterm/releases/tag/v0.1.0
