import { describe, expect, it } from 'vitest'
import { prettyModel } from './models'

describe('prettyModel', () => {
  it('names Claude models by family and version', () => {
    expect(prettyModel('claude-opus-5-5')).toBe('Opus 5.5')
    expect(prettyModel('claude-sonnet-5-5-20261001')).toBe('Sonnet 5.5')
    expect(prettyModel('claude-haiku-5-5')).toBe('Haiku 5.5')
    expect(prettyModel('claude-fable-5-2')).toBe('Fable 5.2')
    expect(prettyModel('claude-3-5-sonnet-20241022')).toBe('Sonnet 3.5')
    expect(prettyModel('claude-opus-5-5[1m]')).toBe('Opus 5.5')
  })
  it('shows other ids as they are, clipped', () => {
    expect(prettyModel('gpt-5-codex')).toBe('gpt-5-codex')
    expect(prettyModel('a-very-long-model-name-from-somewhere')).toBe('a-very-long-model-name-…')
    expect(prettyModel('')).toBe('')
    expect(prettyModel(undefined)).toBe('')
  })
})
