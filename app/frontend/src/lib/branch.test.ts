import { describe, expect, it } from 'vitest'
import { shortBranch } from './branch'

describe('shortBranch', () => {
  it('drops the agent/ prefix', () => {
    expect(shortBranch('agent/claude-3145')).toBe('claude-3145')
    expect(shortBranch('feat/agent/x')).toBe('feat/agent/x')
  })
  it('keeps short names whole', () => {
    expect(shortBranch('main')).toBe('main')
    expect(shortBranch('feat/notify-settings-panel')).toBe('feat/notify-settings-panel')
  })
  it('cuts long names in the middle', () => {
    const got = shortBranch('feat/a-really-long-branch-name-for-testing', 20)
    expect([...got].length).toBe(20)
    expect(got.startsWith('feat/a-re')).toBe(true)
    expect(got.endsWith('testing')).toBe(true)
    expect(got).toContain('…')
  })
})
