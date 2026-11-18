import { describe, expect, it } from 'vitest'
import { ackOnView } from './attention'

describe('ackOnView', () => {
  it('acks a finished turn or an error once seen', () => {
    expect(ackOnView({ unread: true, attentionKind: 'done', state: 'idle' })).toBe(true)
    expect(ackOnView({ unread: true, attentionKind: 'error', state: 'idle' })).toBe(true)
  })
  it('keeps a pending approval or question flagged', () => {
    expect(ackOnView({ unread: true, attentionKind: 'permission', state: 'waiting_input' })).toBe(false)
    expect(ackOnView({ unread: true, attentionKind: 'question', state: 'waiting_input' })).toBe(false)
  })
  it('has nothing to ack otherwise', () => {
    expect(ackOnView({ unread: false, attentionKind: 'done', state: 'idle' })).toBe(false)
    expect(ackOnView({ unread: true, attentionKind: '', state: 'idle' })).toBe(false)
    expect(ackOnView({ unread: true, attentionKind: 'done', state: 'exited' })).toBe(false)
  })
})
