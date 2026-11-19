import { describe, expect, it } from 'vitest'
import { permHelp, permLabel } from './permission'

describe('permission labels', () => {
  it('names the known modes', () => {
    expect(permLabel('bypass')).toBe('auto-approve')
    expect(permLabel('auto-edit')).toBe('auto-edit')
    expect(permHelp('plan')).toContain('plan')
  })
  it('says nothing for the default or an unknown mode', () => {
    expect(permLabel(undefined)).toBe('')
    expect(permLabel('')).toBe('')
    expect(permHelp('future')).toBe('')
  })
})
