import { describe, expect, it } from 'vitest'
import { DEFAULT_THEME, normPrefs, themeAliases, themeLabels, themeName, themeNames, themes } from './themes'

// WCAG 2 relative luminance and contrast ratio.
function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}
function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

describe('themes', () => {
  it('names every theme after an album, dark ones first', () => {
    expect(themeNames).toEqual([
      'bright-lights',
      'room-on-fire',
      'is-this-it',
      'funeral',
      'silent-alarm',
      'z',
      'it-still-moves',
      'alphabetical',
      'hot-fuss',
    ])
    expect(Object.keys(themeLabels).sort()).toEqual([...themeNames].sort())
    // Only the album title: no artist or year.
    for (const n of themeNames) expect(themeLabels[n]).not.toMatch(/[·(]|\d{4}/)
    expect(DEFAULT_THEME).toBe('bright-lights')
  })

  it('maps retired names to their replacements', () => {
    expect(themeName('panal-dark')).toBe('bright-lights')
    expect(themeName(' Panal-Light ')).toBe('alphabetical')
    expect(themeName('nord')).toBe('silent-alarm')
    expect(themeName('Dracula')).toBe('funeral')
    expect(themeName('gruvbox-dark')).toBe('is-this-it')
    expect(themeName('solarized-dark')).toBe('z')
    expect(themeName('solarized-light')).toBe('hot-fuss')
    expect(themeName('z')).toBe('z')
    expect(themeName('nope')).toBe(DEFAULT_THEME)
    expect(normPrefs({ theme: 'panal-light' }).theme).toBe('alphabetical')
    for (const [from, to] of Object.entries(themeAliases)) {
      expect(themes[to]).toBeDefined()
      expect(themes[from]).toBeUndefined()
      // A light palette stays light.
      expect(themes[to].light).toBe(from.endsWith('-light'))
    }
  })

  it('defines the same colours in every palette', () => {
    const keys = Object.keys(themes[DEFAULT_THEME].colors).sort()
    for (const n of themeNames) expect(Object.keys(themes[n].colors).sort(), n).toEqual(keys)
  })

  it('marks light palettes', () => {
    const light = themeNames.filter((n) => themes[n].light)
    expect(light).toEqual(['it-still-moves', 'alphabetical', 'hot-fuss'])
  })

  it('keeps text and ANSI colours readable', () => {
    const ansi = ['red', 'green', 'yellow', 'blue', 'magenta', 'cyan'] as const
    for (const n of themeNames) {
      const c = themes[n].colors
      const bg = c.background!
      expect(contrast(c.foreground!, bg), `${n} foreground`).toBeGreaterThanOrEqual(7)
      for (const k of ansi) {
        const bright = `bright${k[0].toUpperCase()}${k.slice(1)}` as keyof typeof c
        expect(contrast(c[k] as string, bg), `${n} ${k}`).toBeGreaterThanOrEqual(3)
        expect(contrast(c[bright] as string, bg), `${n} ${bright}`).toBeGreaterThanOrEqual(3)
      }
      // On a light background "white" text must still show (CLIs use it for emphasis).
      if (themes[n].light) {
        expect(contrast(c.white!, bg), `${n} white`).toBeGreaterThanOrEqual(4.5)
        expect(contrast(c.brightWhite!, bg), `${n} brightWhite`).toBeGreaterThanOrEqual(3)
        // Dim text (bright black) and emphasis (bright white) must not look alike.
        expect(contrast(c.brightBlack!, bg), `${n} brightBlack`).toBeGreaterThanOrEqual(4.5)
        expect(contrast(c.brightWhite!, c.brightBlack!), `${n} brightWhite vs brightBlack`).toBeGreaterThanOrEqual(1.5)
      }
    }
  })
})
