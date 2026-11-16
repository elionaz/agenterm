# Development

**English** · [Español](es/development.md)

Requirements: those in [README.md](../README.md#status-and-requirements) (Go, Node 24+ with npm and
the Wails v2 CLI). The rules for opening a pull request are in [CONTRIBUTING.md](../CONTRIBUTING.md).

## App in development mode

```sh
cd app
wails dev     # hot reload (frontend on Vite); also serves http://localhost:34115
wails build   # -> app/build/bin/agenterm.app
```

Frontend only:

```sh
cd app/frontend
npm ci
npm run check   # svelte-check
npm run build   # vite build
```

## Tests

```sh
gofmt -l .                       # must print nothing
go vet ./... && go test -race ./...
```

This is what CI runs (plus the frontend's `npm run check` and `npm run build`).

- One integration test brings up the daemon in a temporary `AGENTERM_HOME` and exercises spawn,
  attach/detach/reattach with scrollback, notify, events, kill and shutdown; another launches a
  shell in worktree mode on a temporary repo, makes changes by typing into it and checks
  `session.diff`, that `kill` keeps the worktree, the orphan after restarting the daemon, reopening
  it, the merge and `worktree.remove` (refusal and force).
- The `internal/gitx` tests use real repos in temporary directories with isolated git configuration
  (`GIT_CONFIG_NOSYSTEM=1`, empty `GIT_CONFIG_GLOBAL`, temporary `HOME`).

## Testing without touching your installation

Use your own `AGENTERM_HOME`. It must be a **short** path (`/tmp/…`): on macOS a unix socket path is
limited to about 104 bytes and the temporary folders under `/var/folders/…` go over it.

```sh
export AGENTERM_HOME=/tmp/agt
go run ./cmd/agenterm project add .
go run ./cmd/agenterm spawn agenterm shell
go run ./cmd/agenterm ls
go run ./cmd/agenterm stop --force      # stops only the /tmp/agt daemon

cd app && AGENTERM_HOME=/tmp/agt wails dev   # the app against that same daemon
```

Since `AGENTERM_HOME` takes precedence over `AGENTERM_SOCK`, this is also safe inside an agenterm
session: it doesn't touch the daemon hosting the session.

## Wails bindings

The TypeScript bindings for the methods in `app/app.go` live in `app/frontend/wailsjs/` and are
checked in. `wails dev` and `wails build` regenerate them; if you change an exported method's
signature and only want the bindings:

```sh
cd app && wails generate module
```

## Icon

The sources are in `assets/icon/`:

- `agenterm-icon.svg`: the square, full-bleed icon;
- `agenterm-icon-rounded.svg`: with rounded corners (the one in the README);
- `agenterm-icon-macos.svg`: with the macOS icon template's margin, which is the one the app uses.

Wails generates the `.app` icon from `app/build/appicon.png` (1024×1024). To regenerate it with
Inkscape 1.x:

```sh
inkscape assets/icon/agenterm-icon-macos.svg \
  --export-type=png --export-filename=app/build/appicon.png -w 1024 -h 1024
```

On macOS the Inkscape CLI isn't on the `PATH`; it's at
`/Applications/Inkscape.app/Contents/MacOS/inkscape`. Use solid colors on the edges (not
`stroke-opacity`): semi-transparent strokes add up where they overlap.

## Known limitations

- **macOS only.** The app, notifications and the Dock are macOS-only, and it's only tested on macOS
  (Apple Silicon). The transport is a unix socket; Windows isn't supported.
- **No signed releases** yet: you have to build from source.
- **Sessions don't survive a daemon restart** (or a computer restart): their processes die and the
  scrollback is lost. Worktrees are kept, as orphans.
- **Notifications with the app closed**: if the CLI started the daemon (it isn't inside the `.app`)
  or the bundle has no permission, `osascript` is used: the notification shows up as *Script Editor*
  and clicking it doesn't open agenterm.
- **codex** doesn't report when it starts working: "working" is decided by the heuristic and there's
  no "stalled?" warning.
- **"Stalled?"** only exists in sessions with hooks (Claude Code), and since Claude's spinner
  redraws every second, it mostly catches complete hangs, not slow tools.
- **Activity line in TUIs** that redraw the screen (opencode, codex…): it's the last line they drew,
  which isn't always the most informative.
- **Kiro CLI**: the binary name (`kiro-cli chat`) comes from its documentation and hasn't been
  verified against a real installation; you can fix it with `cmd:` in `profiles.yaml`.
- **`profiles.yaml` loses its comments** when the app saves it (it's rewritten in canonical form).
  `settings.yaml` keeps them.
- Merging a worktree **doesn't stash**: the main folder must be clean and on the base branch.
