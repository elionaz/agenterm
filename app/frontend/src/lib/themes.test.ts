import { describe, expect, it } from 'vitest'
import { DEFAULT_THEME, normPrefs, themeAliases, themeCredits, themeLabels, themeName, themeNames, themes } from './themes'

describe('themes', () => {
  it('names the album themes and the classic palettes', () => {
    expect(themeNames.slice(0, 4)).toEqual(['bright-lights', 'room-on-fire', 'it-still-moves', 'alphabetical'])
    for (const n of themeNames) expect(themeLabels[n]).toBeTruthy()
    for (const n of Object.keys(themeCredits)) expect(themes[n]).toBeDefined()
    expect(DEFAULT_THEME).toBe('bright-lights')
  })

  it('maps retired names to their replacements', () => {
    expect(themeName('agenterm-dark')).toBe('bright-lights')
    expect(themeName(' Agenterm-Light ')).toBe('alphabetical')
    expect(themeName('nord')).toBe('nord')
    expect(themeName('nope')).toBe(DEFAULT_THEME)
    expect(normPrefs({ theme: 'agenterm-light' }).theme).toBe('alphabetical')
    for (const to of Object.values(themeAliases)) expect(themes[to]).toBeDefined()
  })

  it('marks light palettes', () => {
    expect(themes['it-still-moves'].light).toBe(true)
    expect(themes['alphabetical'].light).toBe(true)
    expect(themes['room-on-fire'].light).toBe(false)
  })
})
