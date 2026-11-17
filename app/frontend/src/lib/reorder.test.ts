import { describe, expect, it } from 'vitest'
import { arrangeWithin, dropSlot, moveBy, moveTo } from './reorder'

describe('dropSlot', () => {
  const rect = { top: 100, height: 20 }
  it('picks the gap before or after the item', () => {
    expect(dropSlot(105, rect, 3)).toBe(3)
    expect(dropSlot(115, rect, 3)).toBe(4)
  })
})

describe('moveTo', () => {
  const ids = ['a', 'b', 'c', 'd']
  it('moves an item into a gap', () => {
    expect(moveTo(ids, 'a', 4)).toEqual(['b', 'c', 'd', 'a'])
    expect(moveTo(ids, 'd', 0)).toEqual(['d', 'a', 'b', 'c'])
    expect(moveTo(ids, 'b', 3)).toEqual(['a', 'c', 'b', 'd'])
  })
  it('returns null when nothing changes', () => {
    expect(moveTo(ids, 'b', 1)).toBeNull()
    expect(moveTo(ids, 'b', 2)).toBeNull()
    expect(moveTo(ids, 'x', 0)).toBeNull()
    expect(moveTo(ids, 'a', 9)).toBeNull()
  })
  it('does not modify its input', () => {
    moveTo(ids, 'a', 4)
    expect(ids).toEqual(['a', 'b', 'c', 'd'])
  })
})

describe('moveBy', () => {
  const ids = ['a', 'b', 'c']
  it('moves one place', () => {
    expect(moveBy(ids, 'b', -1)).toEqual(['b', 'a', 'c'])
    expect(moveBy(ids, 'b', 1)).toEqual(['a', 'c', 'b'])
  })
  it('stops at the edges', () => {
    expect(moveBy(ids, 'a', -1)).toBeNull()
    expect(moveBy(ids, 'c', 1)).toBeNull()
  })
})

describe('arrangeWithin', () => {
  it('reorders one group in place', () => {
    const items = ['a1', 'b1', 'a2', 'a3'].map((id) => ({ id, p: id[0] }))
    const out = arrangeWithin(items, (x) => x.p === 'a', ['a3', 'a1'])
    expect(out.map((x) => x.id)).toEqual(['a3', 'b1', 'a1', 'a2'])
  })
})
