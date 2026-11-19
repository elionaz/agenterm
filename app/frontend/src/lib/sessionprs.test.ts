import { describe, expect, it } from 'vitest'
import type { ForgeListItem } from './api'
import { selectedBranch, showPRList } from './sessionprs'

const item = (branch: string, current = false) => ({ branch, current, pr: { number: 1 } }) as unknown as ForgeListItem

describe('showPRList', () => {
  it('stays hidden for none, or only the current branch', () => {
    expect(showPRList(undefined)).toBe(false)
    expect(showPRList([])).toBe(false)
    expect(showPRList([item('feat/b', true)])).toBe(false)
  })
  it('shows several, or one of another branch', () => {
    expect(showPRList([item('feat/b', true), item('fix/a')])).toBe(true)
    expect(showPRList([item('fix/a')])).toBe(true)
  })
})

describe('selectedBranch', () => {
  const items = [item('feat/b', true), item('fix/a')]
  it('keeps a listed other branch', () => {
    expect(selectedBranch(items, 'fix/a')).toBe('fix/a')
  })
  it('falls back to the current branch (empty)', () => {
    expect(selectedBranch(items, '')).toBe('')
    expect(selectedBranch(items, 'feat/b')).toBe('')
    expect(selectedBranch(items, 'gone')).toBe('')
    expect(selectedBranch([], 'fix/a')).toBe('')
  })
})
