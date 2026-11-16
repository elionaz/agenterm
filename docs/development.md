# Development

**English** · [Español](es/development.md)

Requirements: those in [README.md](../README.md#build-from-source) (Go, Node 24+ with npm and
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
npm run check   # svelte-check + scripts/check-i18n.ts (needs Node 22.18+)
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
- `internal/daemon/persist_test.go` covers persistence: a shell session writes output, the daemon
  stops (`stop --force` and cancellation), a new daemon on the same `AGENTERM_HOME` lists it as
  `suspended` with its scrollback replayed, resumes it (same folder, "session restored" line) and
  discards another; the `always`/`never` policies; a fake `setup_hooks: claude` script that
  checks `--resume <session_id>` comes after the `--settings` injection; failed resumes and a
  corrupt `sessions.json`. No real agent is ever launched.
- The `internal/gitx` tests use real repos in temporary directories with isolated git configuration
  (`GIT_CONFIG_NOSYSTEM=1`, empty `GIT_CONFIG_GLOBAL`, temporary `HOME`).
- `go test` never posts real notifications: test daemons get a no-op or recording notifier (a
  `daemon.Config` without one posts nothing), the app tests use a fake desktop, and the `app`,
  `internal/daemon` and `internal/daemoncmd` tests set `AGENTERM_NOTIFY=off` in `TestMain`. Keep it
  that way in new tests.

## Testing without touching your installation

Use your own `AGENTERM_HOME`, and set `AGENTERM_NOTIFY=off`. That's the rule for **any** test or
demo instance (daemon, app or CLI that may auto-start a daemon):

- `AGENTERM_HOME` must be a **short** path (`/tmp/…`): on macOS a unix socket path is limited to
  about 104 bytes and the temporary folders under `/var/folders/…` go over it.
- `AGENTERM_NOTIFY=off` disables every desktop notification of that app and daemon (native and the
  `osascript` fallback), whatever its `settings.yaml` says, so test sessions don't pop up
  notifications (or Script Editor) on your desktop. The daemon inherits it from whoever starts it.

```sh
export AGENTERM_HOME=/tmp/agt AGENTERM_NOTIFY=off
go run ./cmd/agenterm project add .
go run ./cmd/agenterm spawn agenterm shell
go run ./cmd/agenterm ls
go run ./cmd/agenterm stop --force      # stops only the /tmp/agt daemon

cd app && AGENTERM_HOME=/tmp/agt AGENTERM_NOTIFY=off wails dev   # the app against that same daemon
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

## Releasing

The version has a single source of truth: the git tag `vX.Y.Z`. Release builds inject it with
`-ldflags -X` into `internal/version` (what `agenterm version` and the app's About panel show) and
write it into the bundle's `Info.plist` (`CFBundleShortVersionString` and `CFBundleVersion`).
`productVersion` in `app/wails.json` is only what plain `wails build` uses; builds without ldflags
report `dev`.

1. Move the `[Unreleased]` entries of [CHANGELOG.md](../CHANGELOG.md) into a `## [X.Y.Z] - date`
   section (and update the links at the bottom), and merge that to `main`.
2. Tag `main` and push the tag:

   ```sh
   git switch main && git pull
   git tag -a v0.1.0 -m "agenterm 0.1.0"
   git push origin v0.1.0
   ```

3. `.github/workflows/release.yml` runs on the tag (macOS runner) and:
   - builds everything with `scripts/release-local.sh <version>` (the same script you run locally):
     the universal app (`wails build -platform darwin/universal`), the universal CLI (cgo, `lipo`)
     bundled at `agenterm.app/Contents/Helpers/agenterm`, and the Linux CLI (`CGO_ENABLED=0`);
   - embeds Sparkle (see [In-app updates](#in-app-updates-sparkle)) and signs it inside-out
     before the rest;
   - signs the CLI and then the app with the Developer ID, hardened runtime, a secure timestamp and
     `app/build/darwin/entitlements.plist`, if the certificate secrets exist; otherwise it keeps an
     ad-hoc signature, warns, and the release notes say it's an unsigned build;
   - notarizes the zip and then the dmg with `notarytool` and staples both, if the App Store Connect
     secrets exist too; a rejected submission fails the job and prints the `notarytool log`;
   - packages `agenterm-<v>-macos-universal.{zip,dmg}`, `agenterm-<v>-darwin-universal.tar.gz`,
     `agenterm-<v>-linux-{amd64,arm64}.tar.gz` and `checksums.txt`, and uploads them to the run;
   - if `SPARKLE_PRIVATE_ED_KEY` exists, EdDSA-signs the (stapled) zip and writes `appcast.xml`;
   - creates the GitHub release `vX.Y.Z` with the CHANGELOG section as notes (or generated notes if
     there's none) and every artifact attached, `appcast.xml` included when the build is notarized;
   - if `HOMEBREW_TAP_TOKEN` exists, renders `packaging/homebrew/agenterm.rb.tmpl` with the version
     and the zip's sha256 (`scripts/render-cask.sh`) and pushes it to `Casks/agenterm.rb` in
     [elionaz/homebrew-tap](https://github.com/elionaz/homebrew-tap) as "agenterm X.Y.Z".

### Secrets

Set them in the repo's *Settings → Secrets and variables → Actions*, or with `gh secret set`. All of
them are optional: without them the release is still published, unsigned, and the cask isn't
updated.

| secret | used for |
|---|---|
| `MACOS_CERT_P12_BASE64` | the Developer ID Application certificate and its private key (`.p12`), base64 |
| `MACOS_CERT_PASSWORD` | the `.p12`'s password |
| `MACOS_SIGN_IDENTITY` | optional: the identity name, e.g. `Developer ID Application: Jane Doe (TEAMID)`; by default the first Developer ID Application identity in the `.p12` |
| `APPLE_API_KEY_ID` | App Store Connect API key id (notarization) |
| `APPLE_API_ISSUER_ID` | App Store Connect issuer id (notarization) |
| `APPLE_API_KEY_P8_BASE64` | the API key's `.p8` file, base64 (notarization) |
| `HOMEBREW_TAP_TOKEN` | a GitHub token that can push to `elionaz/homebrew-tap` |
| `SPARKLE_PRIVATE_ED_KEY` | Sparkle's EdDSA private key, as `generate_keys -x` exports it: signs the update zip and enables `appcast.xml` |
| `SPARKLE_PUBLIC_ED_KEY` | its public key (`SUPublicEDKey`); a repository **variable** (it isn't secret), or a secret |

Without `SPARKLE_PUBLIC_ED_KEY` the app is built with its updater off; without
`SPARKLE_PRIVATE_ED_KEY` there's no `appcast.xml`, so installed apps aren't offered the release.
Both cases only warn.

**Developer ID Application certificate** (needs the Account Holder role in the Apple Developer
program):

1. In *Keychain Access → Certificate Assistant → Request a Certificate From a Certificate
   Authority…*, enter your email, choose "Saved to disk" and save the `.certSigningRequest`.
2. In [developer.apple.com → Certificates](https://developer.apple.com/account/resources/certificates/list),
   click **+**, choose **Developer ID Application** (the G2 Sub-CA), upload the request and download
   the `.cer`. Double-click it to add it to your login keychain.
3. In Keychain Access → *My Certificates*, right-click "Developer ID Application: … (TEAMID)" (it
   must show its private key) → **Export…** → `.p12` with a strong password.
4. Store it:

   ```sh
   base64 -i DeveloperID.p12 | gh secret set MACOS_CERT_P12_BASE64
   gh secret set MACOS_CERT_PASSWORD          # paste the password
   security find-identity -v -p codesigning   # the exact name, for MACOS_SIGN_IDENTITY
   ```

   Then delete the exported `.p12`.

**App Store Connect API key** (notarization):

1. In [App Store Connect](https://appstoreconnect.apple.com/access/integrations/api) → *Users and
   Access → Integrations → App Store Connect API → Team Keys*, generate a key with the
   **Developer** role.
2. Download `AuthKey_<KEYID>.p8` (Apple only lets you download it once) and note the **Key ID** and
   the **Issuer ID** shown above the list.
3. Store them:

   ```sh
   base64 -i AuthKey_XXXXXXXXXX.p8 | gh secret set APPLE_API_KEY_P8_BASE64
   gh secret set APPLE_API_KEY_ID --body XXXXXXXXXX
   gh secret set APPLE_API_ISSUER_ID --body 00000000-0000-0000-0000-000000000000
   ```

**`HOMEBREW_TAP_TOKEN`**: a
[fine-grained personal access token](https://github.com/settings/personal-access-tokens/new) with
*Repository access → Only select repositories → elionaz/homebrew-tap* and the permission
*Contents: Read and write* (nothing else). Store it with `gh secret set HOMEBREW_TAP_TOKEN` and
renew it before it expires.

### Dry runs

Locally (macOS, with the requirements in the README):

```sh
scripts/release-local.sh 0.1.0        # -> dist/ (gitignored), ad-hoc signed
codesign --verify --deep --strict --verbose=2 dist/work/agenterm.app
lipo -info dist/work/agenterm.app/Contents/MacOS/agenterm
scripts/render-cask.sh 0.1.0 "$(shasum -a 256 dist/agenterm-0.1.0-macos-universal.zip | cut -d' ' -f1)"
```

To try the signing path, pass an identity from your keychain: `SIGN_IDENTITY="Developer ID
Application: …" scripts/release-local.sh 0.1.0`. Notarization also needs `NOTARY_KEY_PATH` (the
`.p8`), `NOTARY_KEY_ID` and `NOTARY_ISSUER_ID`.

On GitHub: *Actions → Release → Run workflow* with a version. It builds, signs and notarizes (if the
secrets exist) and uploads the artifacts to the run (`appcast.xml` included), but doesn't create a
release or touch the tap.

To try the appcast locally without your real key, use a throwaway ed25519 pair (OpenSSL 3, no
keychain involved):

```sh
k=$(mktemp -d)
openssl genpkey -algorithm ed25519 -out "$k/key.pem"
export SPARKLE_PRIVATE_ED_KEY=$(openssl pkey -in "$k/key.pem" -outform DER | tail -c 32 | base64)
export SPARKLE_PUBLIC_ED_KEY=$(openssl pkey -in "$k/key.pem" -pubout -outform DER | tail -c 32 | base64)
scripts/release-local.sh 0.2.0-dev
xmllint --noout dist/appcast.xml
```

### In-app updates (Sparkle)

The app embeds [Sparkle 2](https://sparkle-project.org): `scripts/fetch-sparkle.sh` downloads the
pinned release (`SPARKLE_VERSION`), checks it against the SHA-256 GitHub publishes for it and
caches it in `.cache/sparkle/` (gitignored). `release-local.sh` then:

- builds the app with `-tags sparkle` (`app/updater_sparkle_darwin.{go,m}`; every other build,
  `wails dev` and tests included, gets `app/updater_stub.go`), weak-linking `Sparkle.framework`
  through `@rpath` = `Contents/Frameworks`;
- copies the framework to `agenterm.app/Contents/Frameworks/` and signs, in this order and without
  `--deep`, as Sparkle documents for Developer ID apps: `XPCServices/Installer.xpc`,
  `XPCServices/Downloader.xpc` (`--preserve-metadata=entitlements`), `Autoupdate`, `Updater.app`,
  the framework; then the CLI and the app as before;
- sets `SUPublicEDKey` in `Info.plist` (the template has `SUFeedURL` =
  `https://github.com/elionaz/agenterm/releases/latest/download/appcast.xml`, daily automatic checks
  and automatic updates allowed), or removes it when `SPARKLE_PUBLIC_ED_KEY` is empty: the app then
  keeps the updater off;
- after notarizing and stapling the zip, signs it with `bin/sign_update --ed-key-file` (the key goes
  through a 0600 temp file, never the command line or the log), verifies the signature, checks it
  against `SPARKLE_PUBLIC_ED_KEY` with OpenSSL and writes `dist/appcast.xml`
  (`scripts/render-appcast.sh`: a single item, the release's zip, `minimumSystemVersion` 13.0 and
  the CHANGELOG section as release notes).

The release uploads `appcast.xml` as an asset, so `releases/latest/download/appcast.xml` always
points at the newest release. Unnotarized builds don't publish it: Sparkle would install an app
Gatekeeper then refuses to open.

The app turns its updater off, and says why in **Settings → General → Updates** and in **Check for
Updates…**, when it's a development build, when it has no `SUPublicEDKey`, and when it was
installed by Homebrew: the bundle resolves into a `Caskroom`, or
`<brew prefix>/Caskroom/agenterm/<bundle version>` exists (brew moves the `.app` to `/Applications`
and keeps that folder as its record), for the prefixes `$HOMEBREW_PREFIX`, `/opt/homebrew` and
`/usr/local`. That's also why the cask doesn't declare `auto_updates true`: brew remains the only
updater for brew installs and `brew outdated` keeps reporting new versions.

**Generating the key pair** (once, on your Mac; the private key never goes into the repo):

```sh
SPARKLE_DIR=$(scripts/fetch-sparkle.sh)
"$SPARKLE_DIR/bin/generate_keys" --account agenterm          # creates it in your login keychain, prints the public key
"$SPARKLE_DIR/bin/generate_keys" --account agenterm -x sparkle-private.txt
gh secret set SPARKLE_PRIVATE_ED_KEY < sparkle-private.txt
gh variable set SPARKLE_PUBLIC_ED_KEY --body "$("$SPARKLE_DIR/bin/generate_keys" --account agenterm -p)"
rm sparkle-private.txt
```

Keep a backup of the private key (the keychain item, or the exported text in a password manager):
if it's lost, installed apps can't verify updates any more and users have to download the next
version by hand.

**Rotating the key**: generate a new pair (another `--account`), then publish one release signed
with the **old** private key that ships the **new** public key: keep `SPARKLE_PRIVATE_ED_KEY` as
is, set the variable `SPARKLE_PUBLIC_ED_KEY` to the new key and `SPARKLE_KEY_ROTATION=1` (a
repository variable; it skips the check that both keys are a pair). Once that release is out,
replace `SPARKLE_PRIVATE_ED_KEY` with the new private key and delete `SPARKLE_KEY_ROTATION`. If the
old key leaked, do the same right away; if it's lost, there's no in-app path: announce that the
next version has to be installed by hand.

**The daemon after an update.** Sparkle replaces and relaunches the app, but the daemon (started
from the old executable) keeps running, by design, with the sessions. On every (re)connection the
app compares `daemon.ping`'s `version` (absent in daemons before 0.2.0) with its own and, if they
differ, shows a banner with **Restart now**: `RestartDaemon` stops the daemon with force (live
sessions are recorded as suspended), starts the new one and, with `sessions.restore: ask`, resumes
them all (with `always` the daemon does it itself). Development builds (`dev`) never show it.

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
- **Upgrading doesn't restart the daemon**: after installing a new version, a daemon that was
  already running keeps the old code until `agenterm stop` (or a computer restart).
- The **Linux CLI** builds and is published, but is untested; there's no app or notifications there.
- **A daemon restart (or a computer restart) kills the processes.** Sessions come back as
  `suspended`, with their scrollback, and are respawned on resume: Claude Code and codex continue
  their conversation by id, opencode with `--continue` (the project's *last* session, which may not
  be the same one if several run in one folder), other agents only with `resume_args`. A shell
  comes back in its starting folder, without whatever was running in it. codex only reports its
  thread id after the first turn, so a codex session that never finished a turn starts fresh.
- **Scrollback persistence costs disk writes**: the whole ring (up to 8 MiB) is rewritten at most
  every 2 s per session while it produces output.
- **Notifications with the app closed**: if the daemon isn't inside the `.app` (a CLI installed
  outside the bundle started it) or the bundle has no permission, notifications are not shown
  (`daemon.log` says why). With `notify.fallback: osascript` they are sent through `osascript`
  instead: they show up as *Script Editor*, and clicking one opens Script Editor, not agenterm.
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
