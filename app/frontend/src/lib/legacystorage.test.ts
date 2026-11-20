import { describe, expect, it } from 'vitest'
import { migrateLegacyKeys } from './legacystorage'

function memStore(init: Record<string, string>): Storage {
  const m = new Map(Object.entries(init))
  return {
    get length() { return m.size },
    key: (i: number) => [...m.keys()][i] ?? null,
    getItem: (k: string) => m.get(k) ?? null,
    setItem: (k: string, v: string) => { m.set(k, v) },
    removeItem: (k: string) => { m.delete(k) },
    clear: () => m.clear(),
  }
}

describe('migrateLegacyKeys', () => {
  it('moves agenterm.* keys to panal.* without overwriting', () => {
    const s = memStore({ 'agenterm.language': 'es', 'agenterm.sidebarWidth': '300', 'panal.sidebarWidth': '250', other: 'x' })
    migrateLegacyKeys(s)
    expect(s.getItem('panal.language')).toBe('es')
    expect(s.getItem('panal.sidebarWidth')).toBe('250')
    expect(s.getItem('agenterm.language')).toBeNull()
    expect(s.getItem('agenterm.sidebarWidth')).toBeNull()
    expect(s.getItem('other')).toBe('x')
  })
})
