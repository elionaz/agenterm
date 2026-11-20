import { describe, expect, it } from 'vitest'
import { keyLabel, primary } from './platform'

describe('keyLabel', () => {
  it('leaves macOS labels alone', () => {
    expect(keyLabel('Press ⌘N', true)).toBe('Press ⌘N')
  })
  it('rewrites for Linux', () => {
    expect(keyLabel('Settings (⌘,)', false)).toBe('Settings (Ctrl+Shift+,)')
    expect(keyLabel('<kbd>⌘N</kbd>', false)).toBe('<kbd>Ctrl+Shift+N</kbd>')
    expect(keyLabel('⌘⌥0 resets', false)).toBe('Ctrl+Shift+Alt+0 resets')
    expect(keyLabel('⇧↩ or ⌘⇧G', false)).toBe('Shift+Enter or Ctrl+Shift+G')
    expect(keyLabel('⌘↩ sends', false)).toBe('Ctrl+Enter sends')
    expect(keyLabel('⌘1', false)).toBe('Ctrl+Shift+1')
  })
})

it('primary', () => {
  expect(primary({ metaKey: true, ctrlKey: false, shiftKey: false }, true)).toBe(true)
  expect(primary({ metaKey: false, ctrlKey: true, shiftKey: false }, false)).toBe(false)
  expect(primary({ metaKey: false, ctrlKey: true, shiftKey: true }, false)).toBe(true)
})
