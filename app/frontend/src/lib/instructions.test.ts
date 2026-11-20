import { describe, expect, it } from 'vitest'
import type { InstructionTopic, Profile } from './api'
import { blankTopic, byteLength, liveProjects, openSuggestions, receivers, slugify } from './instructions'

const p = (id: string, setupHooks: string, more: Partial<Profile> = {}): Profile =>
  ({ id, name: id, cmd: id, status: 'hooks', setupHooks, source: 'builtin', enabled: true, installed: true, ...more }) as Profile

describe('receivers', () => {
  it('splits by integration', () => {
    const r = receivers(
      [
        p('claude', 'claude'),
        p('claude-yolo', 'claude', { name: 'claude' }),
        p('codex', 'codex'),
        p('gemini', 'none'),
        p('shell', ''),
        p('aider', '', { installed: false }),
        p('off', 'claude', { enabled: false }),
      ],
      false,
    )
    expect(r).toEqual({ yes: ['claude', 'codex'], no: ['gemini'] })
  })
  it('gemini, opencode and cursor get them through their integration', () => {
    const r = receivers([p('gemini', 'gemini'), p('opencode', 'opencode'), p('cursor-agent', 'cursor', { name: 'Cursor CLI' }), p('amp', '')], true)
    expect(r).toEqual({ yes: ['gemini', 'opencode', 'Cursor CLI'], no: ['amp'] })
  })
  it('codex keeps its own developer_instructions', () => {
    expect(receivers([p('codex', 'codex')], true)).toEqual({ yes: [], no: ['codex'] })
  })
})

it('byteLength counts UTF-8', () => {
  expect(byteLength('añ')).toBe(3)
})

it('slugify matches the daemon', () => {
  expect(slugify('Data management')).toBe('data-management')
  expect(slugify('  Seguridad / Auth ')).toBe('seguridad-auth')
  expect(slugify('Diseño de API')).toBe('diseno-de-api')
  expect(slugify('!!!')).toBe('topic')
})

it('openSuggestions hides created ones, in either language', () => {
  const t = (slug: string): InstructionTopic => ({ ...blankTopic(slug), slug })
  const es = { security: 'Seguridad', architecture: 'Arquitectura', data: 'Datos', git: 'Git' }
  const left = openSuggestions([t('seguridad'), t('git')], (s) => es[s.key]).map((s) => s.key)
  expect(left).toEqual(['architecture', 'data'])
  // Created in Spanish, the app now in English: still hidden.
  const en = { security: 'Security', architecture: 'Architecture', data: 'Data management', git: 'Git' }
  expect(openSuggestions([t('seguridad'), t('manejo-de-datos')], (s) => en[s.key]).map((s) => s.key)).toEqual(['architecture', 'git'])
})

it('liveProjects drops removed projects', () => {
  expect(liveProjects({ ...blankTopic('x'), projects: ['a', 'gone'] }, ['a', 'b'])).toEqual(['a'])
})
