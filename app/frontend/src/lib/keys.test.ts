import { describe, expect, it } from 'vitest'
import { SHIFT_ENTER, isShiftEnter } from './keys'

const key = (over: Partial<KeyboardEvent>) => ({
  key: 'Enter', shiftKey: false, metaKey: false, ctrlKey: false, altKey: false, isComposing: false, ...over,
})

describe('isShiftEnter', () => {
  it('matches Shift+Enter', () => {
    expect(isShiftEnter(key({ shiftKey: true }))).toBe(true)
  })

  it('leaves Enter and other combinations alone', () => {
    expect(isShiftEnter(key({}))).toBe(false)
    expect(isShiftEnter(key({ shiftKey: true, metaKey: true }))).toBe(false)
    expect(isShiftEnter(key({ shiftKey: true, ctrlKey: true }))).toBe(false)
    expect(isShiftEnter(key({ shiftKey: true, altKey: true }))).toBe(false)
    expect(isShiftEnter(key({ key: 'a', shiftKey: true }))).toBe(false)
  })

  it('does not interfere with an IME composition', () => {
    expect(isShiftEnter(key({ shiftKey: true, isComposing: true }))).toBe(false)
  })

  it('sends a line feed, not a carriage return', () => {
    expect(SHIFT_ENTER).toBe('\n')
  })
})
