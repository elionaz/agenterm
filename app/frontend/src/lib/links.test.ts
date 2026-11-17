import { describe, expect, it } from 'vitest'
import { openLink } from './links'

describe('openLink', () => {
  const cmd = { metaKey: true }

  it('ignores a plain click', () => {
    const opened: string[] = []
    expect(openLink({ metaKey: false }, 'https://example.com', (u) => opened.push(u))).toBe(false)
    expect(opened).toEqual([])
  })

  it('opens http and https URLs on ⌘-click', () => {
    const opened: string[] = []
    const open = (u: string) => { opened.push(u) }
    expect(openLink(cmd, 'https://example.com/a?b=1', open)).toBe(true)
    expect(openLink(cmd, 'http://localhost:5173/', open)).toBe(true)
    expect(opened).toEqual(['https://example.com/a?b=1', 'http://localhost:5173/'])
  })

  it.each(['javascript:alert(1)', 'file:///etc/passwd', 'agenterm://session/x', 'not a url', ''])(
    'refuses %j',
    (bad) => {
      const opened: string[] = []
      expect(openLink(cmd, bad, (u) => opened.push(u))).toBe(false)
      expect(opened).toEqual([])
    },
  )
})
