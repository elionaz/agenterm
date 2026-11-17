// A newer release, as reactive state shared by the banner (App.svelte) and
// Settings → General → Updates. Fed by UpdaterState().available at load and
// the "updates.available" event; "Later" hides the banner for that version
// and stage (a later "Restart to update" for it shows again), remembered in
// localStorage.
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { api, type UpdateInfo } from './api'

export const BREW_UPGRADE = 'brew upgrade --cask agenterm'
const DISMISSED_KEY = 'agenterm.updateDismissed'
/** How long the button says "Downloading…" before it can be tried again. */
const BUSY_MS = 15_000

function dismissedKey(): string {
  try {
    return localStorage.getItem(DISMISSED_KEY) ?? ''
  } catch {
    return ''
  }
}

const keyOf = (u: UpdateInfo) => `${u.version}${u.ready ? '|ready' : ''}`

const state = $state({
  info: null as UpdateInfo | null,
  dismissed: dismissedKey(),
  copied: false,
  /** Sparkle was still downloading the update when asked to show it. */
  downloading: false,
})

let copiedTimer: number | undefined
let busyTimer: number | undefined

/** The update found (null: none). */
export function availableUpdate(): UpdateInfo | null {
  return state.info
}

export function setAvailableUpdate(u: UpdateInfo | null | undefined) {
  state.info = u?.version ? u : null
  state.downloading = false
  clearTimeout(busyTimer)
}

/** Whether the banner shows (not dismissed for this version and stage). */
export function updateBannerVisible(): boolean {
  return !!state.info && keyOf(state.info) !== state.dismissed
}

/** "Later": hides the banner until a newer version (or this one is ready). */
export function dismissUpdate() {
  if (!state.info) return
  state.dismissed = keyOf(state.info)
  try {
    localStorage.setItem(DISMISSED_KEY, state.dismissed)
  } catch {
    /* storage unavailable */
  }
}

/** "Copy command" was clicked recently (Homebrew). */
export function upgradeCopied(): boolean {
  return state.copied
}

/** The update is still downloading in the background (the button waits). */
export function updateDownloading(): boolean {
  return state.downloading
}

/** The update button: Homebrew copies the upgrade command; Sparkle shows its
 * window for the update ("Install and Relaunch" once it's downloaded). */
export async function updateNow() {
  if (state.info?.brew) {
    await ClipboardSetText(BREW_UPGRADE)
    state.copied = true
    clearTimeout(copiedTimer)
    copiedTimer = window.setTimeout(() => (state.copied = false), 2500)
    return
  }
  if (await api.installUpdateNow()) return
  // Busy downloading: the "ready" event resets this; else allow a retry.
  state.downloading = true
  clearTimeout(busyTimer)
  busyTimer = window.setTimeout(() => (state.downloading = false), BUSY_MS)
}
