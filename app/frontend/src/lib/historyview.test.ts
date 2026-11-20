import { describe, expect, it } from 'vitest'
import { duration, highlight, matchingLines } from './historyview'

describe('highlight', () => {
  it('splits at byte ranges, multibyte safe', () => {
    expect(highlight('fix the redirect now', [[8, 16]])).toEqual([
      { text: 'fix the ', hit: false },
      { text: 'redirect', hit: true },
      { text: ' now', hit: false },
    ])
    // "…" is 3 bytes: the match after it is at byte 3.
    expect(highlight('…abc', [[3, 6]])).toEqual([
      { text: '…', hit: false },
      { text: 'abc', hit: true },
    ])
  })
  it('ignores bad or overlapping ranges', () => {
    expect(highlight('abc', null)).toEqual([{ text: 'abc', hit: false }])
    expect(highlight('abcdef', [[0, 3], [1, 2], [4, 99]])).toEqual([
      { text: 'abc', hit: true },
      { text: 'def', hit: false },
    ])
  })
})

describe('matchingLines', () => {
  it('finds lines with any word', () => {
    expect(matchingLines('one\nTwo words\nthree', 'two THREE')).toEqual([1, 2])
    expect(matchingLines('x', '  ')).toEqual([])
  })
})

describe('duration', () => {
  it('formats', () => {
    const t0 = '2026-11-20T10:00:00Z'
    expect(duration(t0, '2026-11-20T10:45:00Z')).toBe('45 min')
    expect(duration(t0, '2026-11-20T12:05:00Z')).toBe('2 h 5 min')
    expect(duration(t0, '2026-11-20T12:00:00Z')).toBe('2 h')
    expect(duration(t0, '2026-11-23T10:00:00Z')).toBe('3 d')
    expect(duration(t0, 'nope')).toBe('')
  })
})
