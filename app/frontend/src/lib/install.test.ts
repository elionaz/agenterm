import { describe, expect, it } from 'vitest'
import { installCommand } from './install'

describe('installCommand', () => {
  it('splits the command from its alternatives', () => {
    expect(installCommand('npm i -g @openai/codex   (or brew install --cask codex)')).toEqual({
      cmd: 'npm i -g @openai/codex',
      alt: 'brew install --cask codex',
    })
  })
  it('keeps a single command whole', () => {
    expect(installCommand('brew install block-goose-cli')).toEqual({ cmd: 'brew install block-goose-cli', alt: '' })
    expect(installCommand(undefined)).toEqual({ cmd: '', alt: '' })
  })
})
