import { describe, expect, it } from 'vitest'
import { branchParts, shortBranch } from './branch'

describe('shortBranch', () => {
  it('drops the agent/ prefix', () => {
    expect(shortBranch('agent/claude-3145')).toBe('claude-3145')
    expect(shortBranch('feat/agent/x')).toBe('feat/agent/x')
  })
})

describe('branchParts', () => {
  it('keeps short names in one piece', () => {
    expect(branchParts('main')).toEqual({ head: 'main', tail: '' })
    expect(branchParts('agent/claude-1234')).toEqual({ head: 'claude-1234', tail: '' })
  })
  it('splits long names near the end, on a separator', () => {
    const b = 'feat/una-rama-muy-larga-de-prueba-para-ver-el-recorte'
    const p = branchParts(b)
    expect(p.head + p.tail).toBe(b)
    expect(p.tail).toBe('-el-recorte')
    expect([...p.tail].length).toBeLessThanOrEqual(12)
  })
  it('splits without a separator too', () => {
    const p = branchParts('abcdefghijklmnopqrstuvwxyz')
    expect(p).toEqual({ head: 'abcdefghijklmn', tail: 'opqrstuvwxyz' })
  })
})
