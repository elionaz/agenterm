import { describe, expect, it } from 'vitest'
import { clampSidebar, loadSidebarWidth, saveSidebarWidth, SIDEBAR_DEFAULT, SIDEBAR_MAX, SIDEBAR_MIN } from './sidebarwidth'

function memStore() {
  const m = new Map<string, string>()
  return {
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => void m.set(k, v),
    removeItem: (k: string) => void m.delete(k),
    m,
  }
}

describe('sidebar width', () => {
  it('clamps', () => {
    expect(clampSidebar(100)).toBe(SIDEBAR_MIN)
    expect(clampSidebar(1000)).toBe(SIDEBAR_MAX)
    expect(clampSidebar(300.6)).toBe(301)
    expect(clampSidebar(NaN)).toBe(SIDEBAR_DEFAULT)
  })
  it('round-trips and forgets the default', () => {
    const s = memStore()
    expect(loadSidebarWidth(s)).toBe(SIDEBAR_DEFAULT)
    saveSidebarWidth(320, s)
    expect(loadSidebarWidth(s)).toBe(320)
    saveSidebarWidth(SIDEBAR_DEFAULT, s)
    expect(s.m.size).toBe(0)
  })
  it('survives broken storage', () => {
    const bad = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
      removeItem: () => {},
    }
    expect(loadSidebarWidth(bad)).toBe(SIDEBAR_DEFAULT)
    expect(() => saveSidebarWidth(300, bad)).not.toThrow()
    expect(loadSidebarWidth({ getItem: () => 'junk' })).toBe(SIDEBAR_DEFAULT)
  })
})
