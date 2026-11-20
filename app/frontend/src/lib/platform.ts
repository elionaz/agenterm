// macOS or not (Linux): the app's shortcuts use ⌘ on macOS and Ctrl+Shift
// elsewhere, like Linux terminals do, so plain Ctrl+letter keeps reaching
// the shell (Ctrl+C, Ctrl+D, Ctrl+W…).

function detectMac(): boolean {
  if (typeof navigator === 'undefined') return true
  return /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent || '')
}

export const IS_MAC = detectMac()

const names: Record<string, string> = { '⌃': 'Ctrl', '⌥': 'Alt', '⇧': 'Shift', '⌘': 'Ctrl+Shift' }
const keys: Record<string, string> = { '↩': 'Enter', '⌫': 'Backspace' }

/**
 * A shortcut label for this platform: "⌘N" stays on macOS and becomes
 * "Ctrl+Shift+N" elsewhere (⌘↩ → Ctrl+Enter, ⌘⌥0 → Ctrl+Shift+Alt+0).
 * Works on whole sentences: every run of modifier glyphs and the key after
 * it is rewritten.
 */
export function keyLabel(text: string, mac = IS_MAC): string {
  if (mac) return text
  return text.replace(/([⌃⌥⇧⌘]+)(Space|[^\s⌃⌥⇧⌘<]?)/g, (_m, mods: string, key: string) => {
    const parts: string[] = []
    const add = (p: string) => p.split('+').forEach((x) => !parts.includes(x) && parts.push(x))
    // ⌘↩ (send) is Ctrl+Enter, as in Linux chat apps.
    if (mods === '⌘' && key === '↩') return 'Ctrl+Enter'
    for (const m of mods) add(names[m])
    const order = ['Ctrl', 'Shift', 'Alt']
    parts.sort((a, b) => order.indexOf(a) - order.indexOf(b))
    return key ? `${parts.join('+')}+${keys[key] ?? key.toUpperCase()}` : parts.join('+')
  })
}

/** The app's primary modifier held: ⌘ on macOS, Ctrl+Shift elsewhere. */
export function primary(e: Pick<KeyboardEvent, 'metaKey' | 'ctrlKey' | 'shiftKey'>, mac = IS_MAC): boolean {
  return mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && e.shiftKey && !e.metaKey
}
