// A newer release, as reactive state shared by the banner (App.svelte) and
// Settings → General → Updates. Fed by UpdaterState().available at load and
// the "updates.available" event; "Later" hides the banner for that version
// (remembered in localStorage).
import { ClipboardSetText } from '../../wailsjs/runtime/runtime'
import { api, type UpdateInfo } from './api'

export const BREW_UPGRADE = 'brew upgrade --cask agenterm'
const DISMISSED_KEY = 'agenterm.updateDismissed'

function dismissedVersion(): string {
  try {
    return localStorage.getItem(DISMISSED_KEY) ?? ''
  } catch {
    return ''
  }
}

const state = $state({
  info: null as UpdateInfo | null,
  dismissed: dismissedVersion(),
  copied: false,
})

let copiedTimer: number | undefined

/** The update found (null: none). */
export function availableUpdate(): UpdateInfo | null {
  return state.info
}

export function setAvailableUpdate(u: UpdateInfo | null | undefined) {
  state.info = u?.version ? u : null
}

/** Whether the banner shows (not dismissed for this version). */
export function updateBannerVisible(): boolean {
  return !!state.info && state.info.version !== state.dismissed
}

/** "Later": hides the banner until a newer version shows up. */
export function dismissUpdate() {
  if (!state.info) return
  state.dismissed = state.info.version
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

/** The update button: Homebrew copies the upgrade command; Sparkle installs
 * a downloaded update and relaunches, or shows its window for the update. */
export async function updateNow() {
  if (state.info?.brew) {
    await ClipboardSetText(BREW_UPGRADE)
    state.copied = true
    clearTimeout(copiedTimer)
    copiedTimer = window.setTimeout(() => (state.copied = false), 2500)
    return
  }
  await api.installUpdateNow()
}
