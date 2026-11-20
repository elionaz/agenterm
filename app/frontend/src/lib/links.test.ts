import { describe, expect, it } from 'vitest'
import { openLink } from './links'

describe('openLink', () => {
  const cmd = { metaKey: true }

  it('ignores a plain click', () => {
    const opened: string[] = []
    expect(openLink({ metaKey: false }, 'https://example.com', (u) => opened.push(u), true)).toBe(false)
    expect(opened).toEqual([])
  })

  it('opens http and https URLs on ⌘-click', () => {
    const opened: string[] = []
    const open = (u: string) => { opened.push(u) }
    expect(openLink(cmd, 'https://example.com/a?b=1', open, true)).toBe(true)
    expect(openLink(cmd, 'http://localhost:5173/', open, true)).toBe(true)
    expect(opened).toEqual(['https://example.com/a?b=1', 'http://localhost:5173/'])
  })

  it.each(['javascript:alert(1)', 'file:///etc/passwd', 'agenterm://session/x', 'not a url', ''])(
    'refuses %j',
    (bad) => {
      const opened: string[] = []
      expect(openLink(cmd, bad, (u) => opened.push(u), true)).toBe(false)
      expect(opened).toEqual([])
    },
  )
})

it('opens on Ctrl-click on Linux', () => {
  const opened: string[] = []
  expect(openLink({ metaKey: true, ctrlKey: false }, 'https://example.com', (u) => opened.push(u), false)).toBe(false)
  expect(openLink({ metaKey: false, ctrlKey: true }, 'https://example.com', (u) => opened.push(u), false)).toBe(true)
  expect(opened).toEqual(['https://example.com/'])
})
