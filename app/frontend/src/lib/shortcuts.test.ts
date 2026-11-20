import { describe, expect, it } from 'vitest'
import { shortcutOf } from './shortcuts'

const ev = (code: string, key: string, mods: Partial<KeyboardEvent> = {}) =>
  ({ code, key, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...mods }) as KeyboardEvent

describe('shortcutOf', () => {
  it('macOS: ⌘ combos', () => {
    expect(shortcutOf(ev('KeyN', 'n', { metaKey: true }), true)).toEqual({ kind: 'new' })
    expect(shortcutOf(ev('KeyD', 'D', { metaKey: true, shiftKey: true }), true)).toEqual({ kind: 'split', dir: 'col' })
    expect(shortcutOf(ev('KeyN', 'n', { ctrlKey: true, shiftKey: true }), true)).toBeNull()
    expect(shortcutOf(ev('KeyF', 'F', { metaKey: true, shiftKey: true }), true)).toEqual({ kind: 'history' })
    expect(shortcutOf(ev('KeyF', 'f', { metaKey: true }), true)).toEqual({ kind: 'find' })
  })
  it('Linux: Ctrl+Shift combos, plain Ctrl stays the shell', () => {
    const cs = { ctrlKey: true, shiftKey: true }
    expect(shortcutOf(ev('KeyN', 'N', cs), false)).toEqual({ kind: 'new' })
    expect(shortcutOf(ev('KeyD', 'D', cs), false)).toEqual({ kind: 'split', dir: 'row' })
    expect(shortcutOf(ev('KeyE', 'E', cs), false)).toEqual({ kind: 'split', dir: 'col' })
    expect(shortcutOf(ev('Digit3', '#', cs), false)).toEqual({ kind: 'session', n: 3 })
    expect(shortcutOf(ev('Digit0', ')', cs), false)).toEqual({ kind: 'grid' })
    expect(shortcutOf(ev('Comma', '<', cs), false)).toEqual({ kind: 'settings' })
    expect(shortcutOf(ev('KeyH', 'H', cs), false)).toEqual({ kind: 'history' })
    expect(shortcutOf(ev('Equal', '+', cs), false)).toEqual({ kind: 'font', delta: 1 })
    expect(shortcutOf(ev('PageDown', 'PageDown', cs), false)).toEqual({ kind: 'moveSession', delta: 1 })
    expect(shortcutOf(ev('ArrowLeft', 'ArrowLeft', { ...cs, altKey: true }), false)).toEqual({ kind: 'movePane', toward: 'left' })
    expect(shortcutOf(ev('KeyC', 'c', { ctrlKey: true }), false)).toBeNull()
    expect(shortcutOf(ev('KeyW', 'w', { ctrlKey: true }), false)).toBeNull()
    expect(shortcutOf(ev('KeyN', 'n', { metaKey: true }), false)).toBeNull()
  })
})
