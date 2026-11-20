import { describe, expect, it } from 'vitest'
import { orphanList, removable } from './orphans'

const w = (id: string, orphan: boolean, merged?: boolean, uncommitted = 0, git = true) => ({
  id,
  orphan,
  createdAt: '',
  git: git ? { merged, uncommitted } : undefined,
})

describe('orphans', () => {
  it('removable only when merged and clean', () => {
    expect(removable(w('a', true, true))).toBe(true)
    expect(removable(w('b', true, true, 2))).toBe(false)
    expect(removable(w('c', true, false))).toBe(false)
    expect(removable(w('d', true, undefined, 0, false))).toBe(false)
  })
  it('lists orphans only, removable first, keeping the order otherwise', () => {
    const list = [w('open', true, false), w('live', false, true), w('m1', true, true), w('dirty', true, true, 1), w('m2', true, true)]
    expect(orphanList(list).map((x) => x.id)).toEqual(['m1', 'm2', 'open', 'dirty'])
  })
})
