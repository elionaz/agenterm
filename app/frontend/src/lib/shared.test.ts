import { describe, expect, it } from 'vitest'
import { folderSharers, inProjectFolder, joinNames } from './shared'

const s = (id: string, projectId: string, state: string, wt?: string, profileId = 'claude') => ({
  id,
  projectId,
  profileId,
  state,
  mode: wt ? 'worktree' : 'same',
  worktreeId: wt,
})

describe('folderSharers', () => {
  const list = [
    s('a', 'p1', 'running'),
    s('b', 'p1', 'idle'),
    s('c', 'p1', 'running', 'c'), // own worktree
    s('d', 'p1', 'exited'),
    s('e', 'p1', 'suspended'),
    s('g', 'p1', 'running', undefined, 'shell'), // a plain terminal
    s('f', 'p2', 'waiting_input'),
  ]

  it('lists the live agents in the project folder', () => {
    expect(folderSharers(list, 'p1').map((x) => x.id)).toEqual(['a', 'b'])
    expect(folderSharers(list, 'p2').map((x) => x.id)).toEqual(['f'])
    expect(folderSharers(list, 'p3')).toEqual([])
  })

  it('leaves the given session out', () => {
    expect(folderSharers(list, 'p1', 'a').map((x) => x.id)).toEqual(['b'])
    expect(folderSharers(list, 'p2', 'f')).toEqual([])
  })

  it('counts a reopened worktree as a worktree', () => {
    expect(inProjectFolder({ mode: 'same', worktreeId: 'x' })).toBe(false)
    expect(inProjectFolder({ mode: 'worktree' })).toBe(false)
    expect(inProjectFolder({})).toBe(true)
  })
})

describe('joinNames', () => {
  it('counts repeated names once', () => {
    expect(joinNames(['claude', 'codex', 'claude'])).toBe('claude ×2, codex')
    expect(joinNames(['api'])).toBe('api')
    expect(joinNames([])).toBe('')
  })
})
