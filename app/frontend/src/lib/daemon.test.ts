import { describe, expect, it } from 'vitest'
import { shouldAutoRestart } from './daemon'

describe('shouldAutoRestart', () => {
  const old = { mismatch: true, newer: false }

  it('restarts an older daemon when no agent is working', () => {
    expect(shouldAutoRestart(old, 0, false)).toBe(true)
  })

  it('asks instead while an agent is in a turn', () => {
    expect(shouldAutoRestart(old, 1, false)).toBe(false)
  })

  it('tries only once per run', () => {
    expect(shouldAutoRestart(old, 0, true)).toBe(false)
  })

  it('leaves a current or newer daemon alone', () => {
    expect(shouldAutoRestart({ mismatch: false, newer: false }, 0, false)).toBe(false)
    expect(shouldAutoRestart({ mismatch: false, newer: true }, 0, false)).toBe(false)
  })
})
