// App keyboard shortcuts (⌘ combos). The terminal ignores every key this
// recognizes, so none of them reaches the PTY.
import type { Dir } from './panes'

export type Toward = 'left' | 'right' | 'up' | 'down'

export type Shortcut =
  | { kind: 'settings' }
  | { kind: 'grid' }
  | { kind: 'session'; n: number }
  | { kind: 'new' }
  | { kind: 'split'; dir: Dir }
  | { kind: 'closePane' }
  | { kind: 'cyclePane'; delta: 1 | -1 }
  | { kind: 'movePane'; toward: Toward }
  | { kind: 'find' }
  | { kind: 'findNext'; backwards: boolean }
  | { kind: 'font'; delta: 1 | -1 | 0 }
  | { kind: 'moveSession'; delta: 1 | -1 }

const arrows: Record<string, Toward> = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' }

export function shortcutOf(e: KeyboardEvent): Shortcut | null {
  if (!e.metaKey || e.ctrlKey) return null
  if (e.altKey) {
    if (arrows[e.key] && !e.shiftKey) return { kind: 'movePane', toward: arrows[e.key] }
    if (e.code === 'Digit0' && !e.shiftKey) return { kind: 'font', delta: 0 }
    return null
  }
  // Font size by character, so ⌘+ / ⌘- work on any layout (on Spanish
  // ones "=" is ⇧0 and "+" sits where "]" is on US keyboards).
  if (e.key === '=' || e.key === '+') return { kind: 'font', delta: 1 }
  if (e.key === '-') return { kind: 'font', delta: -1 }
  if (/^Digit[0-9]$/.test(e.code)) {
    const n = Number(e.code.slice(5))
    return n === 0 ? { kind: 'grid' } : { kind: 'session', n }
  }
  const k = e.key.toLowerCase()
  if (k === 'g') return { kind: 'findNext', backwards: e.shiftKey }
  if (k === 'd') return { kind: 'split', dir: e.shiftKey ? 'col' : 'row' }
  if (e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
    return { kind: 'moveSession', delta: e.key === 'ArrowUp' ? -1 : 1 }
  }
  if (e.shiftKey) return null
  switch (k) {
    case ',':
      return { kind: 'settings' }
    case 'n':
      return { kind: 'new' }
    case 'w':
      return { kind: 'closePane' }
    case 'f':
      return { kind: 'find' }
    case '[':
      return { kind: 'cyclePane', delta: -1 }
    case ']':
      return { kind: 'cyclePane', delta: 1 }
  }
  if (e.code === 'BracketLeft') return { kind: 'cyclePane', delta: -1 }
  if (e.code === 'BracketRight') return { kind: 'cyclePane', delta: 1 }
  return null
}
