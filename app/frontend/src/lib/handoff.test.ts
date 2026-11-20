import { describe, expect, it } from 'vitest'
import { arrange, topLevel } from './handoff'

type S = { id: string; state: string; continuedFrom?: string[] }
const s = (id: string, state = 'running', ...from: string[]): S => ({ id, state, continuedFrom: from.length ? from : undefined })
const ids = (rows: { s: S; child: boolean }[]) => rows.map((r) => (r.child ? `↳${r.s.id}` : r.s.id))

describe('arrange', () => {
  it('folds a closed session under the one that continues it', () => {
    const list = [s('a', 'exited'), s('x'), s('b', 'running', 'a')]
    expect(ids(arrange(list, {}))).toEqual(['x', 'b'])
    expect(ids(arrange(list, { b: true }))).toEqual(['x', 'b', '↳a'])
    const b = arrange(list, {}).find((r) => r.s.id === 'b')
    expect(b?.earlier).toBe(1)
    expect(b?.from?.id).toBe('a')
  })
  it('chains earlier sessions', () => {
    const list = [s('a', 'exited'), s('b', 'exited', 'a'), s('c', 'idle', 'b')]
    expect(ids(arrange(list, { c: true }))).toEqual(['c', '↳b', '↳a'])
    expect(arrange(list, {})[0].earlier).toBe(2)
  })
  it('folds several sessions handed into one', () => {
    const list = [s('a', 'exited'), s('b', 'exited'), s('t', 'running', 'a', 'b')]
    expect(ids(arrange(list, {}))).toEqual(['t'])
    expect(ids(arrange(list, { t: true }))).toEqual(['t', '↳a', '↳b'])
    expect(topLevel(list).length).toBe(1)
  })
  it('keeps an original left open, and one whose successor is gone', () => {
    expect(ids(arrange([s('a'), s('b', 'running', 'a')], {}))).toEqual(['a', 'b'])
    expect(ids(arrange([s('a', 'exited'), s('c')], {}))).toEqual(['a', 'c'])
  })
  it('numbers top-level rows only', () => {
    const list = [s('a', 'exited'), s('b', 'running', 'a'), s('c')]
    expect(arrange(list, { b: true }).map((r) => r.n)).toEqual([0, -1, 1])
    expect(topLevel(list).map((x) => x.id)).toEqual(['b', 'c'])
  })
  it('survives a loop', () => {
    const list = [s('a', 'exited', 'b'), s('b', 'exited', 'a')]
    expect(arrange(list, {}).length).toBeGreaterThan(0)
  })
})
