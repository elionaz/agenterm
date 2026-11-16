// Appearance preferences as reactive state: the terminal font/theme
// (settings terminal.*) and the chrome's color scheme (ui.appearance).
// Both are cached in localStorage so a reload paints right away, before the
// daemon answers.
import { normPrefs, type TermPrefs, type TermPrefsInput } from './themes'

export type Appearance = 'system' | 'dark' | 'light'

const TERM_KEY = 'agenterm.terminal'
const APPEARANCE_KEY = 'agenterm.appearance'

function normAppearance(v: string | null | undefined): Appearance {
  return v === 'dark' || v === 'light' ? v : 'system'
}

function stored<T>(key: string, parse: (v: string | null) => T): T {
  try {
    return parse(localStorage.getItem(key))
  } catch {
    return parse(null)
  }
}

function store(key: string, v: string) {
  try {
    localStorage.setItem(key, v)
  } catch {
    /* storage unavailable */
  }
}

const state = $state({
  term: stored(TERM_KEY, (v) => {
    try {
      return normPrefs(v ? (JSON.parse(v) as TermPrefsInput) : null)
    } catch {
      return normPrefs(null)
    }
  }),
  appearance: stored(APPEARANCE_KEY, normAppearance),
})

export function termPrefs(): TermPrefs {
  return state.term
}

export function setTermPrefs(p: TermPrefsInput | undefined | null) {
  const next = normPrefs(p)
  if (JSON.stringify(next) === JSON.stringify(state.term)) return
  state.term = next
  store(TERM_KEY, JSON.stringify(next))
}

export function appearance(): Appearance {
  return state.appearance
}

/** Applies ui.appearance to the chrome (style.css reads data-appearance). */
export function setAppearance(v: string | null | undefined) {
  state.appearance = normAppearance(v)
  document.documentElement.dataset.appearance = state.appearance
  store(APPEARANCE_KEY, state.appearance)
}

/** Paints the cached appearance before the app mounts. */
export function applyStoredAppearance() {
  document.documentElement.dataset.appearance = state.appearance
}
