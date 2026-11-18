import { describe, expect, it } from 'vitest'
import { PROJECT_COLORS, badgeContent, initials, projectColor } from './badge'

describe('initials', () => {
  it('takes two letters', () => {
    expect(initials('agenterm')).toBe('AG')
    expect(initials('web-shop')).toBe('WS')
    expect(initials('my api')).toBe('MA')
    expect(initials('@scope/pkg_name')).toBe('SP')
    expect(initials('x')).toBe('X')
    expect(initials('ñandú')).toBe('ÑA')
    expect(initials('2026 notes')).toBe('2N')
  })
  it('falls back to the first symbol', () => {
    expect(initials('🚀🚀')).toBe('🚀')
    expect(initials('')).toBe('?')
  })
})

describe('projectColor', () => {
  it('keeps a picked color', () => {
    expect(projectColor({ id: 'ab12', color: 'pink' })).toBe('pink')
  })
  it('derives a stable color from the id otherwise', () => {
    const c = projectColor({ id: 'ab12' })
    expect(PROJECT_COLORS).toContain(c)
    expect(projectColor({ id: 'ab12', color: '' })).toBe(c)
    expect(projectColor({ id: 'ab12', color: 'nope' })).toBe(c)
    // Different ids spread over the palette.
    const seen = new Set(['a1', 'b2', 'c3', 'd4', 'e5', 'f6', 'g7', 'h8'].map((id) => projectColor({ id })))
    expect(seen.size).toBeGreaterThan(3)
  })
})

describe('badgeContent', () => {
  it('shows the icon, or the initials without one', () => {
    expect(badgeContent({ name: 'web', icon: '🦊' })).toEqual({ text: '🦊', emoji: true })
    expect(badgeContent({ name: 'web', icon: '👩‍💻' })).toEqual({ text: '👩‍💻', emoji: true })
    expect(badgeContent({ name: 'web', icon: 'ab' })).toEqual({ text: 'ab', emoji: false })
    expect(badgeContent({ name: 'web', icon: ' ' })).toEqual({ text: 'WE', emoji: false })
    expect(badgeContent({ name: 'web-shop' })).toEqual({ text: 'WS', emoji: false })
  })
})
