import { describe, expect, it } from 'vitest'

import { proposedMode } from './spawnmode'

describe('proposedMode', () => {
  const base = { shell: false, busy: false, remembered: null, fallback: 'worktree' as const }
  it('agents get the remembered choice, else the setting', () => {
    expect(proposedMode(base)).toBe('worktree')
    expect(proposedMode({ ...base, remembered: 'same' })).toBe('same')
  })
  it('a busy folder means a worktree for an agent', () => {
    expect(proposedMode({ ...base, remembered: 'same', busy: true })).toBe('worktree')
  })
  it('shells always start in the project folder', () => {
    expect(proposedMode({ ...base, shell: true, busy: true, remembered: 'worktree' })).toBe('same')
  })
})
